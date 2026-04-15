package minio

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	minio "github.com/minio/minio-go/v7"
)

var (
	_ resource.Resource                = &bucketObjectLockConfigurationResource{}
	_ resource.ResourceWithConfigure   = &bucketObjectLockConfigurationResource{}
	_ resource.ResourceWithImportState = &bucketObjectLockConfigurationResource{}
)

type bucketObjectLockConfigurationResource struct {
	client *S3MinioClient
}

type bucketObjectLockConfigurationResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Bucket            types.String `tfsdk:"bucket"`
	ObjectLockEnabled types.String `tfsdk:"object_lock_enabled"`
	Rule              []ruleModel  `tfsdk:"rule"`
}

type ruleModel struct {
	DefaultRetention []defaultRetentionModel `tfsdk:"default_retention"`
}

type defaultRetentionModel struct {
	Mode  types.String `tfsdk:"mode"`
	Days  types.Int64  `tfsdk:"days"`
	Years types.Int64  `tfsdk:"years"`
}

func (r *bucketObjectLockConfigurationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket_object_lock_configuration"
}

func (r *bucketObjectLockConfigurationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: `Configures object lock (WORM) retention policies at the bucket level. Sets default retention that applies to all new objects automatically.

Object locking must be enabled when creating the bucket - can't add it later unless you're on MinIO RELEASE.2025-05-20T20-30-00Z+.

Useful for compliance: SEC17a-4(f), FINRA 4511(C), CFTC 1.31(c)-(d)`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Bucket name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Description: "Bucket name. Must have object locking enabled at creation time.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"object_lock_enabled": schema.StringAttribute{
				Description: "Object lock status. Only valid value is 'Enabled'.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("Enabled"),
				Validators: []validator.String{
					stringvalidator.OneOf("Enabled"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"rule": schema.ListNestedAttribute{
				Description: "Retention rule configuration",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"default_retention": schema.ListNestedAttribute{
							Description: "Default retention applied to all new objects",
							Optional:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"mode": schema.StringAttribute{
										Description: "GOVERNANCE (bypassable with permissions) or COMPLIANCE (strict, no overrides).",
										Required:    true,
										Validators: []validator.String{
											stringvalidator.OneOf("GOVERNANCE", "COMPLIANCE"),
										},
									},
									"days": schema.Int64Attribute{
										Description: "Retention period in days. Mutually exclusive with years.",
										Optional:    true,
										Validators:  []validator.Int64{int64validator.AtLeast(1), int64validator.ConflictsWith(path.MatchRelative().AtParent().AtName("years"))},
									},
									"years": schema.Int64Attribute{
										Description: "Retention period in years. Mutually exclusive with days.",
										Optional:    true,
										Validators:  []validator.Int64{int64validator.AtLeast(1), int64validator.ConflictsWith(path.MatchRelative().AtParent().AtName("days"))},
									},
								},
							},
							PlanModifiers: []planmodifier.List{
								listplanmodifier.RequiresReplace(),
							},
						},
					},
				},
			},
		},
	}
}

func (r *bucketObjectLockConfigurationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *bucketObjectLockConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data bucketObjectLockConfigurationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.validatePrerequisites(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.applyConfiguration(ctx, &data)...)
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

func (r *bucketObjectLockConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data bucketObjectLockConfigurationResourceModel

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

func (r *bucketObjectLockConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data bucketObjectLockConfigurationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.validatePrerequisites(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.applyConfiguration(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketObjectLockConfigurationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data bucketObjectLockConfigurationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.S3Client.SetBucketObjectLockConfig(ctx, data.Bucket.ValueString(), nil, nil, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error clearing object lock configuration",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketObjectLockConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("bucket"), req, resp)
}

func (r *bucketObjectLockConfigurationResource) validatePrerequisites(ctx context.Context, data *bucketObjectLockConfigurationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	exists, err := r.client.S3Client.BucketExists(ctx, data.Bucket.ValueString())
	if err != nil {
		diags.AddError(
			"Error checking bucket existence",
			err.Error(),
		)
		return diags
	}
	if !exists {
		diags.AddError(
			"Bucket does not exist",
			fmt.Sprintf("bucket %s does not exist", data.Bucket.ValueString()),
		)
		return diags
	}

	versioning, err := r.client.S3Client.GetBucketVersioning(ctx, data.Bucket.ValueString())
	if err != nil {
		diags.AddError(
			"Error checking bucket versioning",
			err.Error(),
		)
		return diags
	}

	if !versioning.Enabled() {
		diags.AddError(
			"Versioning not enabled",
			fmt.Sprintf("bucket %s does not have versioning enabled. Object locking requires versioning", data.Bucket.ValueString()),
		)
		return diags
	}

	objectLockStatus, _, _, _, err := r.client.S3Client.GetObjectLockConfig(ctx, data.Bucket.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "Object Lock configuration does not exist") {
			diags.AddError(
				"Object lock not enabled",
				fmt.Sprintf("bucket %s doesn't have object lock enabled (must be set at bucket creation)", data.Bucket.ValueString()),
			)
			return diags
		}
		diags.AddError(
			"Error checking object lock configuration",
			err.Error(),
		)
		return diags
	}

	if objectLockStatus != "Enabled" {
		diags.AddError(
			"Object lock not enabled",
			fmt.Sprintf("bucket %s doesn't have object lock enabled", data.Bucket.ValueString()),
		)
		return diags
	}

	return diags
}

func (r *bucketObjectLockConfigurationResource) applyConfiguration(ctx context.Context, data *bucketObjectLockConfigurationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	if data.Rule == nil {
		err := r.client.S3Client.SetBucketObjectLockConfig(ctx, data.Bucket.ValueString(), nil, nil, nil)
		if err != nil {
			diags.AddError(
				"Error clearing object lock configuration",
				err.Error(),
			)
			return diags
		}
		return diags
	}

	if len(data.Rule) == 0 || len(data.Rule[0].DefaultRetention) == 0 {
		err := r.client.S3Client.SetBucketObjectLockConfig(ctx, data.Bucket.ValueString(), nil, nil, nil)
		if err != nil {
			diags.AddError(
				"Error clearing object lock configuration",
				err.Error(),
			)
			return diags
		}
		return diags
	}

	retention := data.Rule[0].DefaultRetention[0]
	mode := minio.RetentionMode(retention.Mode.ValueString())

	var validity uint
	var unit minio.ValidityUnit

	if !retention.Days.IsNull() {
		days := retention.Days.ValueInt64()
		if days < 0 || days > math.MaxInt {
			diags.AddError(
				"Invalid days value",
				fmt.Sprintf("days value %d is out of valid range", days),
			)
			return diags
		}
		validity = uint(days) // #nosec G115 -- bounds checked above
		unit = minio.Days
	} else if !retention.Years.IsNull() {
		years := retention.Years.ValueInt64()
		if years < 0 || years > math.MaxInt {
			diags.AddError(
				"Invalid years value",
				fmt.Sprintf("years value %d is out of valid range", years),
			)
			return diags
		}
		validity = uint(years) // #nosec G115 -- bounds checked above
		unit = minio.Years
	} else {
		diags.AddError(
			"Missing retention period",
			"either days or years must be specified in default_retention",
		)
		return diags
	}

	err := r.client.S3Client.SetBucketObjectLockConfig(ctx, data.Bucket.ValueString(), &mode, &validity, &unit)
	if err != nil {
		diags.AddError(
			"Error setting object lock configuration",
			err.Error(),
		)
		return diags
	}

	return diags
}

func (r *bucketObjectLockConfigurationResource) read(ctx context.Context, data *bucketObjectLockConfigurationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	exists, err := r.client.S3Client.BucketExists(ctx, data.Bucket.ValueString())
	if err != nil {
		diags.AddError(
			"Error checking bucket existence",
			err.Error(),
		)
		return diags
	}
	if !exists {
		data.ID = types.StringNull()
		return diags
	}

	objectLockStatus, mode, validity, unit, err := r.client.S3Client.GetObjectLockConfig(ctx, data.Bucket.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "Object Lock configuration does not exist") {
			data.ID = types.StringNull()
			return diags
		}
		if isS3CompatNotSupported(r.client, err) {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError(
			"Error reading object lock configuration",
			err.Error(),
		)
		return diags
	}

	data.ObjectLockEnabled = types.StringValue(objectLockStatus)

	if mode != nil && validity != nil && unit != nil {
		defaultRetention := defaultRetentionModel{
			Mode: types.StringValue(mode.String()),
		}

		validityInt := int64(math.MaxInt)
		if *validity <= uint(math.MaxInt) {
			validityInt = int64(*validity)
		}

		switch *unit {
		case minio.Days:
			defaultRetention.Days = types.Int64Value(validityInt)
		case minio.Years:
			defaultRetention.Years = types.Int64Value(validityInt)
		}

		data.Rule = []ruleModel{
			{
				DefaultRetention: []defaultRetentionModel{defaultRetention},
			},
		}
	}

	if data.ID.IsNull() {
		data.ID = data.Bucket
	}

	return diags
}

func newBucketObjectLockConfigurationResource() resource.Resource {
	return &bucketObjectLockConfigurationResource{}
}
