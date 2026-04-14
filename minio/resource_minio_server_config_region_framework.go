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
	_ resource.Resource                = &serverConfigRegionResource{}
	_ resource.ResourceWithConfigure   = &serverConfigRegionResource{}
	_ resource.ResourceWithImportState = &serverConfigRegionResource{}
)

type serverConfigRegionResource struct {
	client *S3MinioClient
}

type serverConfigRegionResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Comment         types.String `tfsdk:"comment"`
	RestartRequired types.Bool   `tfsdk:"restart_required"`
}

func newServerConfigRegionResource() resource.Resource {
	return &serverConfigRegionResource{}
}

func (r *serverConfigRegionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_config_region"
}

func (r *serverConfigRegionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *serverConfigRegionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages MinIO server region/site name configuration. Use this resource to configure the region or site name for MinIO distributed deployments.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The region configuration identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Region or site name (e.g., \"us-east-1\", \"dc1-rack2\").",
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Region description.",
			},
			"restart_required": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether a MinIO server restart is required.",
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *serverConfigRegionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverConfigRegionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Setting MinIO region configuration")

	var parts []string
	parts = append(parts, fmt.Sprintf("name=%s", plan.Name.ValueString()))
	if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() && plan.Comment.ValueString() != "" {
		val := plan.Comment.ValueString()
		if strings.ContainsAny(val, " \t") {
			parts = append(parts, fmt.Sprintf("comment=%q", val))
		} else {
			parts = append(parts, fmt.Sprintf("comment=%s", val))
		}
	}

	configString := "region " + strings.Join(parts, " ")

	var restartRequired bool
	err := retry.RetryContext(ctx, 5*time.Minute, func() *retry.RetryError {
		restart, err := r.client.S3Admin.SetConfigKV(ctx, configString)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error setting region config: %w", err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to set region config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("Failed to set region config after retries: %s", err))
		resp.Diagnostics.AddError("Setting region configuration", fmt.Sprintf("Failed to set region config: %s", err))
		return
	}

	plan.ID = types.StringValue("region")
	plan.RestartRequired = types.BoolValue(restartRequired)

	tflog.Debug(ctx, fmt.Sprintf("Set region config (restart_required=%v)", restartRequired))

	if restartRequired {
		tflog.Warn(ctx, "Region config change requires MinIO server restart to take effect")
	} else {
		resp.Diagnostics.Append(r.readRegion(ctx, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigRegionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverConfigRegionResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Reading MinIO region configuration")

	resp.Diagnostics.Append(r.readRegion(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigRegionResource) readRegion(ctx context.Context, model *serverConfigRegionResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	configData, err := r.client.S3Admin.GetConfigKV(ctx, "region")
	if err != nil {
		diags.AddError("Reading region configuration", fmt.Sprintf("Failed to read region config: %s", err))
		return diags
	}

	configStr := strings.TrimSpace(string(configData))
	tflog.Debug(ctx, fmt.Sprintf("Raw config data for region: %s", configStr))

	var valueStr string
	if strings.HasPrefix(configStr, "region ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	model.ID = types.StringValue("region")

	if v, ok := cfgMap["name"]; ok && v != "" {
		model.Name = types.StringValue(v)
	}
	if v, ok := cfgMap["comment"]; ok && v != "" {
		model.Comment = types.StringValue(v)
	}

	return diags
}

func (r *serverConfigRegionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverConfigRegionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Updating MinIO region configuration")

	var parts []string
	parts = append(parts, fmt.Sprintf("name=%s", plan.Name.ValueString()))
	if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() && plan.Comment.ValueString() != "" {
		val := plan.Comment.ValueString()
		if strings.ContainsAny(val, " \t") {
			parts = append(parts, fmt.Sprintf("comment=%q", val))
		} else {
			parts = append(parts, fmt.Sprintf("comment=%s", val))
		}
	}

	configString := "region " + strings.Join(parts, " ")

	var restartRequired bool
	err := retry.RetryContext(ctx, 5*time.Minute, func() *retry.RetryError {
		restart, err := r.client.S3Admin.SetConfigKV(ctx, configString)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error updating region config: %w", err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to update region config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("Failed to update region config after retries: %s", err))
		resp.Diagnostics.AddError("Updating region configuration", fmt.Sprintf("Failed to update region config: %s", err))
		return
	}

	plan.ID = types.StringValue("region")
	plan.RestartRequired = types.BoolValue(restartRequired)

	tflog.Debug(ctx, fmt.Sprintf("Updated region config (restart_required=%v)", restartRequired))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigRegionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverConfigRegionResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Deleting MinIO region configuration")

	_, err := r.client.S3Admin.DelConfigKV(ctx, "region")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			resp.Diagnostics.AddError("Resetting region configuration", fmt.Sprintf("Failed to reset region config: %s", err))
			return
		}
	}

	state.ID = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigRegionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
