package minio

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
				Optional:    true,
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

// Configure sets up the provider - reads config and creates MinIO client
func (p *minioFrameworkProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerModel

	// Read provider configuration
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read environment variables as fallback
	minioServer := getStringOrDefault(data.MinioServer, os.Getenv("MINIO_ENDPOINT"))
	minioRegion := getStringOrDefault(data.MinioRegion, "us-east-1")
	if minioRegion == "us-east-1" && data.MinioRegion.IsNull() {
		if v := os.Getenv("MINIO_REGION"); v != "" {
			minioRegion = v
		}
	}
	minioUser := getStringOrDefault(data.MinioUser, os.Getenv("MINIO_USER"))
	minioPassword := getStringOrDefault(data.MinioPassword, os.Getenv("MINIO_PASSWORD"))
	minioSessionToken := getStringOrDefault(data.MinioSessionToken, os.Getenv("MINIO_SESSION_TOKEN"))
	minioAPIVersion := getStringOrDefault(data.MinioAPIVersion, "v4")
	minioSSL := getBoolOrDefault(data.MinioSSL, false)
	if !minioSSL && data.MinioSSL.IsNull() {
		if v := os.Getenv("MINIO_ENABLE_HTTPS"); v != "" {
			minioSSL = v == "true" || v == "1" || v == "on"
		}
	}
	minioInsecure := getBoolOrDefault(data.MinioInsecure, false)
	if !minioInsecure && data.MinioInsecure.IsNull() {
		if v := os.Getenv("MINIO_INSECURE"); v != "" {
			minioInsecure = v == "true" || v == "1" || v == "on"
		}
	}
	minioCACertFile := getStringOrDefault(data.MinioCACertFile, os.Getenv("MINIO_CACERT_FILE"))
	minioCertFile := getStringOrDefault(data.MinioCertFile, os.Getenv("MINIO_CERT_FILE"))
	minioKeyFile := getStringOrDefault(data.MinioKeyFile, os.Getenv("MINIO_KEY_FILE"))
	minioDebug := getBoolOrDefault(data.MinioDebug, false)
	if !minioDebug && data.MinioDebug.IsNull() {
		if v := os.Getenv("MINIO_DEBUG"); v != "" {
			minioDebug = v == "true" || v == "1" || v == "on"
		}
	}
	skipBucketTagging := getBoolOrDefault(data.SkipBucketTagging, false)
	if !skipBucketTagging && data.SkipBucketTagging.IsNull() {
		if v := os.Getenv("MINIO_SKIP_BUCKET_TAGGING"); v != "" {
			skipBucketTagging = v == "true" || v == "1" || v == "on"
		}
	}
	s3CompatMode := getBoolOrDefault(data.S3CompatMode, false)
	if !s3CompatMode && data.S3CompatMode.IsNull() {
		if v := os.Getenv("MINIO_S3_COMPAT_MODE"); v != "" {
			s3CompatMode = v == "true" || v == "1" || v == "on"
		}
	}

	// Build S3MinioConfig from framework data
	cfg := &S3MinioConfig{
		S3HostPort:        minioServer,
		S3Region:          minioRegion,
		S3UserAccess:      minioUser,
		S3UserSecret:      minioPassword,
		S3SessionToken:    minioSessionToken,
		S3APISignature:    minioAPIVersion,
		S3SSL:             minioSSL,
		S3SSLCACertFile:   minioCACertFile,
		S3SSLCertFile:     minioCertFile,
		S3SSLKeyFile:      minioKeyFile,
		S3SSLSkipVerify:   minioInsecure,
		SkipBucketTagging: skipBucketTagging,
		S3CompatMode:      s3CompatMode,
	}

	// Create MinIO client
	client, err := cfg.NewClient()
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create MinIO client",
			err.Error(),
		)
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

// providerModel represents the provider configuration data
type providerModel struct {
	MinioServer       types.String `tfsdk:"minio_server"`
	MinioRegion       types.String `tfsdk:"minio_region"`
	MinioUser         types.String `tfsdk:"minio_user"`
	MinioPassword     types.String `tfsdk:"minio_password"`
	MinioSessionToken types.String `tfsdk:"minio_session_token"`
	MinioAPIVersion   types.String `tfsdk:"minio_api_version"`
	MinioSSL          types.Bool   `tfsdk:"minio_ssl"`
	MinioInsecure     types.Bool   `tfsdk:"minio_insecure"`
	MinioCACertFile   types.String `tfsdk:"minio_cacert_file"`
	MinioCertFile     types.String `tfsdk:"minio_cert_file"`
	MinioKeyFile      types.String `tfsdk:"minio_key_file"`
	MinioDebug        types.Bool   `tfsdk:"minio_debug"`
	SkipBucketTagging types.Bool   `tfsdk:"skip_bucket_tagging"`
	S3CompatMode      types.Bool   `tfsdk:"s3_compat_mode"`
}

// Helper functions for converting framework types to Go types
func getStringOrDefault(v types.String, defaultVal string) string {
	if v.IsNull() || v.IsUnknown() {
		return defaultVal
	}
	return v.ValueString()
}

func getBoolOrDefault(v types.Bool, defaultVal bool) bool {
	if v.IsNull() || v.IsUnknown() {
		return defaultVal
	}
	return v.ValueBool()
}

// Resources returns the list of resources
// Note: Some resources are temporarily excluded from v4 due to compatibility issues:
//   - Resources with ListNestedAttribute/MapNestedAttribute: Not compatible with protocol v5
//     Affected: bucket_notification, site_replication, bucket_replication
//     (bucket_cors was fixed by converting to ListAttribute with types.Object)
//   - Resources with framework timeouts: Incompatible with protocol v5
//     Affected: config, server_config_*, accesskey, prometheus_bearer_token
//
// These resources will be fixed in follow-up updates by:
// - Converting nested attributes to ListAttribute with types.Object
// - Removing or replacing timeout handling
// Users needing these resources can use v3 until fixes are available
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
		newBucketQuotaResource,
		newBucketTagsResource,
		newBucketRetentionResource,
		newBucketCorsResource,
		newBucketNotificationResource,
		// Excluded due to nested attributes:
		// newBucketNotificationResource,
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
		// Notify resources
		resourceMinioNotifyAmqpFramework,
		resourceMinioNotifyElasticsearchFramework,
		resourceMinioNotifyKafkaFramework,
		resourceMinioNotifyMqttFramework,
		resourceMinioNotifyMysqlFramework,
		resourceMinioNotifyNatsFramework,
		resourceMinioNotifyNsqFramework,
		resourceMinioNotifyPostgresFramework,
		resourceMinioNotifyRedisFramework,
		resourceMinioNotifyWebhookFramework,
		// Audit resources
		resourceMinioAuditKafkaFramework,
		resourceMinioAuditWebhookFramework,
		// Logger resources
		resourceMinioLoggerWebhookFramework,
		// Excluded due to nested attributes:
		// newSiteReplicationResource,
		// newBucketReplicationResource(),
	}
}

// DataSources returns the list of data sources
// Note: All data sources are provided by the SDK provider to avoid conflicts
func (p *minioFrameworkProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}
