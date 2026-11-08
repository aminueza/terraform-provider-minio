package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketEncryption() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the server-side encryption configuration of an existing S3 bucket.",
		ReadContext: dataSourceMinioS3BucketEncryptionRead,
		Schema: map[string]*schema.Schema{
			"bucket":            {Type: schema.TypeString, Required: true},
			"encryption_type":   {Type: schema.TypeString, Computed: true},
			"kms_master_key_id": {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketEncryptionRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	bucket := d.Get("bucket").(string)

	d.SetId(bucket)

	var diags diag.Diagnostics
	encryptionType, kmsMasterKeyID := "", ""

	cfg, err := client.S3Client.GetBucketEncryption(ctx, bucket)
	switch {
	case err == nil:
		if len(cfg.Rules) > 0 {
			encryptionType = cfg.Rules[0].Apply.SSEAlgorithm
			kmsMasterKeyID = cfg.Rules[0].Apply.KmsMasterKeyID
		}
	case isBucketEncryptionNotFoundError(err):
	case isS3CompatNotSupported(client, s3Compat405IsMissingFeature, err):
		diags = append(diags, dataSourceS3CompatGap(bucket, "encryption_type and kms_master_key_id", "bucket encryption"))
	default:
		return NewResourceError("reading bucket encryption", bucket, err)
	}

	if err := d.Set("encryption_type", encryptionType); err != nil {
		return NewResourceError("setting encryption_type", bucket, err)
	}
	if err := d.Set("kms_master_key_id", kmsMasterKeyID); err != nil {
		return NewResourceError("setting kms_master_key_id", bucket, err)
	}

	return diags
}
