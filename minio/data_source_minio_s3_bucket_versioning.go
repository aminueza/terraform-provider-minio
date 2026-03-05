package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketVersioning() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the versioning configuration of an existing S3 bucket.",
		Read:        dataSourceMinioS3BucketVersioningRead,
		Schema: map[string]*schema.Schema{
			"bucket":    {Type: schema.TypeString, Required: true},
			"enabled":   {Type: schema.TypeBool, Computed: true},
			"suspended": {Type: schema.TypeBool, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketVersioningRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	cfg, err := client.GetBucketVersioning(context.Background(), bucket)
	if err != nil {
		return err
	}

	d.SetId(bucket)
	_ = d.Set("enabled", cfg.Enabled())
	_ = d.Set("suspended", cfg.Suspended())
	return nil
}
