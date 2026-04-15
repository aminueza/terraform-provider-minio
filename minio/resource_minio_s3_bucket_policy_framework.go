package minio

import (
	"context"
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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
)

// Ensure provider defined types fully satisfy framework interfaces
var (
	_ resource.Resource                = &bucketPolicyResource{}
	_ resource.ResourceWithConfigure   = &bucketPolicyResource{}
	_ resource.ResourceWithImportState = &bucketPolicyResource{}
)

// bucketPolicyResource defines the resource implementation
type bucketPolicyResource struct {
	client *S3MinioClient
}

// bucketPolicyResourceModel describes the resource data model
type bucketPolicyResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Bucket types.String `tfsdk:"bucket"`
	Policy types.String `tfsdk:"policy"`
}

func (r *bucketPolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket_policy"
}

func (r *bucketPolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a MinIO S3 Bucket Policy resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Bucket name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Description: "Name of the bucket.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"policy": schema.StringAttribute{
				Description: "Policy JSON string.",
				Required:    true,
			},
		},
	}
}

func (r *bucketPolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *bucketPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data bucketPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.putPolicy(ctx, &data)...)
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

func (r *bucketPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data bucketPolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Handle external deletion - if ID is null after read, remove resource from state
	if data.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data bucketPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.putPolicy(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data bucketPolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete policy by setting it to empty
	err := r.client.S3Client.SetBucketPolicy(ctx, data.Bucket.ValueString(), "")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting bucket policy",
			"Could not delete bucket policy: "+err.Error(),
		)
		return
	}

	// Clear ID
	data.ID = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("bucket"), req, resp)
}

func (r *bucketPolicyResource) putPolicy(ctx context.Context, data *bucketPolicyResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	policy, err := structure.NormalizeJsonString(data.Policy.ValueString())
	if err != nil {
		diags.AddError(
			"Invalid policy JSON",
			err.Error(),
		)
		return diags
	}

	// Wait for bucket to be ready
	timeout := 5 * time.Minute
	if err := waitForBucketReadyFramework(ctx, r.client.S3Client, data.Bucket.ValueString(), timeout); err != nil {
		diags.AddError(
			"Error waiting for bucket to be ready",
			err.Error(),
		)
		return diags
	}

	// Retry SetBucketPolicy for transient NoSuchBucket errors
	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err := r.client.S3Client.SetBucketPolicy(ctx, data.Bucket.ValueString(), policy)
		if err != nil {
			if isNoSuchBucketError(err) {
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		diags.AddError(
			"Error putting bucket policy",
			err.Error(),
		)
		return diags
	}

	// Handle versioning preservation
	versioningBefore, _ := r.client.S3Client.GetBucketVersioning(ctx, data.Bucket.ValueString())
	if versioningBefore.Status != "" {
		time.Sleep(500 * time.Millisecond)
		versioningAfter, _ := r.client.S3Client.GetBucketVersioning(ctx, data.Bucket.ValueString())
		if versioningAfter.Status == "" {
			_ = r.client.S3Client.SetBucketVersioning(ctx, data.Bucket.ValueString(), versioningBefore)
		}
	}

	return diags
}

func (r *bucketPolicyResource) read(ctx context.Context, data *bucketPolicyResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	// For new resources, wait for bucket to be ready
	if data.ID.IsNull() {
		timeout := 2 * time.Minute
		if err := waitForBucketReadyFramework(ctx, r.client.S3Client, data.Bucket.ValueString(), timeout); err != nil {
			if isNoSuchBucketError(err) {
				data.ID = types.StringNull()
				return diags
			}
			diags.AddError(
				"Error waiting for bucket to be ready",
				err.Error(),
			)
			return diags
		}
	}

	var actualPolicyText string
	var readPolicyErr error

	actualPolicyText, readPolicyErr = r.client.S3Client.GetBucketPolicy(ctx, data.Bucket.ValueString())
	if readPolicyErr != nil {
		if isNoSuchBucketError(readPolicyErr) && !data.ID.IsNull() {
			data.ID = types.StringNull()
			return diags
		}
		if data.ID.IsNull() {
			diags.AddError(
				"Error reading bucket policy",
				readPolicyErr.Error(),
			)
			return diags
		}
	}

	if strings.TrimSpace(actualPolicyText) == "" || strings.TrimSpace(actualPolicyText) == "{}" {
		retryTimeout := 5 * time.Second
		retryErr := retry.RetryContext(ctx, retryTimeout, func() *retry.RetryError {
			var err error
			actualPolicyText, err = r.client.S3Client.GetBucketPolicy(ctx, data.Bucket.ValueString())
			if err != nil {
				if isNoSuchBucketError(err) && data.ID.IsNull() {
					return retry.RetryableError(err)
				}
				return retry.NonRetryableError(err)
			}
			if strings.TrimSpace(actualPolicyText) == "" || strings.TrimSpace(actualPolicyText) == "{}" {
				return retry.RetryableError(fmt.Errorf("policy not yet available for bucket %s", data.Bucket.ValueString()))
			}
			return nil
		})
		if retryErr != nil {
			if data.ID.IsNull() {
				diags.AddError(
					"Error reading bucket policy",
					retryErr.Error(),
				)
				return diags
			}
			data.ID = types.StringNull()
			return diags
		}
	}

	existingPolicy := ""
	if !data.Policy.IsNull() && !data.Policy.IsUnknown() {
		existingPolicy = data.Policy.ValueString()
	}

	policy, err := NormalizeAndCompareJSONPolicies(existingPolicy, actualPolicyText)
	if err != nil {
		diags.AddError(
			"Error comparing policies",
			err.Error(),
		)
		return diags
	}

	data.Policy = types.StringValue(policy)
	if data.ID.IsNull() {
		data.ID = data.Bucket
	}

	return diags
}

// newBucketPolicyResource creates a new bucket policy resource instance
func newBucketPolicyResource() resource.Resource {
	return &bucketPolicyResource{}
}
