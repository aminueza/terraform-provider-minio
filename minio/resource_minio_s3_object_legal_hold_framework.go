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
	_ resource.Resource                = &s3ObjectLegalHoldResource{}
	_ resource.ResourceWithConfigure   = &s3ObjectLegalHoldResource{}
	_ resource.ResourceWithImportState = &s3ObjectLegalHoldResource{}
)

type s3ObjectLegalHoldResource struct {
	client *S3MinioClient
}

type s3ObjectLegalHoldResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Bucket    types.String `tfsdk:"bucket"`
	Key       types.String `tfsdk:"key"`
	VersionID types.String `tfsdk:"version_id"`
	Status    types.String `tfsdk:"status"`
}

func (r *s3ObjectLegalHoldResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_object_legal_hold"
}

func (r *s3ObjectLegalHoldResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages legal hold status for S3 objects in a MinIO bucket. The bucket must have object locking enabled.",
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
			"version_id": schema.StringAttribute{
				Description: "Version ID of the object",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Legal hold status: ON or OFF",
				Required:    true,
			},
		},
	}
}

func (r *s3ObjectLegalHoldResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *s3ObjectLegalHoldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data s3ObjectLegalHoldResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	status := minio.LegalHoldStatus(data.Status.ValueString())
	opts := minio.PutObjectLegalHoldOptions{
		Status:    &status,
		VersionID: data.VersionID.ValueString(),
	}

	if err := r.client.S3Client.PutObjectLegalHold(ctx, data.Bucket.ValueString(), data.Key.ValueString(), opts); err != nil {
		resp.Diagnostics.AddError("Creating object legal hold", err.Error())
		return
	}

	data.ID = types.StringValue(r.legalHoldID(data.Bucket.ValueString(), data.Key.ValueString(), data.VersionID.ValueString()))

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *s3ObjectLegalHoldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data s3ObjectLegalHoldResourceModel

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

func (r *s3ObjectLegalHoldResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data s3ObjectLegalHoldResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	status := minio.LegalHoldStatus(data.Status.ValueString())
	opts := minio.PutObjectLegalHoldOptions{
		Status:    &status,
		VersionID: data.VersionID.ValueString(),
	}

	if err := r.client.S3Client.PutObjectLegalHold(ctx, data.Bucket.ValueString(), data.Key.ValueString(), opts); err != nil {
		resp.Diagnostics.AddError("Updating object legal hold", err.Error())
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *s3ObjectLegalHoldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data s3ObjectLegalHoldResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	status := minio.LegalHoldDisabled
	opts := minio.PutObjectLegalHoldOptions{
		Status:    &status,
		VersionID: data.VersionID.ValueString(),
	}

	if err := r.client.S3Client.PutObjectLegalHold(ctx, data.Bucket.ValueString(), data.Key.ValueString(), opts); err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && (minioErr.Code == "NoSuchKey" || minioErr.Code == "NoSuchVersion") {
			return
		}
		resp.Diagnostics.AddError("Deleting object legal hold", err.Error())
		return
	}
}

func (r *s3ObjectLegalHoldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	bucket, key, versionID := r.parseLegalHoldID(req.ID)

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("bucket"), bucket)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), key)...)
	if versionID != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("version_id"), versionID)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *s3ObjectLegalHoldResource) read(ctx context.Context, data *s3ObjectLegalHoldResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	opts := minio.GetObjectLegalHoldOptions{
		VersionID: data.VersionID.ValueString(),
	}

	status, err := r.client.S3Client.GetObjectLegalHold(ctx, data.Bucket.ValueString(), data.Key.ValueString(), opts)
	if err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && (minioErr.Code == "NoSuchKey" || minioErr.Code == "NoSuchVersion") {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError("Reading object legal hold", err.Error())
		return diags
	}

	holdStatus := "OFF"
	if status != nil {
		holdStatus = string(*status)
	}
	data.Status = types.StringValue(holdStatus)

	if data.ID.IsNull() {
		data.ID = types.StringValue(r.legalHoldID(data.Bucket.ValueString(), data.Key.ValueString(), data.VersionID.ValueString()))
	}

	return diags
}

func (r *s3ObjectLegalHoldResource) legalHoldID(bucket, key, versionID string) string {
	id := fmt.Sprintf("%s/%s", bucket, key)
	if versionID != "" {
		id += "#" + versionID
	}
	return id
}

func (r *s3ObjectLegalHoldResource) parseLegalHoldID(id string) (bucket, key, versionID string) {
	rest := id
	if idx := strings.LastIndex(id, "#"); idx != -1 {
		rest = id[:idx]
		versionID = id[idx+1:]
	}
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 2 {
		bucket = parts[0]
		key = parts[1]
	}
	return bucket, key, versionID
}

func newS3ObjectLegalHoldResource() resource.Resource {
	return &s3ObjectLegalHoldResource{}
}
