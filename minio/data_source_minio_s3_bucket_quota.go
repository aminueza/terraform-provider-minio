package minio

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketQuota() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the quota configuration of an existing S3 bucket.",
		Read:        dataSourceMinioS3BucketQuotaRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true},
			"quota":  {Type: schema.TypeInt, Computed: true},
			"type":   {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketQuotaRead(d *schema.ResourceData, meta interface{}) error {
	admin := meta.(*S3MinioClient).S3Admin
	bucket := d.Get("bucket").(string)

	d.SetId(bucket)

	bucketQuota, err := admin.GetBucketQuota(context.Background(), bucket)
	if err != nil || bucketQuota.Quota == 0 {
		_ = d.Set("quota", 0)
		_ = d.Set("type", "")
		return nil
	}

	quotaVal, ok := SafeUint64ToInt64(bucketQuota.Quota)
	if !ok {
		return fmt.Errorf("quota value overflows int64: %d", bucketQuota.Quota)
	}
	_ = d.Set("quota", int(quotaVal))
	_ = d.Set("type", string(bucketQuota.Type))
	return nil
}
