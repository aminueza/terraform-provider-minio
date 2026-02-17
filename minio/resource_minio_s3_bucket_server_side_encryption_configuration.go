package minio

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7/pkg/sse"
)

func resourceMinioBucketServerSideEncryption() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioPutBucketServerSideEncryption,
		ReadContext:   minioReadBucketServerSideEncryption,
		UpdateContext: minioPutBucketServerSideEncryption,
		DeleteContext: minioDeleteBucketServerSideEncryption,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Description: "Manages server-side encryption configuration for an S3 bucket. Supports SSE-S3 (AES256) and SSE-KMS (aws:kms) encryption types.",
		CustomizeDiff: func(_ context.Context, diff *schema.ResourceDiff, _ interface{}) error {
			encType := diff.Get("encryption_type").(string)
			keyID, _ := diff.Get("kms_key_id").(string)
			if encType == "aws:kms" && keyID == "" {
				return fmt.Errorf("kms_key_id is required when encryption_type is \"aws:kms\"")
			}
			return nil
		},

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Description: "Name of the bucket on which to setup server side encryption",
				Required:    true,
				ForceNew:    true,
			},
			"encryption_type": {
				Type:         schema.TypeString,
				Description:  "Server side encryption type: `AES256` for SSE-S3 or `aws:kms` for SSE-KMS",
				Required:     true,
				ValidateFunc: validation.StringInSlice([]string{"aws:kms", "AES256"}, false),
			},
			"kms_key_id": {
				Type:        schema.TypeString,
				Description: "KMS key id to use for SSE-KMS encryption. Required when encryption_type is `aws:kms`, ignored for `AES256`.",
				Optional:    true,
			},
		},
	}
}

func minioPutBucketServerSideEncryption(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketEncryptionConfig := BucketServerSideEncryptionConfig(d, meta)
	encryptionConfig := getBucketServerSideEncryptionConfig(d)

	if encryptionConfig == nil {
		return nil
	}

	log.Printf("[DEBUG] S3 bucket: %s, putting encryption configuration", bucketEncryptionConfig.MinioBucket)

	err := bucketEncryptionConfig.MinioClient.SetBucketEncryption(
		ctx,
		bucketEncryptionConfig.MinioBucket,
		encryptionConfig,
	)

	if err != nil {
		return NewResourceError("putting bucket encryption configuration", bucketEncryptionConfig.MinioBucket, err)
	}

	d.SetId(bucketEncryptionConfig.MinioBucket)
	log.Printf("[DEBUG] S3 bucket: %s, encryption configuration applied", bucketEncryptionConfig.MinioBucket)

	return minioReadBucketServerSideEncryption(ctx, d, meta)
}

func minioReadBucketServerSideEncryption(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketEncryptionConfig := BucketServerSideEncryptionConfig(d, meta)

	log.Printf("[DEBUG] S3 bucket encryption, read for bucket: %s", d.Id())

	encryptionConfig, err := bucketEncryptionConfig.MinioClient.GetBucketEncryption(ctx, d.Id())
	if err != nil {
		d.SetId("")
		return nil
	}

	if len(encryptionConfig.Rules) == 0 {
		d.SetId("")
		return nil
	}

	if err := d.Set("bucket", d.Id()); err != nil {
		return NewResourceError("setting bucket", d.Id(), err)
	}

	if err := d.Set("encryption_type", encryptionConfig.Rules[0].Apply.SSEAlgorithm); err != nil {
		return NewResourceError("setting encryption_type", d.Id(), err)
	}

	if err := d.Set("kms_key_id", encryptionConfig.Rules[0].Apply.KmsMasterKeyID); err != nil {
		return NewResourceError("setting kms_key_id", d.Id(), err)
	}

	return nil
}

func minioDeleteBucketServerSideEncryption(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketEncryptionConfig := BucketServerSideEncryptionConfig(d, meta)

	if v := getBucketServerSideEncryptionConfig(d); v != nil && len(v.Rules) == 0 {
		log.Printf("[DEBUG] Removing bucket encryption for unencrypted bucket (%s) from state", d.Id())
		return nil
	}

	log.Printf("[DEBUG] S3 bucket: %s, removing bucket encryption", bucketEncryptionConfig.MinioBucket)

	err := bucketEncryptionConfig.MinioClient.RemoveBucketEncryption(ctx, bucketEncryptionConfig.MinioBucket)
	if err != nil {
		return NewResourceError("error removing bucket encryption", bucketEncryptionConfig.MinioBucket, err)
	}

	return nil
}

func getBucketServerSideEncryptionConfig(d *schema.ResourceData) *sse.Configuration {
	encryptionType := d.Get("encryption_type").(string)

	if encryptionType == "AES256" {
		return sse.NewConfigurationSSES3()
	}

	keyID, _ := d.Get("kms_key_id").(string)
	return sse.NewConfigurationSSEKMS(keyID)
}
