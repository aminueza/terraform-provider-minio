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
	_ resource.Resource                = &serverConfigHealResource{}
	_ resource.ResourceWithConfigure   = &serverConfigHealResource{}
	_ resource.ResourceWithImportState = &serverConfigHealResource{}
)

type serverConfigHealResource struct {
	client *S3MinioClient
}

type serverConfigHealResourceModel struct {
	ID              types.String   `tfsdk:"id"`
	Bitrotscan      types.String   `tfsdk:"bitrotscan"`
	MaxSleep        types.String   `tfsdk:"max_sleep"`
	MaxIO           types.String   `tfsdk:"max_io"`
	DriveWorkers    types.String   `tfsdk:"drive_workers"`
	RestartRequired types.Bool     `tfsdk:"restart_required"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func newServerConfigHealResource() resource.Resource {
	return &serverConfigHealResource{}
}

func (r *serverConfigHealResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_config_heal"
}

func (r *serverConfigHealResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *serverConfigHealResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages MinIO object healing configuration. Controls background bitrot scanning and data repair settings.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The heal configuration identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bitrotscan": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Bitrot scan mode: \"on\", \"off\", or cycle duration (e.g., \"12m\" for monthly).",
			},
			"max_sleep": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum sleep between heal operations (e.g., \"250ms\").",
			},
			"max_io": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum concurrent I/O operations for healing.",
			},
			"drive_workers": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Number of workers per drive for healing. Empty for auto (1/4 CPU cores).",
			},
			"restart_required": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether a MinIO server restart is required.",
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

func (r *serverConfigHealResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverConfigHealResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Setting MinIO heal configuration")

	resp.Diagnostics.Append(r.applyHealConfig(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigHealResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverConfigHealResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Reading MinIO heal configuration")

	resp.Diagnostics.Append(r.readHealConfig(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigHealResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverConfigHealResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Updating MinIO heal configuration")

	resp.Diagnostics.Append(r.applyHealConfig(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigHealResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverConfigHealResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Deleting MinIO heal configuration")

	_, err := r.client.S3Admin.DelConfigKV(ctx, "heal")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			resp.Diagnostics.AddError("Resetting heal configuration", fmt.Sprintf("Failed to reset heal config: %s", err))
			return
		}
	}

	state.ID = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigHealResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *serverConfigHealResource) applyHealConfig(ctx context.Context, model *serverConfigHealResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	var parts []string
	for _, f := range []string{"bitrotscan", "max_sleep", "max_io", "drive_workers"} {
		var val string
		switch f {
		case "bitrotscan":
			if !model.Bitrotscan.IsNull() && !model.Bitrotscan.IsUnknown() {
				val = model.Bitrotscan.ValueString()
			}
		case "max_sleep":
			if !model.MaxSleep.IsNull() && !model.MaxSleep.IsUnknown() {
				val = model.MaxSleep.ValueString()
			}
		case "max_io":
			if !model.MaxIO.IsNull() && !model.MaxIO.IsUnknown() {
				val = model.MaxIO.ValueString()
			}
		case "drive_workers":
			if !model.DriveWorkers.IsNull() && !model.DriveWorkers.IsUnknown() {
				val = model.DriveWorkers.ValueString()
			}
		}
		if val != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", f, val))
		}
	}

	if len(parts) == 0 {
		model.ID = types.StringValue("heal")
		diags.Append(r.readHealConfig(ctx, model)...)
		return diags
	}

	configString := "heal " + strings.Join(parts, " ")

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
				return retry.RetryableError(fmt.Errorf("transient error setting heal config: %w", err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to set heal config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		diags.AddError("Setting heal configuration", fmt.Sprintf("Failed to set heal config: %s", err))
		return diags
	}

	model.ID = types.StringValue("heal")
	model.RestartRequired = types.BoolValue(restartRequired)

	tflog.Debug(ctx, fmt.Sprintf("Set heal config (restart_required=%v)", restartRequired))

	diags.Append(r.readHealConfig(ctx, model)...)
	return diags
}

func (r *serverConfigHealResource) readHealConfig(ctx context.Context, model *serverConfigHealResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	configData, err := r.client.S3Admin.GetConfigKV(ctx, "heal")
	if err != nil {
		diags.AddError("Reading heal configuration", fmt.Sprintf("Failed to read heal config: %s", err))
		return diags
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, "heal ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	model.ID = types.StringValue("heal")

	for _, f := range []string{"bitrotscan", "max_sleep", "max_io", "drive_workers"} {
		if v, ok := cfgMap[f]; ok && v != "" {
			switch f {
			case "bitrotscan":
				model.Bitrotscan = types.StringValue(v)
			case "max_sleep":
				model.MaxSleep = types.StringValue(v)
			case "max_io":
				model.MaxIO = types.StringValue(v)
			case "drive_workers":
				model.DriveWorkers = types.StringValue(v)
			}
		}
	}

	return diags
}
