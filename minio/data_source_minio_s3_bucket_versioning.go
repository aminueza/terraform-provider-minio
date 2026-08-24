package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

func dataSourceMinioS3BucketVersioning() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the versioning configuration of an existing S3 bucket.",
		ReadContext:        dataSourceMinioS3BucketVersioningRead,
		Schema: map[string]*schema.Schema{
			"bucket":    {Type: schema.TypeString, Required: true},
			"enabled":   {Type: schema.TypeBool, Computed: true},
			"suspended": {Type: schema.TypeBool, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketVersioningRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	cfg, err := client.GetBucketVersioning(ctx, bucket)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(bucket)
	_ = d.Set("enabled", cfg.Enabled())
	_ = d.Set("suspended", cfg.Suspended())
	return nil
}
