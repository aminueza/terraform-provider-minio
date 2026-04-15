package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/policy"
	"github.com/minio/minio-go/v7/pkg/set"
)

var (
	_ resource.Resource                = &minioS3BucketAnonymousAccessResource{}
	_ resource.ResourceWithConfigure   = &minioS3BucketAnonymousAccessResource{}
	_ resource.ResourceWithImportState = &minioS3BucketAnonymousAccessResource{}
)

type minioS3BucketAnonymousAccessResource struct {
	client *minio.Client
}

type minioS3BucketAnonymousAccessResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Bucket            types.String `tfsdk:"bucket"`
	Policy            types.String `tfsdk:"policy"`
	AccessType        types.String `tfsdk:"access_type"`
	SkipBucketTagging types.Bool   `tfsdk:"skip_bucket_tagging"`
}

func newS3BucketAnonymousAccessResource() resource.Resource {
	return &minioS3BucketAnonymousAccessResource{}
}

func (r *minioS3BucketAnonymousAccessResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket_anonymous_access"
}

func (r *minioS3BucketAnonymousAccessResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Expected *S3MinioClient")
		return
	}
	r.client = client.S3Client
}

func (r *minioS3BucketAnonymousAccessResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages anonymous access policies for MinIO buckets. This resource allows you to configure bucket policies for public access using either custom JSON policies or canned access types.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier for the resource (bucket name).",
			},
			"bucket": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "Name of the bucket",
			},
			"policy": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("access_type")),
				},
				Description: "Custom policy JSON string for anonymous access. For canned policies (public, public-read, public-read-write, public-write), use the access_type field instead.",
			},
			"access_type": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf("public", "public-read", "public-read-write", "public-write"),
					stringvalidator.ConflictsWith(path.MatchRoot("policy")),
				},
				Description: "Canned access type for anonymous access",
			},
			"skip_bucket_tagging": schema.BoolAttribute{
				Optional:    true,
				Description: "Skip bucket tagging API calls. Useful when your S3-compatible endpoint does not support tagging.",
			},
		},
	}
}

func (r *minioS3BucketAnonymousAccessResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioS3BucketAnonymousAccessResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bucketName := plan.Bucket.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating anonymous access policy for bucket: %s", bucketName))

	policy, err := r.getAnonymousPolicy(ctx, &plan, bucketName)
	if err != nil {
		resp.Diagnostics.AddError("Building anonymous access policy", err.Error())
		return
	}

	if policy == "" {
		resp.Diagnostics.AddError("Validating anonymous access configuration", "policy or access_type must be specified")
		return
	}

	normalizedPolicy, err := r.normalizeJSON(policy)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize policy JSON", err.Error())
		return
	}

	plan.Policy = types.StringValue(normalizedPolicy)

	accessType := plan.AccessType.ValueString()
	if accessType == "" {
		accessType, err = r.getAccessTypeFromPolicy(normalizedPolicy, bucketName)
		if err != nil {
			resp.Diagnostics.AddError("Determining access_type", err.Error())
			return
		}
		if accessType != "" {
			plan.AccessType = types.StringValue(accessType)
		}
	}

	if err := r.putBucketPolicy(ctx, bucketName, normalizedPolicy); err != nil {
		resp.Diagnostics.AddError("Setting bucket policy", err.Error())
		return
	}

	plan.ID = types.StringValue(bucketName)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *minioS3BucketAnonymousAccessResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioS3BucketAnonymousAccessResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bucketName := state.Bucket.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Reading anonymous access policy for bucket: %s", bucketName))

	policy, err := r.getBucketPolicy(ctx, bucketName)
	if err != nil {
		if isNoSuchBucketError(err) {
			tflog.Warn(ctx, fmt.Sprintf("Bucket %s no longer exists, removing from state", bucketName))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to load bucket policy", err.Error())
		return
	}

	accessType, err := r.getAccessTypeFromPolicy(policy, bucketName)
	if err != nil {
		resp.Diagnostics.AddError("Determining access_type", err.Error())
		return
	}

	// If accessType is empty, the anonymous access policy was likely deleted externally
	if accessType == "" && !state.AccessType.IsNull() {
		tflog.Warn(ctx, fmt.Sprintf("Anonymous access policy for bucket %s no longer exists, removing from state", bucketName))
		resp.State.RemoveResource(ctx)
		return
	}

	if accessType != "" {
		if !state.AccessType.IsNull() && state.AccessType.ValueString() == accessType {
			canonical, err := r.canonicalPolicyForAccessType(accessType, bucketName)
			if err != nil {
				resp.Diagnostics.AddError("Building canonical anonymous access policy", err.Error())
				return
			}
			if canonical != "" {
				policy = canonical
			}
		}
	}

	state.ID = types.StringValue(bucketName)
	state.Policy = types.StringValue(policy)
	if accessType != "" {
		state.AccessType = types.StringValue(accessType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *minioS3BucketAnonymousAccessResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioS3BucketAnonymousAccessResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bucketName := plan.Bucket.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Updating anonymous access policy for bucket: %s", bucketName))

	policy, err := r.getAnonymousPolicy(ctx, &plan, bucketName)
	if err != nil {
		resp.Diagnostics.AddError("Building anonymous access policy", err.Error())
		return
	}

	normalizedPolicy, err := r.normalizeJSON(policy)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize policy JSON", err.Error())
		return
	}

	plan.Policy = types.StringValue(normalizedPolicy)

	accessType := plan.AccessType.ValueString()
	if accessType == "" {
		accessType, err = r.getAccessTypeFromPolicy(normalizedPolicy, bucketName)
		if err != nil {
			resp.Diagnostics.AddError("Determining access_type", err.Error())
			return
		}
		if accessType != "" {
			plan.AccessType = types.StringValue(accessType)
		}
	}

	if err := r.putBucketPolicy(ctx, bucketName, normalizedPolicy); err != nil {
		resp.Diagnostics.AddError("Setting bucket policy", err.Error())
		return
	}

	plan.ID = types.StringValue(bucketName)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *minioS3BucketAnonymousAccessResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioS3BucketAnonymousAccessResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bucketName := state.Bucket.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting anonymous access policy for bucket: %s", bucketName))

	if err := r.client.SetBucketPolicy(context.Background(), bucketName, ""); err != nil {
		if isNoSuchBucketError(err) {
			tflog.Debug(ctx, fmt.Sprintf("Bucket %q already deleted, skipping policy removal", bucketName))
			return
		}
		resp.Diagnostics.AddError("Deleting bucket policy", err.Error())
		return
	}
}

func (r *minioS3BucketAnonymousAccessResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("bucket"), req.ID)...)
}

func (r *minioS3BucketAnonymousAccessResource) putBucketPolicy(ctx context.Context, bucket, policy string) error {
	timeout := 5 * time.Minute
	waitTimeout := timeout - 30*time.Second
	if waitTimeout < 30*time.Second {
		waitTimeout = 30 * time.Second
	}

	if err := r.waitForBucketReady(ctx, bucket, waitTimeout); err != nil {
		return err
	}

	err := retry.RetryContext(ctx, waitTimeout, func() *retry.RetryError {
		err := r.client.SetBucketPolicy(ctx, bucket, policy)
		if err != nil {
			if isNoSuchBucketError(err) {
				tflog.Debug(ctx, fmt.Sprintf("Bucket %q not yet available for policy, retrying...", bucket))
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}
		return nil
	})

	return err
}

func (r *minioS3BucketAnonymousAccessResource) getBucketPolicy(ctx context.Context, bucket string) (string, error) {
	timeout := 2 * time.Minute
	if err := r.waitForBucketReady(ctx, bucket, timeout); err != nil {
		if isNoSuchBucketError(err) {
			return "", nil
		}
		return "", err
	}

	return r.client.GetBucketPolicy(ctx, bucket)
}

func (r *minioS3BucketAnonymousAccessResource) waitForBucketReady(ctx context.Context, bucket string, timeout time.Duration) error {
	return retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		exists, err := r.client.BucketExists(ctx, bucket)
		if err != nil {
			if isNoSuchBucketError(err) {
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}
		if !exists {
			return retry.RetryableError(fmt.Errorf("bucket %q does not exist", bucket))
		}
		return nil
	})
}

func (r *minioS3BucketAnonymousAccessResource) getAnonymousPolicy(ctx context.Context, plan *minioS3BucketAnonymousAccessResourceModel, bucket string) (string, error) {
	accessType := plan.AccessType.ValueString()

	if !plan.Policy.IsNull() && !plan.Policy.IsUnknown() && plan.Policy.ValueString() != "" {
		return plan.Policy.ValueString(), nil
	}

	if accessType != "" {
		switch accessType {
		case "public":
			return r.marshalPolicy(publicPolicy(bucket))
		case "public-read":
			return r.marshalPolicy(readOnlyPolicy(bucket))
		case "public-read-write":
			return r.marshalPolicy(readWritePolicy(bucket))
		case "public-write":
			return r.marshalPolicy(writeOnlyPolicy(bucket))
		}
	}

	if !plan.Policy.IsNull() && !plan.Policy.IsUnknown() {
		return plan.Policy.ValueString(), nil
	}

	return "", nil
}

func (r *minioS3BucketAnonymousAccessResource) canonicalPolicyForAccessType(accessType, bucketName string) (string, error) {
	switch accessType {
	case "public":
		return r.marshalPolicy(publicPolicy(bucketName))
	case "public-read":
		return r.marshalPolicy(readOnlyPolicy(bucketName))
	case "public-read-write":
		return r.marshalPolicy(readWritePolicy(bucketName))
	case "public-write":
		return r.marshalPolicy(writeOnlyPolicy(bucketName))
	default:
		return "", nil
	}
}

func (r *minioS3BucketAnonymousAccessResource) marshalPolicy(policyStruct BucketPolicy) (string, error) {
	policyJSON, err := json.Marshal(policyStruct)
	if err != nil {
		return "", err
	}
	return string(policyJSON), nil
}

func (r *minioS3BucketAnonymousAccessResource) normalizeJSON(jsonStr string) (string, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
		return "", err
	}
	normalized, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func (r *minioS3BucketAnonymousAccessResource) getAccessTypeFromPolicy(policy, bucketName string) (string, error) {
	publicPolicy, _ := r.marshalPolicy(publicPolicy(bucketName))
	readOnlyPolicy, _ := r.marshalPolicy(readOnlyPolicy(bucketName))
	readWritePolicy, _ := r.marshalPolicy(readWritePolicy(bucketName))
	writeOnlyPolicy, _ := r.marshalPolicy(writeOnlyPolicy(bucketName))

	equivalent, err := awspolicy.PoliciesAreEquivalent(policy, readOnlyPolicy)
	if err != nil {
		return "", err
	}
	if equivalent {
		return "public-read", nil
	}

	equivalent, err = awspolicy.PoliciesAreEquivalent(policy, publicPolicy)
	if err != nil {
		return "", err
	}
	if equivalent {
		return "public", nil
	}

	equivalent, err = awspolicy.PoliciesAreEquivalent(policy, readWritePolicy)
	if err != nil {
		return "", err
	}
	if equivalent {
		return "public-read-write", nil
	}

	equivalent, err = awspolicy.PoliciesAreEquivalent(policy, writeOnlyPolicy)
	if err != nil {
		return "", err
	}
	if equivalent {
		return "public-write", nil
	}

	return "", nil
}

func publicPolicy(bucket string) BucketPolicy {
	bucketResource := fmt.Sprintf("arn:aws:s3:::%s", bucket)
	objectResource := fmt.Sprintf("arn:aws:s3:::%s/*", bucket)

	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "PublicAccess",
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Actions: set.CreateStringSet(
					"s3:GetBucketLocation",
					"s3:ListBucket",
					"s3:ListBucketMultipartUploads",
				),
				Resources: set.CreateStringSet(bucketResource),
			},
			{
				Sid:       "PublicAccess",
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Actions:   set.CreateStringSet("s3:GetObject"),
				Resources: set.CreateStringSet(objectResource),
			},
		},
	}
}

func readOnlyPolicy(bucket string) BucketPolicy {
	objectResource := fmt.Sprintf("arn:aws:s3:::%s/*", bucket)

	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "PublicRead",
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Actions:   set.CreateStringSet("s3:GetObject"),
				Resources: set.CreateStringSet(objectResource),
			},
		},
	}
}

func readWritePolicy(bucket string) BucketPolicy {
	objectResource := fmt.Sprintf("arn:aws:s3:::%s/*", bucket)

	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "PublicReadWrite",
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Actions: set.CreateStringSet(
					"s3:GetObject",
					"s3:PutObject",
				),
				Resources: set.CreateStringSet(objectResource),
			},
		},
	}
}

func writeOnlyPolicy(bucket string) BucketPolicy {
	objectResource := fmt.Sprintf("arn:aws:s3:::%s/*", bucket)

	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "PublicWrite",
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Actions:   set.CreateStringSet("s3:PutObject"),
				Resources: set.CreateStringSet(objectResource),
			},
		},
	}
}
