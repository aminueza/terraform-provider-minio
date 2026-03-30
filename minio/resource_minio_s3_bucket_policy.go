package minio

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
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
		return NewResourceError("unable to set bucket policy with invalid JSON: %w", policy, err)
	}

	log.Printf("[DEBUG] S3 bucket: %s, put policy: %s", bucketPolicyConfig.MinioBucket, policy)

	// Wait for bucket to be ready for eventual consistency
	timeout := d.Timeout(schema.TimeoutCreate)
	if d.Id() != "" {
		timeout = d.Timeout(schema.TimeoutUpdate)
	}
	// Reserve time for the actual operation
	waitTimeout := timeout - 30*time.Second
	if waitTimeout < 30*time.Second {
		waitTimeout = 30 * time.Second
	}

	if err := waitForBucketReady(ctx, bucketPolicyConfig.MinioClient, bucketPolicyConfig.MinioBucket, waitTimeout); err != nil {
		return NewResourceError("error waiting for bucket to be ready", bucketPolicyConfig.MinioBucket, err)
	}

	// Retry SetBucketPolicy for transient NoSuchBucket errors
	err = retry.RetryContext(ctx, waitTimeout, func() *retry.RetryError {
		err := bucketPolicyConfig.MinioClient.SetBucketPolicy(ctx, bucketPolicyConfig.MinioBucket, policy)
		if err != nil {
			if isNoSuchBucketError(err) {
				log.Printf("[DEBUG] Bucket %q not yet available for policy, retrying...", bucketPolicyConfig.MinioBucket)
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return NewResourceError("error putting bucket policy: %s", policy, err)
	}

	// MinIO multi-drive deployments can lose bucket versioning when a policy
	// is set immediately after versioning. Capture versioning before and
	// restore it if SetBucketPolicy destroyed it.
	versioningBefore, _ := bucketPolicyConfig.MinioClient.GetBucketVersioning(ctx, bucketPolicyConfig.MinioBucket)
	if versioningBefore.Status != "" {
		time.Sleep(500 * time.Millisecond)
		versioningAfter, _ := bucketPolicyConfig.MinioClient.GetBucketVersioning(ctx, bucketPolicyConfig.MinioBucket)
		if versioningAfter.Status == "" {
			log.Printf("[WARN] Bucket %s versioning was lost after SetBucketPolicy, restoring", bucketPolicyConfig.MinioBucket)
			_ = bucketPolicyConfig.MinioClient.SetBucketVersioning(ctx, bucketPolicyConfig.MinioBucket, versioningBefore)
		}
	}

	d.SetId(bucketPolicyConfig.MinioBucket)

	return minioReadBucketPolicy(ctx, d, meta)
}

func minioReadBucketPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketPolicyConfig := BucketPolicyConfig(d, meta)

	log.Printf("[DEBUG] S3 bucket policy, read for bucket: %s", d.Id())

	// For new resources, wait for bucket to be ready
	if d.IsNewResource() {
		timeout := d.Timeout(schema.TimeoutRead)
		if err := waitForBucketReady(ctx, bucketPolicyConfig.MinioClient, d.Id(), timeout); err != nil {
			if isNoSuchBucketError(err) {
				log.Printf("[WARN] Bucket %s not found after waiting, removing policy resource from state", d.Id())
				d.SetId("")
				return nil
			}
			return NewResourceError("error waiting for bucket to be ready", d.Id(), err)
		}
	}

	var actualPolicyText string
	var readPolicyErr error

	actualPolicyText, readPolicyErr = bucketPolicyConfig.MinioClient.GetBucketPolicy(ctx, d.Id())
	if readPolicyErr != nil {
		if isNoSuchBucketError(readPolicyErr) && !d.IsNewResource() {
			log.Printf("[WARN] Bucket %s no longer exists, removing policy resource from state", d.Id())
			d.SetId("")
			return nil
		}
		if !d.IsNewResource() {
			return NewResourceError("failed to load bucket policy", d.Id(), readPolicyErr)
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
				return NewResourceError("failed to load bucket policy", d.Id(), retryErr)
			}
			log.Printf("[WARN] Bucket %s policy is empty, assuming deleted externally", d.Id())
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
		return NewResourceError("error while comparing policies", d.Id(), err)
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

	log.Printf("[DEBUG] S3 bucket: %s, delete policy", bucketPolicyConfig.MinioBucket)

	err := bucketPolicyConfig.MinioClient.SetBucketPolicy(ctx, bucketPolicyConfig.MinioBucket, "")

	if err != nil {
		return NewResourceError("error deleting bucket", bucketPolicyConfig.MinioBucket, err)
	}

	return nil
}
