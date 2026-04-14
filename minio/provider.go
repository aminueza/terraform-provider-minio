package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider returns the SDK provider with only data sources
// This is used for backward compatibility during v4 migration
func Provider() *schema.Provider {
	prefix := ""

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
				Type:        schema.TypeString,
				Optional:    true,
				Description: "MinIO API Version (v2 or v4)",
			},
			"minio_ssl": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable SSL/TLS for MinIO connection",
			},
			"minio_insecure": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Skip SSL certificate verification (not recommended for production)",
			},
			"minio_cacert_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to CA certificate file for SSL verification",
			},
			"minio_cert_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to client certificate file for SSL authentication",
			},
			"minio_key_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to client private key file for SSL authentication",
			},
			"minio_debug": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable debug logging for API requests",
			},
			"skip_bucket_tagging": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Skip bucket tagging API calls. Useful when your S3-compatible endpoint does not support tagging.",
			},
			"s3_compat_mode": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable S3 compatibility mode for non-MinIO backends (Hetzner, Cloudflare R2, Backblaze B2, DigitalOcean Spaces). Gracefully handles unsupported S3 features instead of erroring.",
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					prefix + "MINIO_S3_COMPAT_MODE",
				}, false),
			},
			"request_timeout_seconds": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     30,
				Description: "Global HTTP request timeout in seconds for all MinIO API calls (default: 30)",
				DefaultFunc: schema.EnvDefaultFunc(prefix+"MINIO_REQUEST_TIMEOUT_SECONDS", 30),
			},
			"max_retries": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     6,
				Description: "Maximum number of retries for failed operations (default: 6)",
				DefaultFunc: schema.EnvDefaultFunc(prefix+"MINIO_MAX_RETRIES", 6),
			},
			"retry_delay_ms": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     1000,
				Description: "Base delay in milliseconds between retries, used with exponential backoff (default: 1000)",
				DefaultFunc: schema.EnvDefaultFunc(prefix+"MINIO_RETRY_DELAY_MS", 1000),
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

		ResourcesMap: map[string]*schema.Resource{},

		ConfigureContextFunc: providerConfigure,
	}

	return p
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	minioConfig := NewConfig(d)
	client, err := minioConfig.NewClient()
	if err != nil {
		return nil, NewResourceError("Failed to create MinIO client", "client_creation", err)
	}

	return client, nil
}
