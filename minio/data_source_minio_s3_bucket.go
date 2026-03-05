package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3Bucket() *schema.Resource {
	return &schema.Resource{
		Description: "Reads properties of an existing S3 bucket including versioning, region, and object lock status.",
		Read:        dataSourceMinioS3BucketRead,
		Schema: map[string]*schema.Schema{
			"bucket":             {Type: schema.TypeString, Required: true},
			"region":             {Type: schema.TypeString, Computed: true},
			"versioning_enabled": {Type: schema.TypeBool, Computed: true},
			"object_lock_enabled": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"policy": {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client

	bucket := d.Get("bucket").(string)
	ctx := context.Background()

	d.SetId(bucket)

	region, err := client.GetBucketLocation(ctx, bucket)
	if err != nil {
		return err
	}
	_ = d.Set("region", region)

	versioning, err := client.GetBucketVersioning(ctx, bucket)
	if err == nil {
		_ = d.Set("versioning_enabled", versioning.Enabled())
	}

	lockConfig, _, _, _, err := client.GetObjectLockConfig(ctx, bucket)
	if err == nil {
		_ = d.Set("object_lock_enabled", lockConfig == "Enabled")
	} else {
		_ = d.Set("object_lock_enabled", false)
	}

	policy, err := client.GetBucketPolicy(ctx, bucket)
	if err == nil {
		_ = d.Set("policy", policy)
	}

	return nil
}
