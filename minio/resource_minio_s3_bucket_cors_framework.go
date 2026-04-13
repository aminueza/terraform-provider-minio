package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/minio-go/v7/pkg/cors"
)

var (
	_ resource.Resource                = &bucketCorsResource{}
	_ resource.ResourceWithConfigure   = &bucketCorsResource{}
	_ resource.ResourceWithImportState = &bucketCorsResource{}
)

type bucketCorsResource struct {
	client *S3MinioClient
}

type bucketCorsResourceModel struct {
	ID       types.String    `tfsdk:"id"`
	Bucket   types.String    `tfsdk:"bucket"`
	CorsRule []corsRuleModel `tfsdk:"cors_rule"`
}

type corsRuleModel struct {
	ID             types.String   `tfsdk:"id"`
	AllowedHeaders []types.String `tfsdk:"allowed_headers"`
	AllowedMethods []types.String `tfsdk:"allowed_methods"`
	AllowedOrigins []types.String `tfsdk:"allowed_origins"`
	ExposeHeaders  []types.String `tfsdk:"expose_headers"`
	MaxAgeSeconds  types.Int64    `tfsdk:"max_age_seconds"`
}

func (r *bucketCorsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket_cors"
}

func (r *bucketCorsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Cross-Origin Resource Sharing (CORS) configuration for S3 buckets in MinIO.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Bucket name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Description: "Name of the bucket to apply CORS configuration",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cors_rule": schema.ListNestedAttribute{
				Description: "List of CORS rules",
				Required:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "Unique identifier for the rule",
							Optional:    true,
						},
						"allowed_headers": schema.ListAttribute{
							Description: "Headers that are allowed in a preflight OPTIONS request",
							Optional:    true,
							ElementType: types.StringType,
						},
						"allowed_methods": schema.ListAttribute{
							Description: "HTTP methods that the origin is allowed to execute (GET, PUT, POST, DELETE, HEAD)",
							Required:    true,
							ElementType: types.StringType,
						},
						"allowed_origins": schema.ListAttribute{
							Description: "Origins that are allowed to access the bucket",
							Required:    true,
							ElementType: types.StringType,
						},
						"expose_headers": schema.ListAttribute{
							Description: "Headers in the response that customers are able to access from their applications",
							Optional:    true,
							ElementType: types.StringType,
						},
						"max_age_seconds": schema.Int64Attribute{
							Description: "Time in seconds that browser can cache the response for a preflight request",
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

func (r *bucketCorsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *bucketCorsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data bucketCorsResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setCors(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = data.Bucket

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketCorsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data bucketCorsResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketCorsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data bucketCorsResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setCors(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketCorsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data bucketCorsResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	emptyConfig := &cors.Config{
		CORSRules: []cors.Rule{},
	}

	err := r.client.S3Client.SetBucketCors(ctx, data.Bucket.ValueString(), emptyConfig)
	if err != nil {
		if isNoSuchBucketError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting CORS configuration",
			err.Error(),
		)
		return
	}
}

func (r *bucketCorsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("bucket"), req, resp)
}

func (r *bucketCorsResource) setCors(ctx context.Context, data *bucketCorsResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	corsConfig := r.buildCorsConfig(data)

	err := r.client.S3Client.SetBucketCors(ctx, data.Bucket.ValueString(), corsConfig)
	if err != nil {
		diags.AddError(
			"Error setting CORS configuration",
			err.Error(),
		)
		return diags
	}

	return diags
}

func (r *bucketCorsResource) read(ctx context.Context, data *bucketCorsResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	corsConfig, err := r.client.S3Client.GetBucketCors(ctx, data.Bucket.ValueString())
	if err != nil {
		if isS3CompatNotSupported(r.client, err) {
			data.ID = types.StringNull()
			return diags
		}
		if isNoSuchBucketError(err) {
			data.ID = types.StringNull()
			return diags
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "CORS configuration does not exist") || strings.Contains(errMsg, "NoSuchCORSConfiguration") {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError(
			"Error reading CORS configuration",
			err.Error(),
		)
		return diags
	}

	if corsConfig == nil || len(corsConfig.CORSRules) == 0 {
		data.ID = types.StringNull()
		return diags
	}

	data.CorsRule = r.flattenCorsRules(corsConfig.CORSRules)

	if data.ID.IsNull() {
		data.ID = data.Bucket
	}

	return diags
}

func (r *bucketCorsResource) buildCorsConfig(data *bucketCorsResourceModel) *cors.Config {
	rules := make([]cors.Rule, 0, len(data.CorsRule))

	for _, ruleModel := range data.CorsRule {
		rule := cors.Rule{}

		if !ruleModel.ID.IsNull() {
			rule.ID = ruleModel.ID.ValueString()
		}

		if len(ruleModel.AllowedHeaders) > 0 {
			rule.AllowedHeader = make([]string, len(ruleModel.AllowedHeaders))
			for i, h := range ruleModel.AllowedHeaders {
				if !h.IsNull() {
					rule.AllowedHeader[i] = h.ValueString()
				}
			}
		}

		if len(ruleModel.AllowedMethods) > 0 {
			rule.AllowedMethod = make([]string, len(ruleModel.AllowedMethods))
			for i, m := range ruleModel.AllowedMethods {
				if !m.IsNull() {
					rule.AllowedMethod[i] = m.ValueString()
				}
			}
		}

		if len(ruleModel.AllowedOrigins) > 0 {
			rule.AllowedOrigin = make([]string, len(ruleModel.AllowedOrigins))
			for i, o := range ruleModel.AllowedOrigins {
				if !o.IsNull() {
					rule.AllowedOrigin[i] = o.ValueString()
				}
			}
		}

		if len(ruleModel.ExposeHeaders) > 0 {
			rule.ExposeHeader = make([]string, len(ruleModel.ExposeHeaders))
			for i, h := range ruleModel.ExposeHeaders {
				if !h.IsNull() {
					rule.ExposeHeader[i] = h.ValueString()
				}
			}
		}

		if !ruleModel.MaxAgeSeconds.IsNull() {
			rule.MaxAgeSeconds = int(ruleModel.MaxAgeSeconds.ValueInt64())
		}

		rules = append(rules, rule)
	}

	return &cors.Config{
		CORSRules: rules,
	}
}

func (r *bucketCorsResource) flattenCorsRules(rules []cors.Rule) []corsRuleModel {
	result := make([]corsRuleModel, 0, len(rules))

	for _, rule := range rules {
		ruleModel := corsRuleModel{}

		if rule.ID != "" {
			ruleModel.ID = types.StringValue(rule.ID)
		}

		if len(rule.AllowedHeader) > 0 {
			ruleModel.AllowedHeaders = make([]types.String, len(rule.AllowedHeader))
			for i, h := range rule.AllowedHeader {
				ruleModel.AllowedHeaders[i] = types.StringValue(h)
			}
		}

		if len(rule.AllowedMethod) > 0 {
			ruleModel.AllowedMethods = make([]types.String, len(rule.AllowedMethod))
			for i, m := range rule.AllowedMethod {
				ruleModel.AllowedMethods[i] = types.StringValue(m)
			}
		}

		if len(rule.AllowedOrigin) > 0 {
			ruleModel.AllowedOrigins = make([]types.String, len(rule.AllowedOrigin))
			for i, o := range rule.AllowedOrigin {
				ruleModel.AllowedOrigins[i] = types.StringValue(o)
			}
		}

		if len(rule.ExposeHeader) > 0 {
			ruleModel.ExposeHeaders = make([]types.String, len(rule.ExposeHeader))
			for i, h := range rule.ExposeHeader {
				ruleModel.ExposeHeaders[i] = types.StringValue(h)
			}
		}

		if rule.MaxAgeSeconds > 0 {
			ruleModel.MaxAgeSeconds = types.Int64Value(int64(rule.MaxAgeSeconds))
		}

		result = append(result, ruleModel)
	}

	return result
}

func newBucketCorsResource() resource.Resource {
	return &bucketCorsResource{}
}
