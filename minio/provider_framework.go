package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// minioFrameworkProvider defines the provider implementation
type minioFrameworkProvider struct {
	version string
}

// NewFrameworkProvider creates a new framework provider instance
func NewFrameworkProvider(version string) func() provider.Provider {
	return func() provider.Provider {
		return &minioFrameworkProvider{version: version}
	}
}

// Metadata returns provider metadata
func (p *minioFrameworkProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "minio"
	resp.Version = p.version
}

// Schema returns the provider schema - simplified for v4 migration
// Note: Deprecated attributes removed for framework compatibility
// This is a breaking change acceptable for v4 major version
func (p *minioFrameworkProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"minio_server": schema.StringAttribute{
				Required:    true,
				Description: "MinIO server endpoint in the format host:port",
			},
			"minio_region": schema.StringAttribute{
				Optional:    true,
				Description: "MinIO server region (default: us-east-1)",
			},
			"minio_user": schema.StringAttribute{
				Optional:    true,
				Description: "MinIO user (or access key) for authentication",
			},
			"minio_password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "MinIO password (or secret key) for authentication",
			},
			"minio_session_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "MinIO session token for temporary credentials",
			},
			"minio_api_version": schema.StringAttribute{
				Optional:    true,
				Description: "MinIO API Version (v2 or v4)",
			},
			"minio_ssl": schema.BoolAttribute{
				Optional:    true,
				Description: "Enable SSL/TLS for MinIO connection",
			},
			"minio_insecure": schema.BoolAttribute{
				Optional:    true,
				Description: "Skip SSL certificate verification (not recommended for production)",
			},
			"minio_cacert_file": schema.StringAttribute{
				Optional:    true,
				Description: "Path to CA certificate file for SSL verification",
			},
			"minio_cert_file": schema.StringAttribute{
				Optional:    true,
				Description: "Path to client certificate file for SSL authentication",
			},
			"minio_key_file": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Path to client private key file for SSL authentication",
			},
			"minio_debug": schema.BoolAttribute{
				Optional:    true,
				Description: "Enable debug logging for API requests",
			},
			"skip_bucket_tagging": schema.BoolAttribute{
				Optional:    true,
				Description: "Skip bucket tagging API calls. Useful when your S3-compatible endpoint does not support tagging.",
			},
			"s3_compat_mode": schema.BoolAttribute{
				Optional:    true,
				Description: "Enable S3 compatibility mode for non-MinIO backends (Hetzner, Cloudflare R2, Backblaze B2, DigitalOcean Spaces). Gracefully handles unsupported S3 features instead of erroring.",
			},
		},
	}
}

// Configure sets up the provider
func (p *minioFrameworkProvider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
}

// Resources returns the list of resources
func (p *minioFrameworkProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newS3BucketResource,
		newIAMUserResource,
		newIAMPolicyResource,
		newServiceAccountResource,
		newIAMGroupResource,
		newIAMGroupMembershipResource,
		newS3ObjectResource,
		newIAMUserPolicyAttachmentResource,
		newIAMGroupPolicyAttachmentResource,
		newIAMLDAPUserPolicyAttachmentResource,
		newIAMLDAPGroupPolicyAttachmentResource,
		newIAMGroupUserAttachmentResource,
		newS3ObjectTagsResource,
		newS3ObjectLegalHoldResource,
		newS3ObjectRetentionResource,
		newBucketPolicyResource,
		newBucketVersioningResource,
		newBucketEncryptionResource,
		newBucketObjectLockConfigurationResource,
		newBucketQuotaResource,
		newBucketTagsResource,
		newBucketCorsResource,
		newBucketRetentionResource,
		newBucketNotificationResource,
		newILMPolicyResource,
		newILMTierResource,
		newIAMGroupPolicyResource,
		newIAMUserGroupMembershipResource,
		newKMSKeyResource,
		newConfigResource,
		newServerConfigRegionResource,
		newServerConfigHealResource,
		newServerConfigStorageClassResource,
		newServerConfigScannerResource,
		newServerConfigApiResource,
		newServerConfigEtcdResource,
		newAccessKeyResource,
		newPrometheusBearerTokenResource,
		newIAMIdpLdapResource,
		newS3BucketAnonymousAccessResource,
		newSiteReplicationResource,
		newBucketReplicationResource(),
		resourceMinioNotifyWebhookFramework,
		resourceMinioNotifyAmqpFramework,
		resourceMinioNotifyKafkaFramework,
		resourceMinioNotifyMqttFramework,
		resourceMinioNotifyNatsFramework,
		resourceMinioNotifyNsqFramework,
		resourceMinioNotifyMysqlFramework,
		resourceMinioNotifyPostgresFramework,
		resourceMinioNotifyElasticsearchFramework,
		resourceMinioNotifyRedisFramework,
		resourceMinioAuditWebhookFramework,
		resourceMinioAuditKafkaFramework,
		resourceMinioLoggerWebhookFramework,
	}
}

// DataSources returns the list of data sources
func (p *minioFrameworkProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newS3BucketDataSource,
	}
}
