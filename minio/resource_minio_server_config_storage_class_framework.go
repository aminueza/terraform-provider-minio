package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
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
	_ resource.Resource                = &serverConfigStorageClassResource{}
	_ resource.ResourceWithConfigure   = &serverConfigStorageClassResource{}
	_ resource.ResourceWithImportState = &serverConfigStorageClassResource{}
)

type serverConfigStorageClassResource struct {
	client *S3MinioClient
}

type serverConfigStorageClassResourceModel struct {
	ID              types.String   `tfsdk:"id"`
	Standard        types.String   `tfsdk:"standard"`
	Rrs             types.String   `tfsdk:"rrs"`
	Comment         types.String   `tfsdk:"comment"`
	RestartRequired types.Bool     `tfsdk:"restart_required"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func newServerConfigStorageClassResource() resource.Resource {
	return &serverConfigStorageClassResource{}
}

func (r *serverConfigStorageClassResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_config_storage_class"
}

func (r *serverConfigStorageClassResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *S3MinioClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *serverConfigStorageClassResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages MinIO storage class configuration for erasure coding parity. Controls data protection levels for STANDARD and REDUCED_REDUNDANCY storage classes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The storage class configuration identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"standard": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Parity for STANDARD storage class (e.g., \"EC:4\" for 4 parity drives).",
			},
			"rrs": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Parity for REDUCED_REDUNDANCY storage class (e.g., \"EC:2\").",
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Optional comment for the storage class configuration.",
			},
			"restart_required": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether a MinIO server restart is required to apply the configuration.",
				Default:     booldefault.StaticBool(false),
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *serverConfigStorageClassResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverConfigStorageClassResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Setting MinIO storage class configuration")

	resp.Diagnostics.Append(r.applyStorageClassConfig(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigStorageClassResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverConfigStorageClassResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Reading MinIO storage class configuration")

	resp.Diagnostics.Append(r.readStorageClassConfig(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigStorageClassResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverConfigStorageClassResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Updating MinIO storage class configuration")

	resp.Diagnostics.Append(r.applyStorageClassConfig(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigStorageClassResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverConfigStorageClassResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Deleting MinIO storage class configuration")

	_, err := r.client.S3Admin.DelConfigKV(ctx, "storage_class")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			resp.Diagnostics.AddError("Resetting storage class configuration", fmt.Sprintf("Failed to reset storage class config: %s", err))
			return
		}
	}

	state.ID = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigStorageClassResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *serverConfigStorageClassResource) applyStorageClassConfig(ctx context.Context, model *serverConfigStorageClassResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	var parts []string
	for _, f := range []string{"standard", "rrs", "comment"} {
		var val string
		switch f {
		case "standard":
			if !model.Standard.IsNull() && !model.Standard.IsUnknown() {
				val = model.Standard.ValueString()
			}
		case "rrs":
			if !model.Rrs.IsNull() && !model.Rrs.IsUnknown() {
				val = model.Rrs.ValueString()
			}
		case "comment":
			if !model.Comment.IsNull() && !model.Comment.IsUnknown() {
				val = model.Comment.ValueString()
			}
		}
		if val != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", f, val))
		}
	}

	if len(parts) == 0 {
		model.ID = types.StringValue("storage_class")
		diags.Append(r.readStorageClassConfig(ctx, model)...)
		return diags
	}

	configString := "storage_class " + strings.Join(parts, " ")

	timeout, d := model.Timeouts.Create(ctx, 0)
	if d.HasError() {
		diags.Append(d...)
		return diags
	}

	var restartRequired bool
	var err error

	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		restart, err := r.client.S3Admin.SetConfigKV(ctx, configString)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error setting storage class config: %w", err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to set storage class config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		diags.AddError("Setting storage class configuration", fmt.Sprintf("Failed to set storage class config: %s", err))
		return diags
	}

	model.ID = types.StringValue("storage_class")
	model.RestartRequired = types.BoolValue(restartRequired)

	tflog.Debug(ctx, fmt.Sprintf("Set storage class config (restart_required=%v)", restartRequired))

	diags.Append(r.readStorageClassConfig(ctx, model)...)
	return diags
}

func (r *serverConfigStorageClassResource) readStorageClassConfig(ctx context.Context, model *serverConfigStorageClassResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	configData, err := r.client.S3Admin.GetConfigKV(ctx, "storage_class")
	if err != nil {
		diags.AddError("Reading storage class configuration", fmt.Sprintf("Failed to read storage class config: %s", err))
		return diags
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, "storage_class ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	model.ID = types.StringValue("storage_class")

	for _, f := range []string{"standard", "rrs", "comment"} {
		if v, ok := cfgMap[f]; ok && v != "" {
			switch f {
			case "standard":
				model.Standard = types.StringValue(v)
			case "rrs":
				model.Rrs = types.StringValue(v)
			case "comment":
				model.Comment = types.StringValue(v)
			}
		}
	}

	return diags
}
