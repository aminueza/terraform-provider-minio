package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketVersioning() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the versioning configuration of an existing S3 bucket.",
		ReadContext: dataSourceMinioS3BucketVersioningRead,
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
		return NewResourceError("reading bucket versioning", bucket, err)
	}

	d.SetId(bucket)
	if diags := setResourceField(d, "enabled", cfg.Enabled(), bucket); diags.HasError() {
		return diags
	}
	return setResourceField(d, "suspended", cfg.Suspended(), bucket)
}
