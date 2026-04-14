package minio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

var (
	_ resource.Resource                = &serverConfigEtcdResource{}
	_ resource.ResourceWithConfigure   = &serverConfigEtcdResource{}
	_ resource.ResourceWithImportState = &serverConfigEtcdResource{}
)

type serverConfigEtcdResource struct {
	client *S3MinioClient
}

type serverConfigEtcdResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Endpoints       types.String `tfsdk:"endpoints"`
	PathPrefix      types.String `tfsdk:"path_prefix"`
	CorednsPath     types.String `tfsdk:"coredns_path"`
	ClientCert      types.String `tfsdk:"client_cert"`
	ClientCertKey   types.String `tfsdk:"client_cert_key"`
	Comment         types.String `tfsdk:"comment"`
	RestartRequired types.Bool   `tfsdk:"restart_required"`
}

func newServerConfigEtcdResource() resource.Resource {
	return &serverConfigEtcdResource{}
}

func (r *serverConfigEtcdResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_config_etcd"
}

func (r *serverConfigEtcdResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *S3MinioClient, got: %T", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *serverConfigEtcdResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages MinIO etcd configuration for federated deployments and external IAM storage.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The etcd configuration identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"endpoints": schema.StringAttribute{
				Required:    true,
				Description: "Comma-separated list of etcd endpoint URLs (e.g., \"http://etcd1:2379,http://etcd2:2379\").",
			},
			"path_prefix": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Key prefix for MinIO data in etcd. Enables tenant isolation when set.",
			},
			"coredns_path": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "CoreDNS path for bucket DNS registration (default: \"/skydns\").",
			},
			"client_cert": schema.StringAttribute{
				Optional:    true,
				Description: "Path to client TLS certificate for etcd mTLS.",
			},
			"client_cert_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Path to client TLS private key for etcd mTLS.",
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Optional comment for the etcd configuration.",
			},
			"restart_required": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether a MinIO server restart is required to apply the configuration.",
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *serverConfigEtcdResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverConfigEtcdResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Setting MinIO etcd configuration")

	resp.Diagnostics.Append(r.applyEtcdConfig(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigEtcdResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverConfigEtcdResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Reading MinIO etcd configuration")

	resp.Diagnostics.Append(r.readEtcdConfig(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigEtcdResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverConfigEtcdResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Updating MinIO etcd configuration")

	resp.Diagnostics.Append(r.applyEtcdConfig(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigEtcdResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverConfigEtcdResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Deleting MinIO etcd configuration")

	_, err := r.client.S3Admin.DelConfigKV(ctx, "etcd")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			resp.Diagnostics.AddError("Resetting etcd configuration", fmt.Sprintf("Failed to reset etcd config: %s", err))
			return
		}
	}

	state.ID = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigEtcdResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func addEtcdParam(parts *[]string, key, val string) {
	if val != "" {
		if strings.ContainsAny(val, " \t") {
			*parts = append(*parts, key+"="+`"`+val+`"`)
		} else {
			*parts = append(*parts, key+"="+val)
		}
	}
}

func (r *serverConfigEtcdResource) applyEtcdConfig(ctx context.Context, model *serverConfigEtcdResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	var parts []string

	addEtcdParam(&parts, "endpoints", model.Endpoints.ValueString())

	if !model.PathPrefix.IsNull() && !model.PathPrefix.IsUnknown() {
		addEtcdParam(&parts, "path_prefix", model.PathPrefix.ValueString())
	}
	if !model.CorednsPath.IsNull() && !model.CorednsPath.IsUnknown() {
		addEtcdParam(&parts, "coredns_path", model.CorednsPath.ValueString())
	}
	if !model.ClientCert.IsNull() && !model.ClientCert.IsUnknown() {
		addEtcdParam(&parts, "client_cert", model.ClientCert.ValueString())
	}
	if !model.ClientCertKey.IsNull() && !model.ClientCertKey.IsUnknown() {
		addEtcdParam(&parts, "client_cert_key", model.ClientCertKey.ValueString())
	}
	if !model.Comment.IsNull() && !model.Comment.IsUnknown() {
		addEtcdParam(&parts, "comment", model.Comment.ValueString())
	}

	configString := "etcd " + strings.Join(parts, " ")

	var restartRequired bool
	var err error

	err = retry.RetryContext(ctx, 5*time.Minute, func() *retry.RetryError {
		restart, err := r.client.S3Admin.SetConfigKV(ctx, configString)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error setting etcd config: %w", err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to set etcd config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		diags.AddError("Setting etcd configuration", fmt.Sprintf("Failed to set etcd config: %s", err))
		return diags
	}

	model.ID = types.StringValue("etcd")
	model.RestartRequired = types.BoolValue(restartRequired)

	tflog.Debug(ctx, fmt.Sprintf("Set etcd config (restart_required=%v)", restartRequired))

	diags.Append(r.readEtcdConfig(ctx, model)...)
	return diags
}

func (r *serverConfigEtcdResource) readEtcdConfig(ctx context.Context, model *serverConfigEtcdResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	configData, err := r.client.S3Admin.GetConfigKV(ctx, "etcd")
	if err != nil {
		diags.AddError("Reading etcd configuration", fmt.Sprintf("Failed to read etcd config: %s", err))
		return diags
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, "etcd ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	model.ID = types.StringValue("etcd")

	if v, ok := cfgMap["endpoints"]; ok && v != "" {
		model.Endpoints = types.StringValue(v)
	}
	if v, ok := cfgMap["path_prefix"]; ok && v != "" {
		model.PathPrefix = types.StringValue(v)
	}
	if v, ok := cfgMap["coredns_path"]; ok && v != "" {
		model.CorednsPath = types.StringValue(v)
	}
	if v, ok := cfgMap["client_cert"]; ok && v != "" {
		model.ClientCert = types.StringValue(v)
	}
	if v, ok := cfgMap["comment"]; ok && v != "" {
		model.Comment = types.StringValue(v)
	}

	return diags
}
