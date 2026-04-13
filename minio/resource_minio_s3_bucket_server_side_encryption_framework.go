package minio

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/minio-go/v7/pkg/sse"
)

// Ensure provider defined types fully satisfy framework interfaces
var (
	_ resource.Resource                = &bucketEncryptionResource{}
	_ resource.ResourceWithConfigure   = &bucketEncryptionResource{}
	_ resource.ResourceWithImportState = &bucketEncryptionResource{}
)

// bucketEncryptionResource defines the resource implementation
type bucketEncryptionResource struct {
	client *S3MinioClient
}

// bucketEncryptionResourceModel describes the resource data model
type bucketEncryptionResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Bucket         types.String `tfsdk:"bucket"`
	EncryptionType types.String `tfsdk:"encryption_type"`
	KMSKeyID       types.String `tfsdk:"kms_key_id"`
}

func (r *bucketEncryptionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket_server_side_encryption_configuration"
}

func (r *bucketEncryptionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages server-side encryption configuration for an S3 bucket. Supports SSE-S3 (AES256) and SSE-KMS (aws:kms) encryption types.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Bucket name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Description: "Name of the bucket on which to setup server side encryption.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"encryption_type": schema.StringAttribute{
				Description: "Server side encryption type: `AES256` for SSE-S3 or `aws:kms` for SSE-KMS.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("aws:kms", "AES256"),
				},
			},
			"kms_key_id": schema.StringAttribute{
				Description: "KMS key id to use for SSE-KMS encryption. Required when encryption_type is `aws:kms`, ignored for `AES256`.",
				Optional:    true,
			},
		},
	}
}

func (r *bucketEncryptionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *bucketEncryptionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data bucketEncryptionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate kms_key_id is required for aws:kms
	if data.EncryptionType.ValueString() == "aws:kms" && data.KMSKeyID.IsNull() {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"kms_key_id is required when encryption_type is \"aws:kms\"",
		)
		return
	}

	resp.Diagnostics.Append(r.putEncryption(ctx, &data)...)
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

func (r *bucketEncryptionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data bucketEncryptionResourceModel

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

func (r *bucketEncryptionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data bucketEncryptionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate kms_key_id is required for aws:kms
	if data.EncryptionType.ValueString() == "aws:kms" && data.KMSKeyID.IsNull() {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"kms_key_id is required when encryption_type is \"aws:kms\"",
		)
		return
	}

	resp.Diagnostics.Append(r.putEncryption(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketEncryptionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data bucketEncryptionResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Remove encryption
	err := r.client.S3Client.RemoveBucketEncryption(ctx, data.Bucket.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error removing bucket encryption",
			err.Error(),
		)
		return
	}

	// Clear ID
	data.ID = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketEncryptionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("bucket"), req, resp)
}

func (r *bucketEncryptionResource) putEncryption(ctx context.Context, data *bucketEncryptionResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	encryptionType := data.EncryptionType.ValueString()
	var encryptionConfig *sse.Configuration

	if encryptionType == "AES256" {
		encryptionConfig = sse.NewConfigurationSSES3()
	} else {
		keyID := data.KMSKeyID.ValueString()
		encryptionConfig = sse.NewConfigurationSSEKMS(keyID)
	}

	err := r.client.S3Client.SetBucketEncryption(ctx, data.Bucket.ValueString(), encryptionConfig)
	if err != nil {
		diags.AddError(
			"Error putting bucket encryption configuration",
			err.Error(),
		)
		return diags
	}

	return diags
}

func (r *bucketEncryptionResource) read(ctx context.Context, data *bucketEncryptionResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	encryptionConfig, err := r.client.S3Client.GetBucketEncryption(ctx, data.Bucket.ValueString())
	if err != nil {
		data.ID = types.StringNull()
		return diags
	}

	if len(encryptionConfig.Rules) == 0 {
		data.ID = types.StringNull()
		return diags
	}

	data.EncryptionType = types.StringValue(encryptionConfig.Rules[0].Apply.SSEAlgorithm)
	data.KMSKeyID = types.StringValue(encryptionConfig.Rules[0].Apply.KmsMasterKeyID)

	if data.ID.IsNull() {
		data.ID = data.Bucket
	}

	return diags
}

// newBucketEncryptionResource creates a new bucket encryption resource instance
func newBucketEncryptionResource() resource.Resource {
	return &bucketEncryptionResource{}
}
