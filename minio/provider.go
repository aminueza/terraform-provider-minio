package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider creates a new provider
func Provider() *schema.Provider {
	return newProvider()
}

func newProvider(envvarPrefixed ...string) *schema.Provider {
	envVarPrefix := ""
	if len(envvarPrefixed) != 0 {
		envVarPrefix = envvarPrefixed[0]
	}
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"minio_server": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Minio Host and Port",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					envVarPrefix + "MINIO_ENDPOINT",
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
				Optional:    true,
				Description: "Minio Access Key",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					envVarPrefix + "MINIO_ACCESS_KEY",
				}, nil),
				Deprecated: "use minio_user instead",
			},
			"minio_secret_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Minio Secret Key",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					envVarPrefix + "MINIO_SECRET_KEY",
				}, nil),
				Deprecated: "use minio_password instead",
			},
			"minio_user": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Minio User (or access key)",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					envVarPrefix + "MINIO_USER",
				}, nil),
				ConflictsWith: []string{"minio_access_key"},
			},
			"minio_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Minio Password (or secret key)",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					envVarPrefix + "MINIO_PASSWORD",
				}, nil),
				ConflictsWith: []string{"minio_secret_key"},
			},
			"minio_session_token": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Minio Session Token",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					envVarPrefix + "MINIO_SESSION_TOKEN",
				}, ""),
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
				Description: "Minio SSL enabled (default: false)",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					envVarPrefix + "MINIO_ENABLE_HTTPS",
				}, nil),
			},
			"minio_insecure": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Disable SSL certificate verification (default: false)",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					envVarPrefix + "MINIO_INSECURE",
				}, nil),
			},
			"minio_cacert_file": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					envVarPrefix + "MINIO_CACERT_FILE",
				}, nil),
			},
			"minio_cert_file": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					envVarPrefix + "MINIO_CERT_FILE",
				}, nil),
			},
			"minio_key_file": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					envVarPrefix + "MINIO_KEY_FILE",
				}, nil),
			},
		},

		DataSourcesMap: map[string]*schema.Resource{
			"minio_iam_policy_document": dataSourceMinioIAMPolicyDocument(),
			"minio_s3_object":           dataSourceMinioS3Object(),
		},

		ResourcesMap: map[string]*schema.Resource{
			"minio_s3_bucket":                        resourceMinioBucket(),
			"minio_s3_bucket_policy":                 resourceMinioBucketPolicy(),
			"minio_s3_bucket_versioning":             resourceMinioBucketVersioning(),
			"minio_s3_bucket_replication":            resourceMinioBucketReplication(),
			"minio_s3_bucket_notification":           resourceMinioBucketNotification(),
			"minio_s3_bucket_server_side_encryption": resourceMinioBucketServerSideEncryption(),
			"minio_s3_object":                        resourceMinioObject(),
			"minio_iam_group":                        resourceMinioIAMGroup(),
			"minio_iam_group_membership":             resourceMinioIAMGroupMembership(),
			"minio_iam_user":                         resourceMinioIAMUser(),
			"minio_iam_service_account":              resourceMinioServiceAccount(),
			"minio_iam_group_policy":                 resourceMinioIAMGroupPolicy(),
			"minio_iam_policy":                       resourceMinioIAMPolicy(),
			"minio_iam_user_policy_attachment":       resourceMinioIAMUserPolicyAttachment(),
			"minio_iam_group_policy_attachment":      resourceMinioIAMGroupPolicyAttachment(),
			"minio_iam_group_user_attachment":        resourceMinioIAMGroupUserAttachment(),
			"minio_iam_ldap_group_policy_attachment": resourceMinioIAMLDAPGroupPolicyAttachment(),
			"minio_iam_ldap_user_policy_attachment":  resourceMinioIAMLDAPUserPolicyAttachment(),
			"minio_ilm_policy":                       resourceMinioILMPolicy(),
			"minio_kms_key":                          resourceMinioKMSKey(),
			"minio_ilm_tier":                         resourceMinioILMTier(),
		},

		ConfigureContextFunc: providerConfigure,
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	minioConfig := NewConfig(d)
	client, err := minioConfig.NewClient()
	if err != nil {
		return nil, NewResourceError("client creation failed", "client", err)
	}

	return client, nil
}
