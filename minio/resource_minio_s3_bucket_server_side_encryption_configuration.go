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

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Description: "Name of the bucket on which to setup server side encryption",
				Required:    true,
				ForceNew:    true,
			},
			"encryption_type": {
				Type:         schema.TypeString,
				Description:  "Server side encryption type",
				Required:     true,
				ValidateFunc: validation.StringInSlice([]string{"aws:kms"}, false),
			},
			"kms_key_id": {
				Type:        schema.TypeString,
				Description: "KMS key id to use for server side encryption",
				Required:    true,
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

	log.Printf("[DEBUG] S3 bucket: %s, put encryption configuration: %v", bucketEncryptionConfig.MinioBucket, encryptionConfig)

	err := bucketEncryptionConfig.MinioClient.SetBucketEncryption(
		ctx,
		bucketEncryptionConfig.MinioBucket,
		encryptionConfig,
	)

	if err != nil {
		return NewResourceError("error putting bucket encryption configuration", d.Id(), err)
	}

	d.SetId(bucketEncryptionConfig.MinioBucket)

	return nil
}

func minioReadBucketServerSideEncryption(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketEncryptionConfig := BucketServerSideEncryptionConfig(d, meta)

	log.Printf("[DEBUG] S3 bucket encryption, read for bucket: %s", d.Id())

	encryptionConfig, err := bucketEncryptionConfig.MinioClient.GetBucketEncryption(ctx, d.Id())
	if err != nil {
		d.SetId("")

		return nil
	}

	if err := d.Set("bucket", d.Id()); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("encryption_type", encryptionConfig.Rules[0].Apply.SSEAlgorithm); err != nil {
		return diag.FromErr(fmt.Errorf("error setting encryption type: %w", err))
	}

	if err := d.Set("kms_key_id", encryptionConfig.Rules[0].Apply.KmsMasterKeyID); err != nil {
		return diag.FromErr(fmt.Errorf("error setting encryption kms key id: %w", err))
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
	result := &sse.Configuration{
		Rules: []sse.Rule{
			{
				Apply: sse.ApplySSEByDefault{
					SSEAlgorithm:   d.Get("encryption_type").(string),
					KmsMasterKeyID: d.Get("kms_key_id").(string),
				},
			},
		},
	}

	return result
}
