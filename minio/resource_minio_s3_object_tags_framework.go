package minio

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/minio-go/v7"
)

var (
	_ resource.Resource                = &s3ObjectTagsResource{}
	_ resource.ResourceWithConfigure   = &s3ObjectTagsResource{}
	_ resource.ResourceWithImportState = &s3ObjectTagsResource{}
)

type s3ObjectTagsResource struct {
	client *S3MinioClient
}

type s3ObjectTagsResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Bucket types.String `tfsdk:"bucket"`
	Key    types.String `tfsdk:"key"`
	Tags   types.Map    `tfsdk:"tags"`
}

func (r *s3ObjectTagsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_object_tags"
}

func (r *s3ObjectTagsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages tags for S3 objects in a MinIO bucket.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
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
			"key": schema.StringAttribute{
				Description: "Object key",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tags": schema.MapAttribute{
				Description: "Map of tags to assign to the object",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *s3ObjectTagsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *s3ObjectTagsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data s3ObjectTagsResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.Tags.IsNull() && !data.Tags.IsUnknown() {
		if err := r.setObjectTags(ctx, &data); err != nil {
			resp.Diagnostics.AddError("Creating object tags", err.Error())
			return
		}
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.Bucket.ValueString(), data.Key.ValueString()))

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *s3ObjectTagsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data s3ObjectTagsResourceModel

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

func (r *s3ObjectTagsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data s3ObjectTagsResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setObjectTags(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Updating object tags", err.Error())
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *s3ObjectTagsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data s3ObjectTagsResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.deleteObjectTags(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Deleting object tags", err.Error())
		return
	}
}

func (r *s3ObjectTagsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Unexpected import ID format (%q), expected bucket/key", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("bucket"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *s3ObjectTagsResource) read(ctx context.Context, data *s3ObjectTagsResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	opts := minio.GetObjectTaggingOptions{}
	objectTags, err := r.client.S3Client.GetObjectTagging(ctx, data.Bucket.ValueString(), data.Key.ValueString(), opts)
	if err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && minioErr.Code == "NoSuchTagSet" {
			data.Tags = types.MapNull(types.StringType)
			return diags
		}
		diags.AddError("Reading object tags", err.Error())
		return diags
	}

	tagsMap := objectTags.ToMap()
	tags, mapDiags := types.MapValueFrom(ctx, types.StringType, tagsMap)
	diags.Append(mapDiags...)
	if diags.HasError() {
		return diags
	}
	data.Tags = tags

	return diags
}

func (r *s3ObjectTagsResource) setObjectTags(ctx context.Context, data *s3ObjectTagsResourceModel) error {
	var tagsMap map[string]string
	if !data.Tags.IsNull() && !data.Tags.IsUnknown() {
		diags := data.Tags.ElementsAs(ctx, &tagsMap, false)
		if diags.HasError() {
			return fmt.Errorf("reading tags: %v", diags)
		}
	}

	srcOpts := minio.CopySrcOptions{
		Bucket: data.Bucket.ValueString(),
		Object: data.Key.ValueString(),
	}

	dstOpts := minio.CopyDestOptions{
		Bucket:      data.Bucket.ValueString(),
		Object:      data.Key.ValueString(),
		UserTags:    tagsMap,
		ReplaceTags: true,
	}

	if _, err := r.client.S3Client.CopyObject(ctx, dstOpts, srcOpts); err != nil {
		return fmt.Errorf("copying object: %w", err)
	}

	return nil
}

func (r *s3ObjectTagsResource) deleteObjectTags(ctx context.Context, data *s3ObjectTagsResourceModel) error {
	srcOpts := minio.CopySrcOptions{
		Bucket: data.Bucket.ValueString(),
		Object: data.Key.ValueString(),
	}

	dstOpts := minio.CopyDestOptions{
		Bucket:      data.Bucket.ValueString(),
		Object:      data.Key.ValueString(),
		ReplaceTags: true,
	}

	if _, err := r.client.S3Client.CopyObject(ctx, dstOpts, srcOpts); err != nil {
		return fmt.Errorf("copying object: %w", err)
	}

	return nil
}

func newS3ObjectTagsResource() resource.Resource {
	return &s3ObjectTagsResource{}
}
