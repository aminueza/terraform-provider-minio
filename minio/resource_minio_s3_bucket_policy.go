package minio

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
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

	err = bucketPolicyConfig.MinioClient.SetBucketPolicy(ctx, bucketPolicyConfig.MinioBucket, policy)

	if err != nil {
		return NewResourceError("error putting bucket policy: %s", policy, err)
	}

	d.SetId(bucketPolicyConfig.MinioBucket)

	return nil
}

func minioReadBucketPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketPolicyConfig := BucketPolicyConfig(d, meta)

	log.Printf("[DEBUG] S3 bucket policy, read for bucket: %s", d.Id())

	actualPolicyText, err := bucketPolicyConfig.MinioClient.GetBucketPolicy(ctx, d.Id())
	if err != nil {
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
