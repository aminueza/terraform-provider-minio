package pactions

import (
	minioConfig "../mconfig"
	"github.com/hashicorp/terraform/helper/schema"
)

//BucketConfig creates a new config for minio buckets
func BucketConfig(d *schema.ResourceData, meta interface{}) *MinioBucket {
	m := meta.(*minioConfig.S3MinioClient)

	return &MinioBucket{
		MinioClient: m.S3Client,
		MinioRegion: m.S3Region,
		MinioAccess: m.S3UserAccess,
		MinioBucket: d.Get("name").(string),
		MinioDebug:  d.Get("debug_mode").(string),
		MinioACL:    d.Get("acl").(string),
	}
}
