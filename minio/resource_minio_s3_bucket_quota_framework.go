package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/madmin-go/v3"
)

var (
	_ resource.Resource                = &bucketQuotaResource{}
	_ resource.ResourceWithConfigure   = &bucketQuotaResource{}
	_ resource.ResourceWithImportState = &bucketQuotaResource{}
)

type bucketQuotaResource struct {
	client *S3MinioClient
}

type bucketQuotaResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Bucket types.String `tfsdk:"bucket"`
	Quota  types.Int64  `tfsdk:"quota"`
	Type   types.String `tfsdk:"type"`
}

func (r *bucketQuotaResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket_quota"
}

func (r *bucketQuotaResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages quota limits for S3 buckets in MinIO.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Bucket name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Description: "Name of the bucket",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"quota": schema.Int64Attribute{
				Description: "Quota size in bytes",
				Required:    true,
				Validators:  []validator.Int64{int64validator.AtLeast(1)},
			},
			"type": schema.StringAttribute{
				Description: "Quota type (only \"hard\" is supported)",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("hard"),
				},
			},
		},
	}
}

func (r *bucketQuotaResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *bucketQuotaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data bucketQuotaResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setQuota(ctx, &data)...)
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

func (r *bucketQuotaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data bucketQuotaResourceModel

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

func (r *bucketQuotaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data bucketQuotaResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setQuota(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketQuotaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data bucketQuotaResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bucketQuota := madmin.BucketQuota{Quota: 0}
	err := r.client.S3Admin.SetBucketQuota(ctx, data.Bucket.ValueString(), &bucketQuota)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error clearing bucket quota",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketQuotaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("bucket"), req, resp)
}

func (r *bucketQuotaResource) setQuota(ctx context.Context, data *bucketQuotaResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	quotaVal := data.Quota.ValueInt64()
	if quotaVal < 1 {
		diags.AddError(
			"Invalid quota value",
			fmt.Sprintf("quota must be a positive value, got: %d", quotaVal),
		)
		return diags
	}

	bucketQuota := madmin.BucketQuota{Quota: uint64(quotaVal), Type: madmin.HardQuota}
	err := r.client.S3Admin.SetBucketQuota(ctx, data.Bucket.ValueString(), &bucketQuota)
	if err != nil {
		diags.AddError(
			"Error setting bucket quota",
			err.Error(),
		)
		return diags
	}

	return diags
}

func (r *bucketQuotaResource) read(ctx context.Context, data *bucketQuotaResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	bucketQuota, err := r.client.S3Admin.GetBucketQuota(ctx, data.Bucket.ValueString())
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such bucket") ||
			strings.Contains(err.Error(), "does not exist") {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError(
			"Error reading bucket quota",
			err.Error(),
		)
		return diags
	}

	if bucketQuota.Quota == 0 {
		data.ID = types.StringNull()
		return diags
	}

	quotaVal, ok := SafeUint64ToInt64(bucketQuota.Quota)
	if !ok {
		diags.AddError(
			"Error reading bucket quota",
			fmt.Sprintf("quota value overflows int64: %d", bucketQuota.Quota),
		)
		return diags
	}

	data.Quota = types.Int64Value(quotaVal)
	data.Type = types.StringValue(string(bucketQuota.Type))

	if data.ID.IsNull() {
		data.ID = data.Bucket
	}

	return diags
}

func newBucketQuotaResource() resource.Resource {
	return &bucketQuotaResource{}
}
