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
			"assume_role": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: "Use STS AssumeRole to obtain temporary credentials. When configured, the provider exchanges the static credentials for short-lived session credentials.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"role_arn": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "ARN of the role to assume.",
							DefaultFunc: schema.EnvDefaultFunc(prefix+"MINIO_ASSUME_ROLE_ARN", ""),
						},
						"session_name": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "terraform",
							Description: "Session name for the assumed role.",
						},
						"duration_seconds": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     3600,
							Description: "Duration in seconds for the session (default: 3600).",
						},
						"policy": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "IAM policy in JSON format to scope down the assumed role permissions.",
						},
						"external_id": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "External ID for cross-account role assumption.",
						},
					},
				},
			},
			"assume_role_with_web_identity": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: "Use STS AssumeRoleWithWebIdentity to obtain credentials from an OIDC token (e.g., GitHub Actions, GitLab CI).",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"web_identity_token": {
							Type:        schema.TypeString,
							Optional:    true,
							Sensitive:   true,
							Description: "OIDC/JWT token for web identity authentication.",
							DefaultFunc: schema.EnvDefaultFunc(prefix+"MINIO_WEB_IDENTITY_TOKEN", ""),
						},
						"web_identity_token_file": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Path to a file containing the OIDC/JWT token.",
							DefaultFunc: schema.EnvDefaultFunc(prefix+"MINIO_WEB_IDENTITY_TOKEN_FILE", ""),
						},
						"duration_seconds": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     3600,
							Description: "Duration in seconds for the session (default: 3600).",
						},
					},
				},
			},
		},

		DataSourcesMap: map[string]*schema.Resource{
			"minio_iam_policy_document":      dataSourceMinioIAMPolicyDocument(),
			"minio_s3_object":                dataSourceMinioS3Object(),
			"minio_iam_user":                 dataSourceIAMUser(),
			"minio_iam_users":                dataSourceIAMUsers(),
			"minio_server_info":              dataSourceMinioServerInfo(),
			"minio_health_status":            dataSourceMinioHealthStatus(),
			"minio_prometheus_scrape_config":      dataSourceMinioPrometheusScrapeConfig(),
			"minio_iam_group":                    dataSourceIAMGroup(),
			"minio_iam_groups":                   dataSourceIAMGroups(),
			"minio_iam_policy":                   dataSourceIAMPolicy(),
			"minio_s3_buckets":                   dataSourceMinioS3Buckets(),
			"minio_ilm_tiers":                    dataSourceMinioILMTiers(),
			"minio_iam_service_accounts":         dataSourceIAMServiceAccounts(),
			"minio_license_info":                 dataSourceMinioLicenseInfo(),
			"minio_s3_bucket_tags":               dataSourceMinioS3BucketTags(),
			"minio_s3_bucket":                    dataSourceMinioS3Bucket(),
			"minio_s3_bucket_replication_status":      dataSourceMinioS3BucketReplicationStatus(),
			"minio_s3_bucket_versioning":              dataSourceMinioS3BucketVersioning(),
			"minio_s3_bucket_encryption":              dataSourceMinioS3BucketEncryption(),
			"minio_s3_bucket_notification_config":     dataSourceMinioS3BucketNotificationConfig(),
			"minio_s3_bucket_cors_config":             dataSourceMinioS3BucketCorsConfig(),
			"minio_s3_bucket_retention":               dataSourceMinioS3BucketRetention(),
			"minio_s3_bucket_quota":                   dataSourceMinioS3BucketQuota(),
			"minio_s3_bucket_object_lock_configuration": dataSourceMinioS3BucketObjectLockConfiguration(),
			"minio_s3_bucket_replication":              dataSourceMinioS3BucketReplication(),
			"minio_ilm_policy":                        dataSourceMinioILMPolicy(),
			"minio_iam_user_policies":            dataSourceIAMUserPolicies(),
			"minio_s3_bucket_policy":             dataSourceMinioS3BucketPolicy(),
			"minio_account_info":                dataSourceMinioAccountInfo(),
			"minio_storage_info":                dataSourceMinioStorageInfo(),
			"minio_data_usage":                  dataSourceMinioDataUsage(),
			"minio_ilm_tier_stats":              dataSourceMinioILMTierStats(),
			"minio_s3_objects":                   dataSourceMinioS3Objects(),

			// Notification Targets
			"minio_notify_webhook":       dataSourceMinioNotifyWebhook(),
			"minio_notify_amqp":          dataSourceMinioNotifyAmqp(),
			"minio_notify_kafka":         dataSourceMinioNotifyKafka(),
			"minio_notify_mqtt":          dataSourceMinioNotifyMqtt(),
			"minio_notify_nats":          dataSourceMinioNotifyNats(),
			"minio_notify_nsq":           dataSourceMinioNotifyNsq(),
			"minio_notify_mysql":         dataSourceMinioNotifyMysql(),
			"minio_notify_postgres":      dataSourceMinioNotifyPostgres(),
			"minio_notify_elasticsearch": dataSourceMinioNotifyElasticsearch(),
			"minio_notify_redis":         dataSourceMinioNotifyRedis(),
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
			"minio_s3_object_legal_hold":                resourceMinioObjectLegalHold(),
			"minio_s3_object_retention":                 resourceMinioObjectRetention(),
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

			// Identity Provider Operations
			"minio_iam_idp_openid": resourceMinioIAMIdpOpenId(),
			"minio_iam_idp_ldap":   resourceMinioIAMIdpLdap(),

			// ILM and KMS Operations
			"minio_ilm_policy": resourceMinioILMPolicy(),
			"minio_ilm_tier":   resourceMinioILMTier(),
			"minio_kms_key":    resourceMinioKMSKey(),

			// AccessKey Operations
			"minio_accesskey": resourceMinioAccessKey(),

			// Server Configuration
			"minio_config":                  resourceMinioConfig(),
			"minio_audit_webhook":           resourceMinioAuditWebhook(),
			"minio_server_config_api":           resourceMinioServerConfigApi(),
			"minio_server_config_region":        resourceMinioServerConfigRegion(),
			"minio_server_config_scanner":       resourceMinioServerConfigScanner(),
			"minio_server_config_heal":          resourceMinioServerConfigHeal(),
			"minio_server_config_storage_class": resourceMinioServerConfigStorageClass(),
			"minio_server_config_etcd":          resourceMinioServerConfigEtcd(),
			"minio_logger_webhook":              resourceMinioLoggerWebhook(),
			"minio_audit_kafka":                 resourceMinioAuditKafka(),
			"minio_site_replication":        resourceMinioSiteReplication(),

			// Notification Targets
			"minio_notify_webhook":       resourceMinioNotifyWebhook(),
			"minio_notify_amqp":          resourceMinioNotifyAmqp(),
			"minio_notify_kafka":         resourceMinioNotifyKafka(),
			"minio_notify_mqtt":          resourceMinioNotifyMqtt(),
			"minio_notify_nats":          resourceMinioNotifyNats(),
			"minio_notify_nsq":           resourceMinioNotifyNsq(),
			"minio_notify_mysql":         resourceMinioNotifyMysql(),
			"minio_notify_postgres":      resourceMinioNotifyPostgres(),
			"minio_notify_elasticsearch": resourceMinioNotifyElasticsearch(),
			"minio_notify_redis":         resourceMinioNotifyRedis(),
			"minio_prometheus_bearer_token": resourceMinioPrometheusBearerToken(),
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
