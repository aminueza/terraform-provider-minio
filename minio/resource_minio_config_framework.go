package minio

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	_ resource.Resource                = &minioConfigResource{}
	_ resource.ResourceWithConfigure   = &minioConfigResource{}
	_ resource.ResourceWithImportState = &minioConfigResource{}
)

type minioConfigResource struct {
	client *S3MinioClient
}

type minioConfigResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Key             types.String `tfsdk:"key"`
	Value           types.String `tfsdk:"value"`
	RestartRequired types.Bool   `tfsdk:"restart_required"`
}

func newConfigResource() resource.Resource {
	return &minioConfigResource{}
}

func (r *minioConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config"
}

func (r *minioConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *minioConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages MinIO server configuration key-value pairs. Use this resource to configure MinIO server settings such as API limits, notification targets, audit targets, and other subsystem configurations.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The configuration key identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				Required:    true,
				Description: "The configuration key (e.g., 'api', 'notify_webhook:1', 'region')",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				Required:    true,
				Description: "The configuration value in key=value format (e.g., 'requests_max=1000'). For multiple settings, separate with spaces.",
			},
			"restart_required": schema.BoolAttribute{
				Computed:    true,
				Description: "Indicates whether a server restart is required for the configuration to take effect",
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *minioConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioConfigResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key := plan.Key.ValueString()
	value := plan.Value.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Creating MinIO config: %s", key))

	var restartRequired bool
	var err error

	configString := fmt.Sprintf("%s %s", key, value)
	err = retry.RetryContext(ctx, 5*time.Minute, func() *retry.RetryError {
		restart, err := r.client.S3Admin.SetConfigKV(ctx, configString)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error setting config %s: %w", key, err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to set config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("Failed to set config %s after retries: %s", key, err))
		resp.Diagnostics.AddError("Setting config", fmt.Sprintf("Failed to set config %s: %s", key, err))
		return
	}

	plan.ID = types.StringValue(key)
	plan.RestartRequired = types.BoolValue(restartRequired)

	if restartRequired {
		tflog.Warn(ctx, fmt.Sprintf("Config change for %s requires MinIO server restart to take effect", key))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *minioConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key := state.Key.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Reading MinIO config: %s", key))

	// Check if config still exists
	_, err := r.client.S3Admin.GetConfigKV(ctx, key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading config", fmt.Sprintf("Failed to read config %s: %s", key, err))
		return
	}

	// Set ID from key for import to work
	state.ID = state.Key

	// Preserve the user's original value from state
	// MinIO returns expanded config with default values, but we want to keep the user's original format
	// For import, the value will be set by the import logic
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *minioConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioConfigResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key := plan.Key.ValueString()
	value := plan.Value.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Updating MinIO config: %s", key))

	var restartRequired bool
	var err error

	configString := fmt.Sprintf("%s %s", key, value)
	err = retry.RetryContext(ctx, 5*time.Minute, func() *retry.RetryError {
		restart, err := r.client.S3Admin.SetConfigKV(ctx, configString)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error setting config %s: %w", key, err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to set config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("Failed to set config %s after retries: %s", key, err))
		resp.Diagnostics.AddError("Updating config", fmt.Sprintf("Failed to update config %s: %s", key, err))
		return
	}

	plan.ID = types.StringValue(key)
	plan.RestartRequired = types.BoolValue(restartRequired)

	if restartRequired {
		tflog.Warn(ctx, fmt.Sprintf("Config change for %s requires MinIO server restart to take effect", key))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *minioConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key := state.ID.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Deleting MinIO config: %s", key))

	err := retry.RetryContext(ctx, 2*time.Minute, func() *retry.RetryError {
		_, err := r.client.S3Admin.DelConfigKV(ctx, key)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error deleting config %s: %w", key, err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to delete config: %w", err))
		}
		return nil
	})

	if err != nil {
		if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "does not exist") {
			resp.Diagnostics.AddError("Deleting config", fmt.Sprintf("Failed to delete config %s: %s", key, err))
			return
		}
	}
}

func (r *minioConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("key"), req, resp)
}
