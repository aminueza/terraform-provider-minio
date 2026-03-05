package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketEncryption() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the server-side encryption configuration of an existing S3 bucket.",
		Read:        dataSourceMinioS3BucketEncryptionRead,
		Schema: map[string]*schema.Schema{
			"bucket":            {Type: schema.TypeString, Required: true},
			"encryption_type":   {Type: schema.TypeString, Computed: true},
			"kms_master_key_id": {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketEncryptionRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	d.SetId(bucket)

	cfg, err := client.GetBucketEncryption(context.Background(), bucket)
	if err != nil {
		_ = d.Set("encryption_type", "")
		return nil
	}

	if len(cfg.Rules) > 0 {
		rule := cfg.Rules[0]
		_ = d.Set("encryption_type", rule.Apply.SSEAlgorithm)
		_ = d.Set("kms_master_key_id", rule.Apply.KmsMasterKeyID)
	}

	return nil
}
