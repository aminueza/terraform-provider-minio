package minio

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &prometheusBearerTokenResource{}
	_ resource.ResourceWithConfigure   = &prometheusBearerTokenResource{}
	_ resource.ResourceWithImportState = &prometheusBearerTokenResource{}
)

type prometheusBearerTokenResource struct {
	client *S3MinioClient
}

type prometheusBearerTokenResourceModel struct {
	ID          types.String   `tfsdk:"id"`
	MetricType  types.String   `tfsdk:"metric_type"`
	ExpiresIn   types.String   `tfsdk:"expires_in"`
	Limit       types.Int64    `tfsdk:"limit"`
	Token       types.String   `tfsdk:"token"`
	TokenExpiry types.String   `tfsdk:"token_expiry"`
	Timeouts    timeouts.Value `tfsdk:"timeouts"`
}

func newPrometheusBearerTokenResource() resource.Resource {
	return &prometheusBearerTokenResource{}
}

func (r *prometheusBearerTokenResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_prometheus_bearer_token"
}

func (r *prometheusBearerTokenResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *prometheusBearerTokenResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: `Manages MinIO Prometheus bearer tokens for metrics authentication.

Bearer tokens are JWTs signed with MinIO credentials that authenticate
requests to Prometheus metrics endpoints. Each metric type (cluster, node,
bucket, resource) can have its own token.

Tokens are generated locally using the provider's access and secret keys,
so no API call is needed to create them. The token is valid for the specified
duration from creation time.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The metric type identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"metric_type": schema.StringAttribute{
				Required:    true,
				Description: "Type of metrics to authenticate. Valid values: cluster, node, bucket, resource",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"expires_in": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Token expiry duration in whole hours only (e.g., 24h, 87600h). Go time.Duration formats like 24h30m or units such as m/s are not supported. Default: 87600h (10 years)",
				Default:     stringdefault.StaticString("87600h"),
			},
			"limit": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum token expiry in hours. Default: 876000 (100 years)",
			},
			"token": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Generated JWT bearer token for the metrics endpoint",
			},
			"token_expiry": schema.StringAttribute{
				Computed:    true,
				Description: "Expiry timestamp of the token in RFC3339 format",
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

func (r *prometheusBearerTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan prometheusBearerTokenResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metricType := plan.MetricType.ValueString()
	expiresIn := plan.ExpiresIn.ValueString()
	limit := 876000
	if !plan.Limit.IsNull() && !plan.Limit.IsUnknown() {
		limit = int(plan.Limit.ValueInt64())
	}

	tflog.Info(ctx, fmt.Sprintf("Creating Prometheus bearer token for metric type: %s", metricType))

	duration, err := time.ParseDuration(expiresIn)
	if err != nil {
		resp.Diagnostics.AddError("Parsing expires_in duration", fmt.Sprintf("Failed to parse duration: %s", err))
		return
	}

	effectiveDuration := duration
	if limit > 0 && duration > time.Duration(limit)*time.Hour {
		effectiveDuration = time.Duration(limit) * time.Hour
	}

	token, err := generatePrometheusToken(r.client.S3UserAccess, r.client.S3UserSecret, effectiveDuration, limit)
	if err != nil {
		resp.Diagnostics.AddError("Creating Prometheus bearer token", fmt.Sprintf("Failed to generate token: %s", err))
		return
	}

	expiry := time.Now().UTC().Add(effectiveDuration)

	plan.ID = types.StringValue(metricType)
	plan.Token = types.StringValue(token)
	plan.TokenExpiry = types.StringValue(expiry.Format(time.RFC3339))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *prometheusBearerTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state prometheusBearerTokenResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metricType := state.ID.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Reading Prometheus bearer token for metric type: %s", metricType))

	state.MetricType = types.StringValue(metricType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *prometheusBearerTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan prometheusBearerTokenResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metricType := plan.MetricType.ValueString()
	expiresIn := plan.ExpiresIn.ValueString()
	limit := 876000
	if !plan.Limit.IsNull() && !plan.Limit.IsUnknown() {
		limit = int(plan.Limit.ValueInt64())
	}

	tflog.Info(ctx, fmt.Sprintf("Updating Prometheus bearer token for metric type: %s", metricType))

	duration, err := time.ParseDuration(expiresIn)
	if err != nil {
		resp.Diagnostics.AddError("Parsing expires_in duration", fmt.Sprintf("Failed to parse duration: %s", err))
		return
	}

	effectiveDuration := duration
	if limit > 0 && duration > time.Duration(limit)*time.Hour {
		effectiveDuration = time.Duration(limit) * time.Hour
	}

	token, err := generatePrometheusToken(r.client.S3UserAccess, r.client.S3UserSecret, effectiveDuration, limit)
	if err != nil {
		resp.Diagnostics.AddError("Updating Prometheus bearer token", fmt.Sprintf("Failed to generate token: %s", err))
		return
	}

	expiry := time.Now().UTC().Add(effectiveDuration)

	plan.Token = types.StringValue(token)
	plan.TokenExpiry = types.StringValue(expiry.Format(time.RFC3339))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *prometheusBearerTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state prometheusBearerTokenResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metricType := state.ID.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Deleting Prometheus bearer token for metric type: %s", metricType))

	state.ID = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *prometheusBearerTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
