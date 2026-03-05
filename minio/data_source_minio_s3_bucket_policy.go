package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketPolicy() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the bucket policy document for an existing S3 bucket.",
		Read:        dataSourceMinioS3BucketPolicyRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true},
			"policy": {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketPolicyRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client

	bucket := d.Get("bucket").(string)
	policy, err := client.GetBucketPolicy(context.Background(), bucket)
	if err != nil {
		d.SetId(bucket)
		_ = d.Set("policy", "")
		return nil
	}

	d.SetId(bucket)
	_ = d.Set("policy", policy)
	return nil
}
