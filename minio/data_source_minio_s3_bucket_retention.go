package minio

import (
	"context"
	"math"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketRetention() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the object lock retention configuration of an existing S3 bucket.",
		Read:        dataSourceMinioS3BucketRetentionRead,
		Schema: map[string]*schema.Schema{
			"bucket":          {Type: schema.TypeString, Required: true},
			"mode":            {Type: schema.TypeString, Computed: true},
			"unit":            {Type: schema.TypeString, Computed: true},
			"validity_period": {Type: schema.TypeInt, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketRetentionRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	d.SetId(bucket)

	mode, validity, unit, err := client.GetBucketObjectLockConfig(context.Background(), bucket)
	if err != nil || mode == nil || validity == nil || unit == nil {
		_ = d.Set("mode", "")
		_ = d.Set("unit", "")
		_ = d.Set("validity_period", 0)
		return nil
	}

	_ = d.Set("mode", mode.String())
	_ = d.Set("unit", unit.String())
	validityInt := math.MaxInt
	if *validity <= uint(math.MaxInt) {
		validityInt = int(*validity)
	}
	_ = d.Set("validity_period", validityInt)
	return nil
}
