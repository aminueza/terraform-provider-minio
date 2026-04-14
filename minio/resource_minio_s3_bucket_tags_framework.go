package minio

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/tags"
)

var (
	_ resource.Resource                = &bucketTagsResource{}
	_ resource.ResourceWithConfigure   = &bucketTagsResource{}
	_ resource.ResourceWithImportState = &bucketTagsResource{}
)

type bucketTagsResource struct {
	client *S3MinioClient
}

type bucketTagsResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Bucket types.String `tfsdk:"bucket"`
	Tags   types.Map    `tfsdk:"tags"`
}

func (r *bucketTagsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket_tags"
}

func (r *bucketTagsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages tags for S3 buckets in MinIO.",
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
			"tags": schema.MapAttribute{
				Description: "Map of tags to assign to the bucket",
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *bucketTagsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *bucketTagsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data bucketTagsResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setTags(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = data.Bucket

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketTagsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data bucketTagsResourceModel

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

func (r *bucketTagsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data bucketTagsResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setTags(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketTagsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data bucketTagsResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg := &S3MinioBucket{
		SkipBucketTagging: r.client.SkipBucketTagging,
	}

	if shouldSkipBucketTagging(cfg) {
		return
	}

	err := r.client.S3Client.RemoveBucketTagging(ctx, data.Bucket.ValueString())
	if err != nil {
		if !IsS3TaggingNotImplemented(err) {
			resp.Diagnostics.AddError(
				"Error removing bucket tags",
				err.Error(),
			)
			return
		}
	}
}

func (r *bucketTagsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("bucket"), req, resp)
}

func (r *bucketTagsResource) setTags(ctx context.Context, data *bucketTagsResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	cfg := &S3MinioBucket{
		SkipBucketTagging: r.client.SkipBucketTagging,
	}

	if shouldSkipBucketTagging(cfg) {
		return diags
	}

	if data.Tags.IsNull() || data.Tags.IsUnknown() {
		return diags
	}

	var tagsMap map[string]string
	diags.Append(data.Tags.ElementsAs(ctx, &tagsMap, false)...)
	if diags.HasError() {
		return diags
	}

	if len(tagsMap) == 0 {
		return diags
	}

	bucketTags, err := tags.NewTags(tagsMap, false)
	if err != nil {
		diags.AddError(
			"Error creating bucket tags",
			err.Error(),
		)
		return diags
	}

	err = r.client.S3Client.SetBucketTagging(ctx, data.Bucket.ValueString(), bucketTags)
	if err != nil {
		if !IsS3TaggingNotImplemented(err) {
			diags.AddError(
				"Error setting bucket tags",
				err.Error(),
			)
			return diags
		}
	}

	return diags
}

func (r *bucketTagsResource) read(ctx context.Context, data *bucketTagsResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	cfg := &S3MinioBucket{
		SkipBucketTagging: r.client.SkipBucketTagging,
	}

	if shouldSkipBucketTagging(cfg) {
		return diags
	}

	bucketTags, err := r.client.S3Client.GetBucketTagging(ctx, data.Bucket.ValueString())
	if err != nil {
		if isNoSuchBucketError(err) {
			data.ID = types.StringNull()
			return diags
		}
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && minioErr.Code == "NoSuchTagSet" {
			data.Tags = types.MapNull(types.StringType)
			return diags
		}
		if IsS3TaggingNotImplemented(err) {
			return diags
		}
		diags.AddError(
			"Error reading bucket tags",
			err.Error(),
		)
		return diags
	}

	tagsMap, diags := types.MapValueFrom(ctx, types.StringType, bucketTags.ToMap())
	if diags.HasError() {
		return diags
	}

	data.Tags = tagsMap

	if data.ID.IsNull() {
		data.ID = data.Bucket
	}

	return diags
}

func newBucketTagsResource() resource.Resource {
	return &bucketTagsResource{}
}
