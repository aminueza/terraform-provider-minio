package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

var (
	_ resource.Resource                = &serverConfigScannerResource{}
	_ resource.ResourceWithConfigure   = &serverConfigScannerResource{}
	_ resource.ResourceWithImportState = &serverConfigScannerResource{}
)

type serverConfigScannerResource struct {
	client *S3MinioClient
}

type serverConfigScannerResourceModel struct {
	ID              types.String   `tfsdk:"id"`
	Speed           types.String   `tfsdk:"speed"`
	Delay           types.String   `tfsdk:"delay"`
	MaxWait         types.String   `tfsdk:"max_wait"`
	Cycle           types.String   `tfsdk:"cycle"`
	ExcessVersions  types.String   `tfsdk:"excess_versions"`
	ExcessFolders   types.String   `tfsdk:"excess_folders"`
	RestartRequired types.Bool     `tfsdk:"restart_required"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func newServerConfigScannerResource() resource.Resource {
	return &serverConfigScannerResource{}
}

func (r *serverConfigScannerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_config_scanner"
}

func (r *serverConfigScannerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *serverConfigScannerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages MinIO object scanner configuration. The scanner handles background tasks like lifecycle expiration, healing, and versioning cleanup.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The scanner configuration identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"speed": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Scanner speed preset: fastest, fast, default, slow, or slowest.",
				Validators: []validator.String{
					stringvalidator.OneOf("fastest", "fast", "default", "slow", "slowest"),
				},
			},
			"delay": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Scanner delay multiplier between operations.",
			},
			"max_wait": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum wait between scanner cycles (e.g., \"15s\").",
			},
			"cycle": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Time between full scanner cycles (e.g., \"1m\").",
			},
			"excess_versions": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Alert threshold for excess object versions per prefix.",
			},
			"excess_folders": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Alert threshold for excess folders per prefix.",
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

func (r *serverConfigScannerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverConfigScannerResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Setting MinIO scanner configuration")

	resp.Diagnostics.Append(r.applyScannerConfig(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigScannerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverConfigScannerResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Reading MinIO scanner configuration")

	resp.Diagnostics.Append(r.readScannerConfig(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigScannerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverConfigScannerResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Updating MinIO scanner configuration")

	resp.Diagnostics.Append(r.applyScannerConfig(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigScannerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverConfigScannerResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Deleting MinIO scanner configuration")

	_, err := r.client.S3Admin.DelConfigKV(ctx, "scanner")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			resp.Diagnostics.AddError("Resetting scanner configuration", fmt.Sprintf("Failed to reset scanner config: %s", err))
			return
		}
	}

	state.ID = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigScannerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *serverConfigScannerResource) applyScannerConfig(ctx context.Context, model *serverConfigScannerResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	var parts []string
	fields := []string{"speed", "delay", "max_wait", "cycle", "excess_versions", "excess_folders"}
	for _, f := range fields {
		var val string
		switch f {
		case "speed":
			if !model.Speed.IsNull() && !model.Speed.IsUnknown() {
				val = model.Speed.ValueString()
			}
		case "delay":
			if !model.Delay.IsNull() && !model.Delay.IsUnknown() {
				val = model.Delay.ValueString()
			}
		case "max_wait":
			if !model.MaxWait.IsNull() && !model.MaxWait.IsUnknown() {
				val = model.MaxWait.ValueString()
			}
		case "cycle":
			if !model.Cycle.IsNull() && !model.Cycle.IsUnknown() {
				val = model.Cycle.ValueString()
			}
		case "excess_versions":
			if !model.ExcessVersions.IsNull() && !model.ExcessVersions.IsUnknown() {
				val = model.ExcessVersions.ValueString()
			}
		case "excess_folders":
			if !model.ExcessFolders.IsNull() && !model.ExcessFolders.IsUnknown() {
				val = model.ExcessFolders.ValueString()
			}
		}
		if val != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", f, val))
		}
	}

	if len(parts) == 0 {
		model.ID = types.StringValue("scanner")
		diags.Append(r.readScannerConfig(ctx, model)...)
		return diags
	}

	configString := "scanner " + strings.Join(parts, " ")

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
				return retry.RetryableError(fmt.Errorf("transient error setting scanner config: %w", err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to set scanner config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		diags.AddError("Setting scanner configuration", fmt.Sprintf("Failed to set scanner config: %s", err))
		return diags
	}

	model.ID = types.StringValue("scanner")
	model.RestartRequired = types.BoolValue(restartRequired)

	tflog.Debug(ctx, fmt.Sprintf("Set scanner config (restart_required=%v)", restartRequired))

	diags.Append(r.readScannerConfig(ctx, model)...)
	return diags
}

func (r *serverConfigScannerResource) readScannerConfig(ctx context.Context, model *serverConfigScannerResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	configData, err := r.client.S3Admin.GetConfigKV(ctx, "scanner")
	if err != nil {
		diags.AddError("Reading scanner configuration", fmt.Sprintf("Failed to read scanner config: %s", err))
		return diags
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, "scanner ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	model.ID = types.StringValue("scanner")

	for _, f := range []string{"speed", "delay", "max_wait", "cycle", "excess_versions", "excess_folders"} {
		if v, ok := cfgMap[f]; ok {
			switch f {
			case "speed":
				model.Speed = types.StringValue(v)
			case "delay":
				model.Delay = types.StringValue(v)
			case "max_wait":
				model.MaxWait = types.StringValue(v)
			case "cycle":
				model.Cycle = types.StringValue(v)
			case "excess_versions":
				model.ExcessVersions = types.StringValue(v)
			case "excess_folders":
				model.ExcessFolders = types.StringValue(v)
			}
		}
	}

	return diags
}
