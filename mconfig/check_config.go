package mconfig

import (
	"github.com/hashicorp/terraform/helper/schema"
)

//NewConfig creates a new config for minio
func NewConfig(d *schema.ResourceData) *MinioConfig {
	return &MinioConfig{
		S3HostPort:     d.Get("minio_server").(string),
		S3Region:       d.Get("minio_region").(string),
		S3UserAccess:   d.Get("minio_access_key").(string),
		S3UserSecret:   d.Get("minio_secret_key").(string),
		S3APISignature: d.Get("minio_api_version").(string),
		S3SSL:          d.Get("minio_ssl").(string),
		S3Debug:        d.Get("minio_debug").(string),
	}
}
