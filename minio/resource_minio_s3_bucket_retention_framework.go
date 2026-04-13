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
	"github.com/minio/minio-go/v7"
)

var (
	_ resource.Resource                = &bucketRetentionResource{}
	_ resource.ResourceWithConfigure   = &bucketRetentionResource{}
	_ resource.ResourceWithImportState = &bucketRetentionResource{}
)

type bucketRetentionResource struct {
	client *S3MinioClient
}

type bucketRetentionResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Bucket         types.String `tfsdk:"bucket"`
	Mode           types.String `tfsdk:"mode"`
	Unit           types.String `tfsdk:"unit"`
	ValidityPeriod types.Int64  `tfsdk:"validity_period"`
}

func (r *bucketRetentionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket_retention"
}

func (r *bucketRetentionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: `Manages object lock retention settings for a MinIO bucket. Object locking enforces Write-Once Read-Many (WORM) immutability to protect versioned objects from deletion.

Note: Object locking can only be enabled during bucket creation and requires versioning. You cannot enable object locking on an existing bucket.

This resource provides compliance with SEC17a-4(f), FINRA 4511(C), and CFTC 1.31(c)-(d) requirements.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Bucket name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Description: "Name of the bucket to configure object locking. The bucket must be created with object locking enabled.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mode": schema.StringAttribute{
				Description: `Retention mode for the bucket. Valid values are:
- GOVERNANCE: Prevents object modification by non-privileged users. Users with s3:BypassGovernanceRetention permission can modify objects.
- COMPLIANCE: Prevents any object modification by all users, including the root user, until retention period expires.`,
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("GOVERNANCE", "COMPLIANCE"),
				},
			},
			"unit": schema.StringAttribute{
				Description: "Time unit for the validity period. Valid values are DAYS or YEARS.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("DAYS", "YEARS"),
				},
			},
			"validity_period": schema.Int64Attribute{
				Description: "Duration for which objects should be retained under WORM lock, in the specified unit. Must be a positive integer.",
				Required:    true,
				Validators:  []validator.Int64{int64validator.AtLeast(1)},
			},
		},
	}
}

func (r *bucketRetentionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *bucketRetentionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data bucketRetentionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.validateObjectLock(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setRetention(ctx, &data)...)
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

func (r *bucketRetentionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data bucketRetentionResourceModel

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

func (r *bucketRetentionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data bucketRetentionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.validateObjectLock(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setRetention(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketRetentionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data bucketRetentionResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.S3Client.SetBucketObjectLockConfig(ctx, data.Bucket.ValueString(), nil, nil, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error clearing bucket object lock config",
			err.Error(),
		)
		return
	}
}

func (r *bucketRetentionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("bucket"), req, resp)
}

func (r *bucketRetentionResource) validateObjectLock(ctx context.Context, data *bucketRetentionResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	exists, err := r.client.S3Client.BucketExists(ctx, data.Bucket.ValueString())
	if err != nil {
		diags.AddError("Error checking bucket existence", err.Error())
		return diags
	}
	if !exists {
		diags.AddError("Bucket does not exist", fmt.Sprintf("bucket %s does not exist", data.Bucket.ValueString()))
		return diags
	}

	versioning, err := r.client.S3Client.GetBucketVersioning(ctx, data.Bucket.ValueString())
	if err != nil {
		diags.AddError("Error checking bucket versioning", err.Error())
		return diags
	}

	if !versioning.Enabled() {
		diags.AddError("Versioning not enabled", fmt.Sprintf("bucket %s does not have versioning enabled. Object locking requires versioning", data.Bucket.ValueString()))
		return diags
	}

	objectLock, _, _, _, err := r.client.S3Client.GetObjectLockConfig(ctx, data.Bucket.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "Object Lock configuration does not exist") {
			diags.AddError("Object lock not enabled", fmt.Sprintf("bucket %s does not have object lock enabled. Object lock must be enabled when creating the bucket", data.Bucket.ValueString()))
			return diags
		}
		diags.AddError("Error checking object lock configuration", err.Error())
		return diags
	}

	if objectLock != "Enabled" {
		diags.AddError("Object lock not enabled", fmt.Sprintf("bucket %s does not have object lock enabled. Object lock must be enabled when creating the bucket", data.Bucket.ValueString()))
		return diags
	}

	return diags
}

func (r *bucketRetentionResource) setRetention(ctx context.Context, data *bucketRetentionResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	mode := minio.RetentionMode(data.Mode.ValueString())
	unit := minio.ValidityUnit(data.Unit.ValueString())
	validity := uint(data.ValidityPeriod.ValueInt64())

	err := r.client.S3Client.SetBucketObjectLockConfig(ctx, data.Bucket.ValueString(), &mode, &validity, &unit)
	if err != nil {
		diags.AddError("Error setting bucket object lock config", err.Error())
		return diags
	}

	return diags
}

func (r *bucketRetentionResource) read(ctx context.Context, data *bucketRetentionResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	exists, err := r.client.S3Client.BucketExists(ctx, data.Bucket.ValueString())
	if err != nil {
		diags.AddError("Error checking bucket existence", err.Error())
		return diags
	}
	if !exists {
		data.ID = types.StringNull()
		return diags
	}

	mode, validity, unit, err := r.client.S3Client.GetBucketObjectLockConfig(ctx, data.Bucket.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "Object Lock configuration does not exist") {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError("Error reading bucket retention config", err.Error())
		return diags
	}

	if mode == nil || validity == nil || unit == nil {
		data.ID = types.StringNull()
		return diags
	}

	data.Mode = types.StringValue(mode.String())
	data.Unit = types.StringValue(unit.String())
	data.ValidityPeriod = types.Int64Value(int64(*validity))

	if data.ID.IsNull() {
		data.ID = data.Bucket
	}

	return diags
}

func newBucketRetentionResource() resource.Resource {
	return &bucketRetentionResource{}
}
