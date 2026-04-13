package minio

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/minio-go/v7"
	"github.com/mitchellh/go-homedir"
)

var (
	_ resource.Resource                = &s3ObjectResource{}
	_ resource.ResourceWithConfigure   = &s3ObjectResource{}
	_ resource.ResourceWithImportState = &s3ObjectResource{}
)

type s3ObjectResource struct {
	client *S3MinioClient
}

type s3ObjectResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	BucketName         types.String `tfsdk:"bucket_name"`
	ObjectName         types.String `tfsdk:"object_name"`
	ContentType        types.String `tfsdk:"content_type"`
	Source             types.String `tfsdk:"source"`
	Content            types.String `tfsdk:"content"`
	ContentBase64      types.String `tfsdk:"content_base64"`
	ETag               types.String `tfsdk:"etag"`
	VersionID          types.String `tfsdk:"version_id"`
	ACL                types.String `tfsdk:"acl"`
	Metadata           types.Map    `tfsdk:"metadata"`
	CacheControl       types.String `tfsdk:"cache_control"`
	ContentDisposition types.String `tfsdk:"content_disposition"`
	ContentEncoding    types.String `tfsdk:"content_encoding"`
	Expires            types.String `tfsdk:"expires"`
	StorageClass       types.String `tfsdk:"storage_class"`
}

func (r *s3ObjectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_object"
}

func (r *s3ObjectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an S3 object (file) in a MinIO bucket.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket_name": schema.StringAttribute{
				Description: "Name of the bucket",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"object_name": schema.StringAttribute{
				Description: "Name of the object",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"content_type": schema.StringAttribute{
				Description: "Content type of the object, in the form of a MIME type",
				Optional:    true,
				Computed:    true,
			},
			"source": schema.StringAttribute{
				Description:   "Path to the file that will be uploaded. Use only one of content, content_base64, or source",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIfConfigured()},
			},
			"content": schema.StringAttribute{
				Description:   "Content of the object as a string. Use only one of content, content_base64, or source",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIfConfigured()},
			},
			"content_base64": schema.StringAttribute{
				Description:   "Base64-encoded content of the object. Use only one of content, content_base64, or source",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIfConfigured()},
			},
			"etag": schema.StringAttribute{
				Description: "ETag of the object",
				Optional:    true,
				Computed:    true,
			},
			"version_id": schema.StringAttribute{
				Description: "Version ID of the object",
				Optional:    true,
				Computed:    true,
			},
			"acl": schema.StringAttribute{
				Description: "The canned ACL to apply to the object. Valid values: private, public-read, public-read-write, authenticated-read",
				Optional:    true,
				Computed:    true,
			},
			"metadata": schema.MapAttribute{
				Description: "Metadata to store with the object",
				Optional:    true,
				ElementType: types.StringType,
			},
			"cache_control": schema.StringAttribute{
				Description: "Cache control header",
				Optional:    true,
			},
			"content_disposition": schema.StringAttribute{
				Description: "Content disposition header",
				Optional:    true,
			},
			"content_encoding": schema.StringAttribute{
				Description: "Content encoding header",
				Optional:    true,
				Computed:    true,
			},
			"expires": schema.StringAttribute{
				Description: "Expires header in RFC3339 format",
				Optional:    true,
			},
			"storage_class": schema.StringAttribute{
				Description: "Storage class: STANDARD, REDUCED_REDUNDANCY, ONEZONE_IA, INTELLIGENT_TIERING",
				Optional:    true,
				Computed:    true,
			},
		},
	}
}

func (r *s3ObjectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *s3ObjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data s3ObjectResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.putObject(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Creating object failed", err.Error())
		return
	}

	data.ID = data.ObjectName

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *s3ObjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data s3ObjectResourceModel

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

func (r *s3ObjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data s3ObjectResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.putObject(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Updating object failed", err.Error())
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *s3ObjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data s3ObjectResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.S3Client.RemoveObject(
		context.Background(),
		data.BucketName.ValueString(),
		data.ObjectName.ValueString(),
		minio.RemoveObjectOptions{},
	)

	if err != nil {
		resp.Diagnostics.AddError("Deleting object failed", err.Error())
		return
	}
}

func (r *s3ObjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Unexpected import ID format (%q), expected bucket_name/object_name", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("bucket_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

func (r *s3ObjectResource) putObject(ctx context.Context, data *s3ObjectResourceModel) error {
	var body io.ReadSeeker

	if !data.Source.IsNull() && !data.Source.IsUnknown() {
		source := data.Source.ValueString()
		path, err := homedir.Expand(source)
		if err != nil {
			return fmt.Errorf("expanding homedir in source (%s): %w", source, err)
		}
		path = filepath.Clean(path)
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening S3 object source (%s): %w", path, err)
		}
		defer file.Close()

		body = file
	} else if !data.Content.IsNull() && !data.Content.IsUnknown() {
		content := data.Content.ValueString()
		body = bytes.NewReader([]byte(content))
	} else if !data.ContentBase64.IsNull() && !data.ContentBase64.IsUnknown() {
		content := data.ContentBase64.ValueString()
		contentRaw, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return fmt.Errorf("error decoding content_base64: %w", err)
		}
		body = bytes.NewReader(contentRaw)
	} else {
		return errors.New("one of source, content, or content_base64 must be set")
	}

	options := minio.PutObjectOptions{}

	if !data.ContentType.IsNull() && !data.ContentType.IsUnknown() {
		options.ContentType = data.ContentType.ValueString()
	}
	if !data.CacheControl.IsNull() && !data.CacheControl.IsUnknown() {
		options.CacheControl = data.CacheControl.ValueString()
	}
	if !data.ContentDisposition.IsNull() && !data.ContentDisposition.IsUnknown() {
		options.ContentDisposition = data.ContentDisposition.ValueString()
	}
	if !data.ContentEncoding.IsNull() && !data.ContentEncoding.IsUnknown() {
		options.ContentEncoding = data.ContentEncoding.ValueString()
	}
	if !data.Expires.IsNull() && !data.Expires.IsUnknown() {
		t, err := time.Parse(time.RFC3339, data.Expires.ValueString())
		if err != nil {
			return fmt.Errorf("parsing expires: %w", err)
		}
		options.Expires = t
	}
	if !data.StorageClass.IsNull() && !data.StorageClass.IsUnknown() {
		options.StorageClass = data.StorageClass.ValueString()
	}

	if !data.Metadata.IsNull() && !data.Metadata.IsUnknown() {
		metadata := make(map[string]string)
		diags := data.Metadata.ElementsAs(ctx, &metadata, false)
		if diags.HasError() {
			return fmt.Errorf("reading metadata: %v", diags)
		}
		options.UserMetadata = metadata
	}

	if !data.ACL.IsNull() && !data.ACL.IsUnknown() && data.ACL.ValueString() != "" && data.ACL.ValueString() != "private" {
		if options.UserMetadata == nil {
			options.UserMetadata = make(map[string]string)
		}
		options.UserMetadata["x-amz-acl"] = data.ACL.ValueString()
	}

	_, err := r.client.S3Client.PutObject(
		ctx,
		data.BucketName.ValueString(),
		data.ObjectName.ValueString(),
		body, -1,
		options,
	)

	if err != nil {
		return fmt.Errorf("putting object failed: %w", err)
	}

	return nil
}

func (r *s3ObjectResource) read(ctx context.Context, data *s3ObjectResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	objInfo, err := r.client.S3Client.StatObject(
		ctx,
		data.BucketName.ValueString(),
		data.ObjectName.ValueString(),
		minio.StatObjectOptions{},
	)

	if err != nil {
		if strings.Contains(err.Error(), "The specified key does not exist.") {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError("Reading object failed", err.Error())
		return diags
	}

	data.ETag = types.StringValue(objInfo.ETag)
	data.VersionID = types.StringValue(objInfo.VersionID)
	data.ContentType = types.StringValue(objInfo.ContentType)

	if objInfo.ContentEncoding != "" {
		data.ContentEncoding = types.StringValue(objInfo.ContentEncoding)
	}
	if objInfo.StorageClass != "" {
		data.StorageClass = types.StringValue(objInfo.StorageClass)
	}

	if v := objInfo.Metadata.Get("Cache-Control"); v != "" {
		data.CacheControl = types.StringValue(v)
	}
	if v := objInfo.Metadata.Get("Content-Disposition"); v != "" {
		data.ContentDisposition = types.StringValue(v)
	}
	if !objInfo.Expires.IsZero() {
		data.Expires = types.StringValue(objInfo.Expires.Format(time.RFC3339))
	}

	userMeta := make(map[string]string)
	for k, v := range objInfo.UserMetadata {
		lower := strings.ToLower(k)
		if lower == "x-amz-acl" || lower == "content-type" {
			continue
		}
		userMeta[k] = v
	}
	if len(userMeta) > 0 {
		metadata, mapDiags := types.MapValueFrom(ctx, types.StringType, userMeta)
		diags.Append(mapDiags...)
		if diags.HasError() {
			return diags
		}
		data.Metadata = metadata
	}

	return diags
}

func newS3ObjectResource() resource.Resource {
	return &s3ObjectResource{}
}
