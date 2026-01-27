package minio

import (
	"context"
	"log"
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

	actualPolicyText, err := bucketPolicyConfig.MinioClient.GetBucketPolicy(ctx, d.Id())
	if err != nil {
		// Handle bucket not found for existing resources
		if isNoSuchBucketError(err) && !d.IsNewResource() {
			log.Printf("[WARN] Bucket %s no longer exists, removing policy resource from state", d.Id())
			d.SetId("")
			return nil
		}
		return NewResourceError("failed to load bucket policy", d.Id(), err)
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
		return diag.FromErr(err)
	}

	if err := d.Set("bucket", d.Id()); err != nil {
		return diag.FromErr(err)
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
