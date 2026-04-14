package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	ID       types.String `tfsdk:"id"`
	Bucket   types.String `tfsdk:"bucket"`
	CorsRule types.List   `tfsdk:"cors_rule"`
}

type corsRuleModel struct {
	ID             types.String `tfsdk:"id"`
	AllowedHeaders types.List   `tfsdk:"allowed_headers"`
	AllowedMethods types.List   `tfsdk:"allowed_methods"`
	AllowedOrigins types.List   `tfsdk:"allowed_origins"`
	ExposeHeaders  types.List   `tfsdk:"expose_headers"`
	MaxAgeSeconds  types.Int64  `tfsdk:"max_age_seconds"`
}

var corsRuleObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":              types.StringType,
		"allowed_headers": types.ListType{ElemType: types.StringType},
		"allowed_methods": types.ListType{ElemType: types.StringType},
		"allowed_origins": types.ListType{ElemType: types.StringType},
		"expose_headers":  types.ListType{ElemType: types.StringType},
		"max_age_seconds": types.Int64Type,
	},
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
			"cors_rule": schema.ListAttribute{
				Description: "List of CORS rules",
				Required:    true,
				ElementType: corsRuleObjectType,
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

	corsConfig := r.buildCorsConfig(ctx, data, &diags)
	if diags.HasError() {
		return diags
	}

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

	rules, diags := r.flattenCorsRules(ctx, corsConfig.CORSRules)
	if diags.HasError() {
		return diags
	}
	data.CorsRule = rules

	if data.ID.IsNull() {
		data.ID = data.Bucket
	}

	return diags
}

func (r *bucketCorsResource) buildCorsConfig(ctx context.Context, data *bucketCorsResourceModel, diags *diag.Diagnostics) *cors.Config {
	var rules []cors.Rule

	var corsRuleList []corsRuleModel
	diags.Append(data.CorsRule.ElementsAs(ctx, &corsRuleList, false)...)
	if diags.HasError() {
		return nil
	}

	for _, ruleModel := range corsRuleList {
		rule := cors.Rule{}

		if !ruleModel.ID.IsNull() {
			rule.ID = ruleModel.ID.ValueString()
		}

		if !ruleModel.AllowedHeaders.IsNull() {
			var headers []string
			diags.Append(ruleModel.AllowedHeaders.ElementsAs(ctx, &headers, false)...)
			if diags.HasError() {
				return nil
			}
			rule.AllowedHeader = headers
		}

		if !ruleModel.AllowedMethods.IsNull() {
			var methods []string
			diags.Append(ruleModel.AllowedMethods.ElementsAs(ctx, &methods, false)...)
			if diags.HasError() {
				return nil
			}
			rule.AllowedMethod = methods
		}

		if !ruleModel.AllowedOrigins.IsNull() {
			var origins []string
			diags.Append(ruleModel.AllowedOrigins.ElementsAs(ctx, &origins, false)...)
			if diags.HasError() {
				return nil
			}
			rule.AllowedOrigin = origins
		}

		if !ruleModel.ExposeHeaders.IsNull() {
			var exposeHeaders []string
			diags.Append(ruleModel.ExposeHeaders.ElementsAs(ctx, &exposeHeaders, false)...)
			if diags.HasError() {
				return nil
			}
			rule.ExposeHeader = exposeHeaders
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

func (r *bucketCorsResource) flattenCorsRules(ctx context.Context, rules []cors.Rule) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	result := make([]attr.Value, 0, len(rules))

	for _, rule := range rules {
		ruleModel := corsRuleModel{}

		if rule.ID != "" {
			ruleModel.ID = types.StringValue(rule.ID)
		}

		if len(rule.AllowedHeader) > 0 {
			headers := make([]attr.Value, len(rule.AllowedHeader))
			for i, h := range rule.AllowedHeader {
				headers[i] = types.StringValue(h)
			}
			ruleModel.AllowedHeaders = types.ListValueMust(types.StringType, headers)
		}

		if len(rule.AllowedMethod) > 0 {
			methods := make([]attr.Value, len(rule.AllowedMethod))
			for i, m := range rule.AllowedMethod {
				methods[i] = types.StringValue(m)
			}
			ruleModel.AllowedMethods = types.ListValueMust(types.StringType, methods)
		}

		if len(rule.AllowedOrigin) > 0 {
			origins := make([]attr.Value, len(rule.AllowedOrigin))
			for i, o := range rule.AllowedOrigin {
				origins[i] = types.StringValue(o)
			}
			ruleModel.AllowedOrigins = types.ListValueMust(types.StringType, origins)
		}

		if len(rule.ExposeHeader) > 0 {
			exposeHeaders := make([]attr.Value, len(rule.ExposeHeader))
			for i, h := range rule.ExposeHeader {
				exposeHeaders[i] = types.StringValue(h)
			}
			ruleModel.ExposeHeaders = types.ListValueMust(types.StringType, exposeHeaders)
		}

		if rule.MaxAgeSeconds > 0 {
			ruleModel.MaxAgeSeconds = types.Int64Value(int64(rule.MaxAgeSeconds))
		}

		obj, d := types.ObjectValue(corsRuleObjectType.AttrTypes, map[string]attr.Value{
			"id":              ruleModel.ID,
			"allowed_headers": ruleModel.AllowedHeaders,
			"allowed_methods": ruleModel.AllowedMethods,
			"allowed_origins": ruleModel.AllowedOrigins,
			"expose_headers":  ruleModel.ExposeHeaders,
			"max_age_seconds": ruleModel.MaxAgeSeconds,
		})
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(corsRuleObjectType), diags
		}
		result = append(result, obj)
	}

	return types.ListValue(corsRuleObjectType, result)
}

func newBucketCorsResource() resource.Resource {
	return &bucketCorsResource{}
}
