package s3minio

import (
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"

	minioConfig "github.com/aminueza/terraform-minio-provider/s3minio"
)

//Provider creates a new provider
func Provider() terraform.ResourceProvider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"minio_server": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Minio Host and Port",
			},
			"minio_region": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "us-east-1",
				Description: "Minio Region (default: us-east-1)",
			},
			"minio_access_key": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Minio Access Key",
			},
			"minio_secret_key": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Minio Secret Key",
			},
			"minio_api_version": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "v4",
				Description: "Minio API Version (type: string, options: v2 or v4, default: v4)",
			},
			"minio_ssl": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Minio SSL enabled (default: false)",
			},
			"minio_debug": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Minio debug mode (default: false)",
			},
		},

		ResourcesMap: map[string]*schema.Resource{
			"minio_bucket": resourceMinioBucket(),
			"minio_object": resourceMinioObject(),
			"minio_file":   resourceMinioFile(),
		},

		ConfigureFunc: providerConfigure,
	}
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {
	minioConfig.NewConfig(d)
	return minioConfig.NewClient()
}
