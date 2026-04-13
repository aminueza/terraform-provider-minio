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
				Optional:    true,
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
			},
			"minio_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "MinIO password (or secret key) for authentication",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_PASSWORD",
				}, nil),
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
			"skip_bucket_tagging": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Skip bucket tagging API calls. Useful when your S3-compatible endpoint does not support tagging.",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_SKIP_BUCKET_TAGGING",
				}, false),
			},
			"s3_compat_mode": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable S3 compatibility mode for non-MinIO backends (Hetzner, Cloudflare R2, Backblaze B2, DigitalOcean Spaces). Gracefully handles unsupported S3 features instead of erroring.",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_S3_COMPAT_MODE",
				}, false),
			},
		},

		DataSourcesMap: map[string]*schema.Resource{
			"minio_iam_policy_document":                 dataSourceMinioIAMPolicyDocument(),
			"minio_s3_object":                           dataSourceMinioS3Object(),
			"minio_iam_user":                            dataSourceIAMUser(),
			"minio_iam_users":                           dataSourceIAMUsers(),
			"minio_server_info":                         dataSourceMinioServerInfo(),
			"minio_health_status":                       dataSourceMinioHealthStatus(),
			"minio_prometheus_scrape_config":            dataSourceMinioPrometheusScrapeConfig(),
			"minio_iam_group":                           dataSourceIAMGroup(),
			"minio_iam_groups":                          dataSourceIAMGroups(),
			"minio_iam_policy":                          dataSourceIAMPolicy(),
			"minio_s3_buckets":                          dataSourceMinioS3Buckets(),
			"minio_ilm_tiers":                           dataSourceMinioILMTiers(),
			"minio_iam_service_accounts":                dataSourceIAMServiceAccounts(),
			"minio_license_info":                        dataSourceMinioLicenseInfo(),
			"minio_s3_bucket_tags":                      dataSourceMinioS3BucketTags(),
			"minio_s3_bucket":                           dataSourceMinioS3Bucket(),
			"minio_s3_bucket_replication_status":        dataSourceMinioS3BucketReplicationStatus(),
			"minio_s3_bucket_versioning":                dataSourceMinioS3BucketVersioning(),
			"minio_s3_bucket_encryption":                dataSourceMinioS3BucketEncryption(),
			"minio_s3_bucket_notification_config":       dataSourceMinioS3BucketNotificationConfig(),
			"minio_s3_bucket_cors_config":               dataSourceMinioS3BucketCorsConfig(),
			"minio_s3_bucket_retention":                 dataSourceMinioS3BucketRetention(),
			"minio_s3_bucket_quota":                     dataSourceMinioS3BucketQuota(),
			"minio_s3_bucket_object_lock_configuration": dataSourceMinioS3BucketObjectLockConfiguration(),
			"minio_s3_bucket_replication":               dataSourceMinioS3BucketReplication(),
			"minio_ilm_policy":                          dataSourceMinioILMPolicy(),
			"minio_iam_user_policies":                   dataSourceIAMUserPolicies(),
			"minio_s3_bucket_policy":                    dataSourceMinioS3BucketPolicy(),
			"minio_account_info":                        dataSourceMinioAccountInfo(),
			"minio_storage_info":                        dataSourceMinioStorageInfo(),
			"minio_data_usage":                          dataSourceMinioDataUsage(),
			"minio_ilm_tier_stats":                      dataSourceMinioILMTierStats(),
			"minio_s3_objects":                          dataSourceMinioS3Objects(),
		},

		ResourcesMap: map[string]*schema.Resource{
			// Identity Provider Operations - not yet migrated to framework
			"minio_iam_idp_openid": resourceMinioIAMIdpOpenId(),
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
