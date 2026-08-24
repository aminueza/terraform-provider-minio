package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

func dataSourceMinioS3BucketQuota() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the quota configuration of an existing S3 bucket.",
		ReadContext:        dataSourceMinioS3BucketQuotaRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true},
			"quota":  {Type: schema.TypeInt, Computed: true},
			"type":   {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketQuotaRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	bucket := d.Get("bucket").(string)

	d.SetId(bucket)

	bucketQuota, err := admin.GetBucketQuota(ctx, bucket)
	if err != nil || bucketQuota.Size == 0 {
		_ = d.Set("quota", 0)
		_ = d.Set("type", "")
		return nil
	}

	quotaVal, ok := SafeUint64ToInt64(bucketQuota.Size)
	if !ok {
		return diag.Errorf("quota value overflows int64: %d", bucketQuota.Size)
	}
	_ = d.Set("quota", int(quotaVal))
	_ = d.Set("type", string(bucketQuota.Type))
	return nil
}
