package minio

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func Provider() *schema.Provider {
	return newProvider()
}

func newProvider(envVarPrefix ...string) *schema.Provider {
	prefix := ""
	if len(envVarPrefix) > 0 {
		prefix = envVarPrefix[0]
	}

	p := &schema.Provider{
		Schema: map[string]*schema.Schema{
			"minio_server": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "MinIO server endpoint in the format host:port",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_ENDPOINT",
				}, nil),
			},
			"minio_region": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "us-east-1",
				Description: "MinIO server region (default: us-east-1)",
			},
			"minio_user": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "MinIO user (or access key) for authentication",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_USER",
				}, nil),
				ConflictsWith: []string{"minio_access_key"},
			},
			"minio_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "MinIO password (or secret key) for authentication",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_PASSWORD",
				}, nil),
				ConflictsWith: []string{"minio_secret_key"},
			},
			"minio_access_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "MinIO access key (deprecated: use minio_user instead)",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_ACCESS_KEY",
				}, nil),
				Deprecated: "use minio_user instead",
			},
			"minio_secret_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "MinIO secret key (deprecated: use minio_password instead)",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_SECRET_KEY",
				}, nil),
				Deprecated: "use minio_password instead",
			},
			"minio_session_token": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "MinIO session token for temporary credentials",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_SESSION_TOKEN",
				}, ""),
			},
			"minio_api_version": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "v4",
				Description:  "MinIO API Version (v2 or v4)",
				ValidateFunc: validateAPIVersion,
			},
			"minio_ssl": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable SSL/TLS for MinIO connection",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_ENABLE_HTTPS",
				}, false),
			},
			"minio_insecure": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Skip SSL certificate verification (not recommended for production)",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_INSECURE",
				}, false),
			},
			"minio_cacert_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to CA certificate file for SSL verification",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_CACERT_FILE",
				}, nil),
			},
			"minio_cert_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to client certificate file for SSL authentication",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_CERT_FILE",
				}, nil),
			},
			"minio_key_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to client private key file for SSL authentication",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_KEY_FILE",
				}, nil),
			},
			"minio_debug": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable debug logging for API requests",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_DEBUG",
				}, false),
			},
		},

		DataSourcesMap: map[string]*schema.Resource{
			"minio_iam_policy_document": dataSourceMinioIAMPolicyDocument(),
			"minio_s3_object":           dataSourceMinioS3Object(),
			"minio_iam_user":            dataSourceIAMUser(),
			"minio_iam_users":           dataSourceIAMUsers(),
			"minio_server_info":         dataSourceMinioServerInfo(),
			"minio_health_status":       dataSourceMinioHealthStatus(),
		},

		ResourcesMap: map[string]*schema.Resource{
			// S3 Bucket Operations
			"minio_s3_bucket":                           resourceMinioBucket(),
			"minio_s3_bucket_policy":                    resourceMinioBucketPolicy(),
			"minio_s3_bucket_anonymous_access":          resourceMinioS3BucketAnonymousAccess(),
			"minio_s3_bucket_versioning":                resourceMinioBucketVersioning(),
			"minio_s3_bucket_replication":               resourceMinioBucketReplication(),
			"minio_s3_bucket_retention":                 resourceMinioBucketRetention(),
			"minio_s3_bucket_object_lock_configuration": resourceMinioS3BucketObjectLockConfiguration(),
			"minio_s3_bucket_notification":              resourceMinioBucketNotification(),
			"minio_s3_bucket_server_side_encryption":    resourceMinioBucketServerSideEncryption(),
			"minio_s3_bucket_cors":                      resourceMinioS3BucketCors(),
			"minio_s3_bucket_quota":                     resourceMinioBucketQuota(),
			"minio_s3_bucket_tags":                      resourceMinioBucketTags(),
			"minio_s3_object_tags":                      resourceMinioObjectTags(),
			"minio_s3_object":                           resourceMinioObject(),

			// IAM Operations
			"minio_iam_group":                   resourceMinioIAMGroup(),
			"minio_iam_group_membership":        resourceMinioIAMGroupMembership(),
			"minio_iam_user":                    resourceMinioIAMUser(),
			"minio_iam_service_account":         resourceMinioServiceAccount(),
			"minio_iam_group_policy":            resourceMinioIAMGroupPolicy(),
			"minio_iam_policy":                  resourceMinioIAMPolicy(),
			"minio_iam_user_policy_attachment":  resourceMinioIAMUserPolicyAttachment(),
			"minio_iam_group_policy_attachment": resourceMinioIAMGroupPolicyAttachment(),
			"minio_iam_group_user_attachment":   resourceMinioIAMGroupUserAttachment(),
			"minio_iam_user_group_membership":   resourceMinioIAMUserGroupMembership(),

			// LDAP Operations
			"minio_iam_ldap_group_policy_attachment": resourceMinioIAMLDAPGroupPolicyAttachment(),
			"minio_iam_ldap_user_policy_attachment":  resourceMinioIAMLDAPUserPolicyAttachment(),

			// ILM and KMS Operations
			"minio_ilm_policy": resourceMinioILMPolicy(),
			"minio_ilm_tier":   resourceMinioILMTier(),
			"minio_kms_key":    resourceMinioKMSKey(),

			// AccessKey Operations
			"minio_accesskey": resourceMinioAccessKey(),

			// Server Configuration
			"minio_config":           resourceMinioConfig(),
			"minio_site_replication": resourceMinioSiteReplication(),
		},

		ConfigureContextFunc: providerConfigure,
	}

	return p
}

func validateAPIVersion(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	if value != "v2" && value != "v4" {
		errors = append(errors, fmt.Errorf("%q must be either 'v2' or 'v4', got: %s", k, value))
	}
	return
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	minioConfig := NewConfig(d)
	client, err := minioConfig.NewClient()
	if err != nil {
		return nil, NewResourceError("Failed to create MinIO client", "client_creation", err)
	}

	return client, nil
}
