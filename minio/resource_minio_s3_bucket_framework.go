package minio

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/s3utils"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &minioS3BucketResource{}
var _ resource.ResourceWithConfigure = &minioS3BucketResource{}
var _ resource.ResourceWithImportState = &minioS3BucketResource{}

// minioS3BucketResource defines the resource implementation
type minioS3BucketResource struct {
	client *S3MinioClient
}

// minioS3BucketResourceModel describes the resource data model
type minioS3BucketResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Bucket           types.String `tfsdk:"bucket"`
	BucketPrefix     types.String `tfsdk:"bucket_prefix"`
	ForceDestroy     types.Bool   `tfsdk:"force_destroy"`
	ACL              types.String `tfsdk:"acl"`
	ARN              types.String `tfsdk:"arn"`
	BucketDomainName types.String `tfsdk:"bucket_domain_name"`
	Quota            types.Int64  `tfsdk:"quota"`
	ObjectLocking    types.Bool   `tfsdk:"object_locking"`
	Tags             types.Map    `tfsdk:"tags"`
}

func newS3BucketResource() resource.Resource {
	return &minioS3BucketResource{}
}

// Metadata returns the resource type name
func (r *minioS3BucketResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket"
}

// Schema returns the resource schema
func (r *minioS3BucketResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MinIO S3 bucket.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Description: "Name of the bucket",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket_prefix": schema.StringAttribute{
				Description: "Prefix of the bucket. Only used during bucket creation; ignored for existing resources.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"force_destroy": schema.BoolAttribute{
				Description: "A boolean that indicates all objects should be deleted from the bucket so that the bucket can be destroyed without error.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"acl": schema.StringAttribute{
				Description: "Bucket's Access Control List (default: private)",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("private"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"arn": schema.StringAttribute{
				Description: "ARN of the bucket",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket_domain_name": schema.StringAttribute{
				Description: "The bucket domain name",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"quota": schema.Int64Attribute{
				Description: "Quota of the bucket",
				Optional:    true,
			},
			"object_locking": schema.BoolAttribute{
				Description: "Enable object locking for the bucket (default: false)",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"tags": schema.MapAttribute{
				Description: "A map of tags to assign to the bucket",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

// Configure sets up the resource
func (r *minioS3BucketResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *S3MinioClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

// Create creates the resource
func (r *minioS3BucketResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data minioS3BucketResourceModel

	// Read provider data
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Determine bucket name
	var bucket string
	if !data.Bucket.IsNull() && data.Bucket.ValueString() != "" {
		bucket = data.Bucket.ValueString()
	} else if !data.BucketPrefix.IsNull() && data.BucketPrefix.ValueString() != "" {
		bucket = id.PrefixedUniqueId(data.BucketPrefix.ValueString())
	} else {
		bucket = id.UniqueId()
	}

	// Validate bucket name
	if err := s3utils.CheckValidBucketName(bucket); err != nil {
		resp.Diagnostics.AddError("Invalid bucket name", err.Error())
		return
	}

	// Check if bucket already exists
	exists, err := r.client.S3Client.BucketExists(ctx, bucket)
	if err != nil {
		resp.Diagnostics.AddError("Unable to check bucket existence", err.Error())
		return
	}
	if exists {
		resp.Diagnostics.AddError("Bucket already exists", bucket)
		return
	}

	// Get region
	region := r.client.S3Region
	if region == "" {
		region = "us-east-1"
	}

	// Create bucket
	objectLocking := false
	if !data.ObjectLocking.IsNull() {
		objectLocking = data.ObjectLocking.ValueBool()
	}

	err = r.client.S3Client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{
		Region:        region,
		ObjectLocking: objectLocking,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create bucket", err.Error())
		return
	}

	// Set ID
	data.ID = types.StringValue(bucket)
	data.Bucket = types.StringValue(bucket)

	// Ensure bucket_prefix has a known value (it's Computed)
	if data.BucketPrefix.IsNull() || data.BucketPrefix.IsUnknown() {
		data.BucketPrefix = types.StringValue("")
	}

	// Wait for bucket to be ready
	timeout := 30 * time.Second
	if err := waitForBucketReadyFramework(ctx, r.client.S3Client, bucket, timeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for bucket to be ready", err.Error())
		return
	}

	// Compute derived attributes
	data.ARN = types.StringValue(bucketArn(bucket))
	data.BucketDomainName = types.StringValue(fmt.Sprintf("http://%s/%s", r.client.S3Endpoint, bucket))

	// Save data
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read reads the resource
func (r *minioS3BucketResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data minioS3BucketResourceModel

	// Read state data
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if bucket exists
	bucket := data.ID.ValueString()
	exists, err := r.client.S3Client.BucketExists(ctx, bucket)
	if err != nil {
		resp.Diagnostics.AddError("Error checking bucket existence", err.Error())
		return
	}
	if !exists {
		resp.State.RemoveResource(ctx)
		return
	}

	// Ensure bucket field is set
	if data.Bucket.IsNull() || data.Bucket.IsUnknown() {
		data.Bucket = types.StringValue(bucket)
	}

	// Preserve bucket_prefix from state (server doesn't return it)
	// Only set to empty if it's null
	if data.BucketPrefix.IsNull() {
		data.BucketPrefix = types.StringValue("")
	}

	// Compute derived attributes
	data.ARN = types.StringValue(bucketArn(bucket))
	data.BucketDomainName = types.StringValue(fmt.Sprintf("http://%s/%s", r.client.S3Endpoint, bucket))

	// Save data
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update updates the resource
func (r *minioS3BucketResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioS3BucketResourceModel

	// Read plan
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deletes the resource
func (r *minioS3BucketResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data minioS3BucketResourceModel

	// Read state data
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bucket := data.ID.ValueString()
	forceDestroy := false
	if !data.ForceDestroy.IsNull() {
		forceDestroy = data.ForceDestroy.ValueBool()
	}

	// If force_destroy is enabled, delete all objects first
	if forceDestroy {
		opts := minio.RemoveObjectsOptions{
			GovernanceBypass: true,
		}

		objectsCh := make(chan minio.ObjectInfo)
		go func() {
			defer close(objectsCh)
			for object := range r.client.S3Client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
				Recursive:    true,
				WithVersions: true,
			}) {
				objectsCh <- object
			}
		}()

		// Drain the error channel
		for err := range r.client.S3Client.RemoveObjects(ctx, bucket, objectsCh, opts) {
			tflog.Warn(ctx, "Error deleting object", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}

	// Delete the bucket
	err := r.client.S3Client.RemoveBucket(ctx, bucket)
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete bucket", err.Error())
		return
	}
}

// ImportState imports the resource
func (r *minioS3BucketResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Helper functions

func waitForBucketReadyFramework(ctx context.Context, client *minio.Client, bucket string, timeout time.Duration) error {
	return retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		_, err := client.GetBucketLocation(ctx, bucket)
		if err == nil {
			return nil
		}

		errResp := minio.ToErrorResponse(err)

		// Fail fast on credential errors
		if isCredentialError(errResp) {
			return retry.NonRetryableError(fmt.Errorf("access denied while waiting for bucket %q: %w", bucket, err))
		}

		// Retry on NoSuchBucket for eventual consistency
		if isNoSuchBucketError(err) {
			tflog.Debug(ctx, "Bucket not yet available, retrying", map[string]interface{}{"bucket": bucket})
			return retry.RetryableError(fmt.Errorf("bucket %q not yet available: %w", bucket, err))
		}

		return retry.NonRetryableError(err)
	})
}

func bucketArn(bucket string) string {
	return fmt.Sprintf("arn:aws:s3:::%s", bucket)
}
