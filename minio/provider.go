package minio

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

//Provider creates a new provider
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"minio_server": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Minio Host and Port",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"MINIO_ENDPOINT",
				}, nil),
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
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"MINIO_ACCESS_KEY",
				}, nil),
			},
			"minio_secret_key": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Minio Secret Key",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"MINIO_SECRET_KEY",
				}, nil),
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
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"MINIO_ENABLE_HTTPS",
				}, nil),
			},
		},

		DataSourcesMap: map[string]*schema.Resource{
			"minio_iam_policy_document": dataSourceMinioIAMPolicyDocument(),
		},

		ResourcesMap: map[string]*schema.Resource{
			"minio_s3_bucket": resourceMinioBucket(),
			"minio_s3_object": resourceMinioObject(),
			// "minio_s3_file":   resourceMinioFile(),
			"minio_iam_group":                   resourceMinioIAMGroup(),
			"minio_iam_group_membership":        resourceMinioIAMGroupMembership(),
			"minio_iam_user":                    resourceMinioIAMUser(),
			"minio_iam_group_policy":            resourceMinioIAMGroupPolicy(),
			"minio_iam_policy":                  resourceMinioIAMPolicy(),
			"minio_iam_user_policy_attachment":  resourceMinioIAMUserPolicyAttachment(),
			"minio_iam_group_policy_attachment": resourceMinioIAMGroupPolicyAttachment(),
			"minio_iam_group_user_attachment":   resourceMinioIAMGroupUserAttachment(),
		},

		ConfigureFunc: providerConfigure,
	}
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {
	minioConfig := NewConfig(d)
	return minioConfig.NewClient()
}
