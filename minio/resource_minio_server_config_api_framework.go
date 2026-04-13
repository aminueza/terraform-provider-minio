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
	_ resource.Resource                = &serverConfigApiResource{}
	_ resource.ResourceWithConfigure   = &serverConfigApiResource{}
	_ resource.ResourceWithImportState = &serverConfigApiResource{}
)

type serverConfigApiResource struct {
	client *S3MinioClient
}

type serverConfigApiResourceModel struct {
	ID                          types.String   `tfsdk:"id"`
	RequestsMax                 types.String   `tfsdk:"requests_max"`
	CorsAllowOrigin             types.String   `tfsdk:"cors_allow_origin"`
	TransitionWorkers           types.String   `tfsdk:"transition_workers"`
	StaleUploadsExpiry          types.String   `tfsdk:"stale_uploads_expiry"`
	StaleUploadsCleanupInterval types.String   `tfsdk:"stale_uploads_cleanup_interval"`
	ClusterDeadline             types.String   `tfsdk:"cluster_deadline"`
	RemoteTransportDeadline     types.String   `tfsdk:"remote_transport_deadline"`
	RootAccess                  types.Bool     `tfsdk:"root_access"`
	SyncEvents                  types.Bool     `tfsdk:"sync_events"`
	RestartRequired             types.Bool     `tfsdk:"restart_required"`
	Timeouts                    timeouts.Value `tfsdk:"timeouts"`
}

func newServerConfigApiResource() resource.Resource {
	return &serverConfigApiResource{}
}

func (r *serverConfigApiResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_config_api"
}

func (r *serverConfigApiResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *serverConfigApiResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages MinIO API server configuration including request throttling, CORS, transition workers, and stale upload cleanup.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The API configuration identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"requests_max": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum concurrent API requests. Use 0 or empty for auto.",
			},
			"cors_allow_origin": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Comma-separated list of allowed CORS origins (e.g., \"https://app.example.com\").",
			},
			"transition_workers": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Number of ILM transition workers.",
			},
			"stale_uploads_expiry": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Duration after which incomplete multipart uploads are cleaned up (e.g., \"24h\").",
			},
			"stale_uploads_cleanup_interval": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Interval between stale upload cleanup runs (e.g., \"6h\").",
			},
			"cluster_deadline": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Deadline for cluster read operations (e.g., \"10s\").",
			},
			"remote_transport_deadline": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Deadline for remote transport operations (e.g., \"2h\").",
			},
			"root_access": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether root user (access/secret key) access is enabled for S3 API.",
			},
			"sync_events": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Enable synchronous bucket notification events.",
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

func (r *serverConfigApiResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverConfigApiResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Setting MinIO API configuration")

	resp.Diagnostics.Append(r.applyApiConfig(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigApiResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverConfigApiResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Reading MinIO API configuration")

	resp.Diagnostics.Append(r.readApiConfig(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigApiResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverConfigApiResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Updating MinIO API configuration")

	resp.Diagnostics.Append(r.applyApiConfig(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverConfigApiResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverConfigApiResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Deleting MinIO API configuration")

	_, err := r.client.S3Admin.DelConfigKV(ctx, "api")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			resp.Diagnostics.AddError("Resetting API configuration", fmt.Sprintf("Failed to reset API config: %s", err))
			return
		}
	}

	state.ID = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverConfigApiResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *serverConfigApiResource) applyApiConfig(ctx context.Context, model *serverConfigApiResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	var parts []string

	// String fields
	strFields := map[string]types.String{
		"requests_max":                   model.RequestsMax,
		"cors_allow_origin":              model.CorsAllowOrigin,
		"transition_workers":             model.TransitionWorkers,
		"stale_uploads_expiry":           model.StaleUploadsExpiry,
		"stale_uploads_cleanup_interval": model.StaleUploadsCleanupInterval,
		"cluster_deadline":               model.ClusterDeadline,
		"remote_transport_deadline":      model.RemoteTransportDeadline,
	}

	for key, val := range strFields {
		if !val.IsNull() && !val.IsUnknown() && val.ValueString() != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", key, val.ValueString()))
		}
	}

	// Bool fields
	if !model.RootAccess.IsNull() && !model.RootAccess.IsUnknown() {
		if model.RootAccess.ValueBool() {
			parts = append(parts, "root_access=on")
		} else {
			parts = append(parts, "root_access=off")
		}
	}
	if !model.SyncEvents.IsNull() && !model.SyncEvents.IsUnknown() {
		if model.SyncEvents.ValueBool() {
			parts = append(parts, "sync_events=on")
		} else {
			parts = append(parts, "sync_events=off")
		}
	}

	if len(parts) == 0 {
		model.ID = types.StringValue("api")
		diags.Append(r.readApiConfig(ctx, model)...)
		return diags
	}

	configString := "api " + strings.Join(parts, " ")

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
				return retry.RetryableError(fmt.Errorf("transient error setting API config: %w", err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to set API config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		diags.AddError("Setting API configuration", fmt.Sprintf("Failed to set API config: %s", err))
		return diags
	}

	model.ID = types.StringValue("api")
	model.RestartRequired = types.BoolValue(restartRequired)

	tflog.Debug(ctx, fmt.Sprintf("Set API config (restart_required=%v)", restartRequired))

	diags.Append(r.readApiConfig(ctx, model)...)
	return diags
}

func (r *serverConfigApiResource) readApiConfig(ctx context.Context, model *serverConfigApiResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	configData, err := r.client.S3Admin.GetConfigKV(ctx, "api")
	if err != nil {
		diags.AddError("Reading API configuration", fmt.Sprintf("Failed to read API config: %s", err))
		return diags
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, "api ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	model.ID = types.StringValue("api")

	// String fields
	strFields := map[string]*types.String{
		"requests_max":                   &model.RequestsMax,
		"cors_allow_origin":              &model.CorsAllowOrigin,
		"transition_workers":             &model.TransitionWorkers,
		"stale_uploads_expiry":           &model.StaleUploadsExpiry,
		"stale_uploads_cleanup_interval": &model.StaleUploadsCleanupInterval,
		"cluster_deadline":               &model.ClusterDeadline,
		"remote_transport_deadline":      &model.RemoteTransportDeadline,
	}

	for key, fieldPtr := range strFields {
		if v, ok := cfgMap[key]; ok {
			*fieldPtr = types.StringValue(v)
		}
	}

	// Bool fields
	boolFields := map[string]*types.Bool{
		"root_access": &model.RootAccess,
		"sync_events": &model.SyncEvents,
	}

	for key, fieldPtr := range boolFields {
		if v, ok := cfgMap[key]; ok {
			*fieldPtr = types.BoolValue(v == "on")
		}
	}

	return diags
}
