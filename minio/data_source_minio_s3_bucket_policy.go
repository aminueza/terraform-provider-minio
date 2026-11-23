package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketPolicy() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the bucket policy document for an existing S3 bucket.",
		ReadContext: dataSourceMinioS3BucketPolicyRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true},
			"policy": {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketPolicyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	bucket := d.Get("bucket").(string)

	var diags diag.Diagnostics
	policy, err := client.S3Client.GetBucketPolicy(ctx, bucket)
	switch {
	case err == nil:
	case isS3CompatNotSupported(client, s3Compat405IsMissingFeature, err):
		diags = append(diags, dataSourceS3CompatGap(bucket, "policy", "bucket policies"))
	default:
		return NewResourceError("reading bucket policy", bucket, err)
	}

	d.SetId(bucket)
	if err := d.Set("policy", policy); err != nil {
		return NewResourceError("setting policy", bucket, err)
	}
	return diags
}
