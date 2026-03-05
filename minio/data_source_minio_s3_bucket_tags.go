package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketTags() *schema.Resource {
	return &schema.Resource{
		Description: "Reads tags from an existing S3 bucket.",
		Read:        dataSourceMinioS3BucketTagsRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true},
			"tags": {
				Type:     schema.TypeMap,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func dataSourceMinioS3BucketTagsRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client

	bucket := d.Get("bucket").(string)
	tagging, err := client.GetBucketTagging(context.Background(), bucket)
	if err != nil {
		d.SetId(bucket)
		_ = d.Set("tags", map[string]string{})
		return nil
	}

	d.SetId(bucket)
	_ = d.Set("tags", tagging.ToMap())

	return nil
}
