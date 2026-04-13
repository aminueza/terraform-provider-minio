package minio

import (
	"context"
	"errors"
	"fmt"
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
)

var (
	_ resource.Resource                = &s3ObjectRetentionResource{}
	_ resource.ResourceWithConfigure   = &s3ObjectRetentionResource{}
	_ resource.ResourceWithImportState = &s3ObjectRetentionResource{}
)

type s3ObjectRetentionResource struct {
	client *S3MinioClient
}

type s3ObjectRetentionResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Bucket           types.String `tfsdk:"bucket"`
	Key              types.String `tfsdk:"key"`
	VersionID        types.String `tfsdk:"version_id"`
	Mode             types.String `tfsdk:"mode"`
	RetainUntilDate  types.String `tfsdk:"retain_until_date"`
	GovernanceBypass types.Bool   `tfsdk:"governance_bypass"`
}

func (r *s3ObjectRetentionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_object_retention"
}

func (r *s3ObjectRetentionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages retention policy for individual S3 objects. The bucket must have object locking enabled.",
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
			"mode": schema.StringAttribute{
				Description: "Retention mode: GOVERNANCE or COMPLIANCE",
				Required:    true,
			},
			"retain_until_date": schema.StringAttribute{
				Description: "Date until which the object is retained (RFC3339 format)",
				Required:    true,
			},
			"governance_bypass": schema.BoolAttribute{
				Description: "Allow bypassing governance mode retention",
				Optional:    true,
				Computed:    true,
			},
		},
	}
}

func (r *s3ObjectRetentionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *s3ObjectRetentionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data s3ObjectRetentionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setRetention(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Creating object retention", err.Error())
		return
	}

	data.ID = types.StringValue(r.retentionID(data.Bucket.ValueString(), data.Key.ValueString(), data.VersionID.ValueString()))

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *s3ObjectRetentionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data s3ObjectRetentionResourceModel

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

func (r *s3ObjectRetentionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data s3ObjectRetentionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setRetention(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Updating object retention", err.Error())
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *s3ObjectRetentionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retention is cleared by setting the resource to not exist
	return
}

func (r *s3ObjectRetentionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	bucket, key, versionID := r.parseRetentionID(req.ID)

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("bucket"), bucket)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), key)...)
	if versionID != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("version_id"), versionID)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *s3ObjectRetentionResource) read(ctx context.Context, data *s3ObjectRetentionResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	mode, retainUntil, err := r.client.S3Client.GetObjectRetention(ctx, data.Bucket.ValueString(), data.Key.ValueString(), data.VersionID.ValueString())
	if err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && (minioErr.Code == "NoSuchKey" || minioErr.Code == "NoSuchObjectLockConfiguration") {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError("Reading object retention", err.Error())
		return diags
	}

	if mode != nil {
		data.Mode = types.StringValue(string(*mode))
	}
	if retainUntil != nil {
		data.RetainUntilDate = types.StringValue(retainUntil.Format(time.RFC3339))
	}

	if data.ID.IsNull() {
		data.ID = types.StringValue(r.retentionID(data.Bucket.ValueString(), data.Key.ValueString(), data.VersionID.ValueString()))
	}

	return diags
}

func (r *s3ObjectRetentionResource) setRetention(ctx context.Context, data *s3ObjectRetentionResourceModel) error {
	mode := minio.RetentionMode(data.Mode.ValueString())
	t, err := time.Parse(time.RFC3339, data.RetainUntilDate.ValueString())
	if err != nil {
		return fmt.Errorf("parsing retain_until_date: %w", err)
	}

	opts := minio.PutObjectRetentionOptions{
		GovernanceBypass: data.GovernanceBypass.ValueBool(),
		Mode:             &mode,
		RetainUntilDate:  &t,
		VersionID:        data.VersionID.ValueString(),
	}

	if err := r.client.S3Client.PutObjectRetention(ctx, data.Bucket.ValueString(), data.Key.ValueString(), opts); err != nil {
		return fmt.Errorf("setting object retention: %w", err)
	}

	return nil
}

func (r *s3ObjectRetentionResource) retentionID(bucket, key, versionID string) string {
	id := fmt.Sprintf("%s/%s", bucket, key)
	if versionID != "" {
		id += "#" + versionID
	}
	return id
}

func (r *s3ObjectRetentionResource) parseRetentionID(id string) (bucket, key, versionID string) {
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

func newS3ObjectRetentionResource() resource.Resource {
	return &s3ObjectRetentionResource{}
}
