package minio

import (
	"context"
	"math"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
)

func dataSourceMinioS3BucketObjectLockConfiguration() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the object lock configuration of an existing S3 bucket.",
		Read:        dataSourceMinioS3BucketObjectLockConfigurationRead,
		Schema: map[string]*schema.Schema{
			"bucket":              {Type: schema.TypeString, Required: true},
			"object_lock_enabled": {Type: schema.TypeString, Computed: true},
			"rule": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"default_retention": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"mode":  {Type: schema.TypeString, Computed: true},
									"days":  {Type: schema.TypeInt, Computed: true},
									"years": {Type: schema.TypeInt, Computed: true},
								},
							},
						},
					},
				},
			},
		},
	}
}

func dataSourceMinioS3BucketObjectLockConfigurationRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	d.SetId(bucket)

	objectLockStatus, mode, validity, unit, err := client.GetObjectLockConfig(context.Background(), bucket)
	if err != nil {
		if strings.Contains(err.Error(), "Object Lock configuration does not exist") {
			_ = d.Set("object_lock_enabled", "")
			_ = d.Set("rule", []interface{}{})
			return nil
		}
		return err
	}

	_ = d.Set("object_lock_enabled", objectLockStatus)

	if mode != nil && validity != nil && unit != nil {
		defaultRetention := map[string]interface{}{
			"mode": mode.String(),
		}

		validityInt := math.MaxInt
		if *validity <= uint(math.MaxInt) {
			validityInt = int(*validity)
		}

		switch *unit {
		case minio.Days:
			defaultRetention["days"] = validityInt
		case minio.Years:
			defaultRetention["years"] = validityInt
		}

		_ = d.Set("rule", []interface{}{
			map[string]interface{}{
				"default_retention": []interface{}{defaultRetention},
			},
		})
	} else {
		_ = d.Set("rule", []interface{}{})
	}

	return nil
}
