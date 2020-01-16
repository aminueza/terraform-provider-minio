package minio

import (
	"github.com/hashicorp/terraform/helper/schema"
)

//BucketConfig creates a new config for minio buckets
func BucketConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucket {
	m := meta.(*S3MinioClient)

	return &S3MinioBucket{
		MinioClient:   m.S3Client,
		MinioAdmin:    m.S3Admin,
		MinioRegion:   m.S3Region,
		MinioAccess:   m.S3UserAccess,
		S3MinioBucket: d.Get("bucket").(string),
		MinioACL:      d.Get("acl").(string),
	}
}

//NewConfig creates a new config for minio
func NewConfig(d *schema.ResourceData) *S3MinioConfig {
	return &S3MinioConfig{
		S3HostPort:     d.Get("minio_server").(string),
		S3Region:       d.Get("minio_region").(string),
		S3UserAccess:   d.Get("minio_access_key").(string),
		S3UserSecret:   d.Get("minio_secret_key").(string),
		S3APISignature: d.Get("minio_api_version").(string),
		S3SSL:          d.Get("minio_ssl").(bool),
	}
}

//IAMUserConfig creates new user config
func IAMUserConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMUserConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMUserConfig{
		MinioAdmin:        m.S3Admin,
		MinioIAMName:      d.Get("name").(string),
		MinioDisableUser:  d.Get("disable_user").(bool),
		MinioUpdateKey:    d.Get("update_secret").(bool),
		MinioForceDestroy: d.Get("force_destroy").(bool),
	}
}
