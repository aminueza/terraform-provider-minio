package minio

import (
	"context"
	"math"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketRetention() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the object lock retention configuration of an existing S3 bucket.",
		ReadContext: dataSourceMinioS3BucketRetentionRead,
		Schema: map[string]*schema.Schema{
			"bucket":          {Type: schema.TypeString, Required: true},
			"mode":            {Type: schema.TypeString, Computed: true},
			"unit":            {Type: schema.TypeString, Computed: true},
			"validity_period": {Type: schema.TypeInt, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketRetentionRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	d.SetId(bucket)

	mode, validity, unit, err := client.GetBucketObjectLockConfig(ctx, bucket)
	if err != nil || mode == nil || validity == nil || unit == nil {
		if diags := setResourceField(d, "mode", "", bucket); diags.HasError() {
			return diags
		}
		if diags := setResourceField(d, "unit", "", bucket); diags.HasError() {
			return diags
		}
		return setResourceField(d, "validity_period", 0, bucket)
	}

	if diags := setResourceField(d, "mode", mode.String(), bucket); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "unit", unit.String(), bucket); diags.HasError() {
		return diags
	}
	validityInt := math.MaxInt
	if *validity <= uint(math.MaxInt) {
		validityInt = int(*validity)
	}
	return setResourceField(d, "validity_period", validityInt, bucket)
}
