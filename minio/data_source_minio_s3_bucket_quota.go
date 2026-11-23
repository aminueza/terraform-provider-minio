package minio

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketQuota() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the quota configuration of an existing S3 bucket.",
		ReadContext: dataSourceMinioS3BucketQuotaRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true},
			"quota":  {Type: schema.TypeInt, Computed: true},
			"type":   {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketQuotaRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	admin := client.S3Admin
	bucket := d.Get("bucket").(string)

	var diags diag.Diagnostics
	quotaVal := 0
	quotaType := ""

	bucketQuota, err := admin.GetBucketQuota(ctx, bucket)
	switch {
	case err == nil:
		if bucketQuota.Size > 0 {
			q, ok := SafeUint64ToInt64(bucketQuota.Size)
			if !ok {
				return NewResourceError("reading bucket quota", bucket, fmt.Errorf("quota value overflows int64: %d", bucketQuota.Size))
			}
			quotaVal = int(q)
			quotaType = string(bucketQuota.Type)
		}
	case isS3CompatNotSupported(client, s3Compat405IsMissingFeature, err):
		diags = append(diags, dataSourceS3CompatGap(bucket, "quota", "bucket quota"))
	default:
		return NewResourceError("reading bucket quota", bucket, err)
	}

	d.SetId(bucket)
	if err := d.Set("quota", quotaVal); err != nil {
		return NewResourceError("setting quota", bucket, err)
	}
	if err := d.Set("type", quotaType); err != nil {
		return NewResourceError("setting type", bucket, err)
	}
	return diags
}
