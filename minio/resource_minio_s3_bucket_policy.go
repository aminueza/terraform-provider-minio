package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
	minio "github.com/minio/minio-go/v7"
)

func resourceMinioBucketPolicy() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioPutBucketPolicy,
		ReadContext:   minioReadBucketPolicy,
		UpdateContext: minioPutBucketPolicy,
		DeleteContext: minioDeleteBucketPolicy,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Read:   schema.DefaultTimeout(2 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Description: "Name of the bucket",
				Required:    true,
				ForceNew:    true,
			},
			"policy": {
				Type:             schema.TypeString,
				Description:      "Policy JSON string",
				Required:         true,
				ValidateFunc:     validateIAMPolicyJSON,
				DiffSuppressFunc: suppressEquivalentAwsPolicyDiffs,
			},
		},
	}
}

func minioPutBucketPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketPolicyConfig := BucketPolicyConfig(d, meta)

	policy, err := structure.NormalizeJsonString(bucketPolicyConfig.MinioBucketPolicy)

	if err != nil {
		return NewResourceError("setting bucket policy with invalid JSON", policy, err)
	}

	tflog.Debug(ctx, fmt.Sprintf("S3 bucket: %s, put policy: %s", bucketPolicyConfig.MinioBucket, policy))

	timeout := d.Timeout(schema.TimeoutCreate)
	if d.Id() != "" {
		timeout = d.Timeout(schema.TimeoutUpdate)
	}
	waitTimeout := timeout - 30*time.Second
	if waitTimeout < 30*time.Second {
		waitTimeout = 30 * time.Second
	}

	if err := waitForBucketReady(ctx, bucketPolicyConfig.MinioClient, bucketPolicyConfig.MinioBucket, waitTimeout); err != nil {
		return NewResourceError("waiting for bucket to be ready", bucketPolicyConfig.MinioBucket, err)
	}

	// MinIO multi-drive deployments can lose bucket versioning when a policy
	// is set immediately after it. Capture versioning before the write so a
	// loss caused by SetBucketPolicy can be restored (issue #1389). The read
	// retries NoSuchBucket like the write below: with a configured region
	// GetBucketLocation is answered locally and does not prove the bucket is
	// visible yet.
	var versioningBefore minio.BucketVersioningConfiguration
	beforeErr := retry.RetryContext(ctx, waitTimeout, func() *retry.RetryError {
		cfg, err := bucketPolicyConfig.MinioClient.GetBucketVersioning(ctx, bucketPolicyConfig.MinioBucket)
		if err != nil {
			if isNoSuchBucketError(err) {
				tflog.Debug(ctx, fmt.Sprintf("Bucket %q not yet available for the versioning snapshot, retrying...", bucketPolicyConfig.MinioBucket))
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}
		versioningBefore = cfg
		return nil
	})
	if beforeErr != nil {
		// A backend without the versioning API cannot lose versioning on a
		// policy write, so s3_compat_mode skips the check; any other failure
		// aborts before the write instead of running unprotected.
		if !isS3CompatNotSupported(meta.(*S3MinioClient), s3Compat405IsMissingFeature, beforeErr) {
			return NewResourceError("reading bucket versioning before setting policy", bucketPolicyConfig.MinioBucket, beforeErr)
		}
		tflog.Warn(ctx, fmt.Sprintf("Bucket %s: cannot read versioning before setting the policy, skipping the versioning-loss check: %v", bucketPolicyConfig.MinioBucket, beforeErr))
	}

	// Retry SetBucketPolicy for transient NoSuchBucket errors
	err = retry.RetryContext(ctx, waitTimeout, func() *retry.RetryError {
		err := bucketPolicyConfig.MinioClient.SetBucketPolicy(ctx, bucketPolicyConfig.MinioBucket, policy)
		if err != nil {
			if isNoSuchBucketError(err) {
				tflog.Debug(ctx, fmt.Sprintf("Bucket %q not yet available for policy, retrying...", bucketPolicyConfig.MinioBucket))
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return NewResourceError("putting bucket policy", policy, s3CompatWriteError(meta.(*S3MinioClient), "bucket policy", err))
	}

	// A failed verification read counts as lost: re-applying the captured
	// configuration is idempotent, and skipping the restore would silently
	// drop the protection this workaround exists for.
	if versioningBefore.Status != "" {
		time.Sleep(500 * time.Millisecond)
		versioningAfter, afterErr := bucketPolicyConfig.MinioClient.GetBucketVersioning(ctx, bucketPolicyConfig.MinioBucket)
		if afterErr != nil || versioningAfter.Status == "" {
			tflog.Warn(ctx, fmt.Sprintf("Bucket %s versioning was lost after SetBucketPolicy, restoring", bucketPolicyConfig.MinioBucket))
			if restoreErr := bucketPolicyConfig.MinioClient.SetBucketVersioning(ctx, bucketPolicyConfig.MinioBucket, versioningBefore); restoreErr != nil {
				return NewResourceError("restoring bucket versioning after setting policy", bucketPolicyConfig.MinioBucket, restoreErr)
			}
		}
	}

	d.SetId(bucketPolicyConfig.MinioBucket)

	return minioReadBucketPolicy(ctx, d, meta)
}

func minioReadBucketPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketPolicyConfig := BucketPolicyConfig(d, meta)

	tflog.Debug(ctx, fmt.Sprintf("S3 bucket policy, read for bucket: %s", d.Id()))

	if d.IsNewResource() {
		timeout := d.Timeout(schema.TimeoutRead)
		if err := waitForBucketReady(ctx, bucketPolicyConfig.MinioClient, d.Id(), timeout); err != nil {
			if isNoSuchBucketError(err) {
				tflog.Warn(ctx, fmt.Sprintf("Bucket %s not found after waiting, removing policy resource from state", d.Id()))
				d.SetId("")
				return nil
			}
			return NewResourceError("waiting for bucket to be ready", d.Id(), err)
		}
	}

	var actualPolicyText string
	var readPolicyErr error

	actualPolicyText, readPolicyErr = bucketPolicyConfig.MinioClient.GetBucketPolicy(ctx, d.Id())
	if readPolicyErr != nil {
		if s3CompatReadUnsupported(ctx, meta.(*S3MinioClient), s3Compat405IsMissingFeature, d, "bucket policy", readPolicyErr) {
			return nil
		}
		if isNoSuchBucketError(readPolicyErr) && !d.IsNewResource() {
			tflog.Warn(ctx, fmt.Sprintf("Bucket %s no longer exists, removing policy resource from state", d.Id()))
			d.SetId("")
			return nil
		}
		if !d.IsNewResource() {
			return NewResourceError("loading bucket policy", d.Id(), readPolicyErr)
		}
	}

	if strings.TrimSpace(actualPolicyText) == "" || strings.TrimSpace(actualPolicyText) == "{}" {
		retryTimeout := 5 * time.Second
		if d.IsNewResource() {
			retryTimeout = d.Timeout(schema.TimeoutRead)
		}
		retryErr := retry.RetryContext(ctx, retryTimeout, func() *retry.RetryError {
			var err error
			actualPolicyText, err = bucketPolicyConfig.MinioClient.GetBucketPolicy(ctx, d.Id())
			if err != nil {
				if isNoSuchBucketError(err) && d.IsNewResource() {
					return retry.RetryableError(err)
				}
				return retry.NonRetryableError(err)
			}
			if strings.TrimSpace(actualPolicyText) == "" || strings.TrimSpace(actualPolicyText) == "{}" {
				return retry.RetryableError(fmt.Errorf("policy not yet available for bucket %s", d.Id()))
			}
			return nil
		})
		if retryErr != nil {
			if d.IsNewResource() {
				return NewResourceError("loading bucket policy", d.Id(), retryErr)
			}
			tflog.Warn(ctx, fmt.Sprintf("Bucket %s policy is empty, assuming deleted externally", d.Id()))
			d.SetId("")
			return nil
		}
	}

	existingPolicy := ""
	if v, ok := d.GetOk("policy"); ok {
		existingPolicy = v.(string)
	}

	policy, err := NormalizeAndCompareJSONPolicies(existingPolicy, actualPolicyText)
	if err != nil {
		return NewResourceError("comparing policies", d.Id(), err)
	}

	if err := d.Set("policy", policy); err != nil {
		return NewResourceError("setting bucket policy", d.Id(), err)
	}

	if err := d.Set("bucket", d.Id()); err != nil {
		return NewResourceError("setting bucket attribute", d.Id(), err)
	}

	return nil
}

func minioDeleteBucketPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketPolicyConfig := BucketPolicyConfig(d, meta)

	tflog.Debug(ctx, fmt.Sprintf("S3 bucket: %s, delete policy", bucketPolicyConfig.MinioBucket))

	err := bucketPolicyConfig.MinioClient.SetBucketPolicy(ctx, bucketPolicyConfig.MinioBucket, "")

	if err != nil {
		return NewResourceError("deleting bucket policy", bucketPolicyConfig.MinioBucket, err)
	}

	return nil
}
