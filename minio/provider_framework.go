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

// assumeRoleModel represents the assume_role block in provider config.
type assumeRoleModel struct {
	RoleARN         types.String `tfsdk:"role_arn"`
	SessionName     types.String `tfsdk:"session_name"`
	DurationSeconds types.Int64  `tfsdk:"duration_seconds"`
	Policy          types.String `tfsdk:"policy"`
	ExternalID      types.String `tfsdk:"external_id"`
}

// webIdentityModel represents the assume_role_with_web_identity block in provider config.
type webIdentityModel struct {
	WebIdentityToken     types.String `tfsdk:"web_identity_token"`
	WebIdentityTokenFile types.String `tfsdk:"web_identity_token_file"`
	DurationSeconds      types.Int64  `tfsdk:"duration_seconds"`
}

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
			"request_timeout_seconds": schema.Int64Attribute{
				Optional:    true,
				Description: "Global HTTP request timeout in seconds for all MinIO API calls (default: 30)",
			},
			"max_retries": schema.Int64Attribute{
				Optional:    true,
				Description: "Maximum number of retries for failed operations (default: 6)",
			},
			"retry_delay_ms": schema.Int64Attribute{
				Optional:    true,
				Description: "Base delay in milliseconds between retries, used with exponential backoff (default: 1000)",
			},
			"assume_role": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Use STS AssumeRole to obtain temporary credentials.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"role_arn": schema.StringAttribute{
							Optional:    true,
							Description: "ARN of the role to assume.",
						},
						"session_name": schema.StringAttribute{
							Optional:    true,
							Description: "Session name for the assumed role.",
						},
						"duration_seconds": schema.Int64Attribute{
							Optional:    true,
							Description: "Duration in seconds for the session (default: 3600).",
						},
						"policy": schema.StringAttribute{
							Optional:    true,
							Description: "IAM policy in JSON format to scope down the assumed role permissions.",
						},
						"external_id": schema.StringAttribute{
							Optional:    true,
							Description: "External ID for cross-account role assumption.",
						},
					},
				},
			},
			"assume_role_with_web_identity": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Use STS AssumeRoleWithWebIdentity to obtain credentials from an OIDC token.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"web_identity_token": schema.StringAttribute{
							Optional:    true,
							Sensitive:   true,
							Description: "OIDC/JWT token for web identity authentication.",
						},
						"web_identity_token_file": schema.StringAttribute{
							Optional:    true,
							Description: "Path to a file containing the OIDC/JWT token.",
						},
						"duration_seconds": schema.Int64Attribute{
							Optional:    true,
							Description: "Duration in seconds for the session (default: 3600).",
						},
					},
				},
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

	requestTimeoutSeconds := int(getInt64OrDefault(data.RequestTimeoutSeconds, 30))
	maxRetries := int(getInt64OrDefault(data.MaxRetries, 6))
	retryDelayMs := int(getInt64OrDefault(data.RetryDelayMs, 1000))

	// Build S3MinioConfig from framework data
	cfg := &S3MinioConfig{
		S3HostPort:            minioServer,
		S3Region:              minioRegion,
		S3UserAccess:          minioUser,
		S3UserSecret:          minioPassword,
		S3SessionToken:        minioSessionToken,
		S3APISignature:        minioAPIVersion,
		S3SSL:                 minioSSL,
		S3SSLCACertFile:       minioCACertFile,
		S3SSLCertFile:         minioCertFile,
		S3SSLKeyFile:          minioKeyFile,
		S3SSLSkipVerify:       minioInsecure,
		S3Debug:               minioDebug,
		SkipBucketTagging:     skipBucketTagging,
		S3CompatMode:          s3CompatMode,
		RequestTimeoutSeconds: requestTimeoutSeconds,
		MaxRetries:            maxRetries,
		RetryDelayMs:          retryDelayMs,
	}

	// Decode assume_role block
	if !data.AssumeRole.IsNull() && !data.AssumeRole.IsUnknown() {
		var roles []assumeRoleModel
		if diag := data.AssumeRole.ElementsAs(ctx, &roles, false); !diag.HasError() && len(roles) > 0 {
			ar := roles[0]
			cfg.AssumeRoleARN = getStringOrDefault(ar.RoleARN, "")
			cfg.AssumeRoleSessionName = getStringOrDefault(ar.SessionName, "terraform")
			cfg.AssumeRoleDuration = int(getInt64OrDefault(ar.DurationSeconds, 3600))
			cfg.AssumeRolePolicy = getStringOrDefault(ar.Policy, "")
			cfg.AssumeRoleExternalID = getStringOrDefault(ar.ExternalID, "")
		}
	}

	// Decode assume_role_with_web_identity block
	if !data.AssumeRoleWithWebIdentity.IsNull() && !data.AssumeRoleWithWebIdentity.IsUnknown() {
		var wis []webIdentityModel
		if diag := data.AssumeRoleWithWebIdentity.ElementsAs(ctx, &wis, false); !diag.HasError() && len(wis) > 0 {
			wi := wis[0]
			cfg.WebIdentityToken = getStringOrDefault(wi.WebIdentityToken, "")
			cfg.WebIdentityTokenFile = getStringOrDefault(wi.WebIdentityTokenFile, "")
			cfg.WebIdentityDuration = int(getInt64OrDefault(wi.DurationSeconds, 3600))
		}
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
	MinioServer                  types.String `tfsdk:"minio_server"`
	MinioRegion                  types.String `tfsdk:"minio_region"`
	MinioUser                    types.String `tfsdk:"minio_user"`
	MinioPassword                types.String `tfsdk:"minio_password"`
	MinioSessionToken            types.String `tfsdk:"minio_session_token"`
	MinioAPIVersion              types.String `tfsdk:"minio_api_version"`
	MinioSSL                     types.Bool   `tfsdk:"minio_ssl"`
	MinioInsecure                types.Bool   `tfsdk:"minio_insecure"`
	MinioCACertFile              types.String `tfsdk:"minio_cacert_file"`
	MinioCertFile                types.String `tfsdk:"minio_cert_file"`
	MinioKeyFile                 types.String `tfsdk:"minio_key_file"`
	MinioDebug                   types.Bool   `tfsdk:"minio_debug"`
	SkipBucketTagging            types.Bool   `tfsdk:"skip_bucket_tagging"`
	S3CompatMode                 types.Bool   `tfsdk:"s3_compat_mode"`
	RequestTimeoutSeconds        types.Int64  `tfsdk:"request_timeout_seconds"`
	MaxRetries                   types.Int64  `tfsdk:"max_retries"`
	RetryDelayMs                 types.Int64  `tfsdk:"retry_delay_ms"`
	AssumeRole                   types.List   `tfsdk:"assume_role"`
	AssumeRoleWithWebIdentity    types.List   `tfsdk:"assume_role_with_web_identity"`
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

func getInt64OrDefault(v types.Int64, defaultVal int64) int64 {
	if v.IsNull() || v.IsUnknown() {
		return defaultVal
	}
	return v.ValueInt64()
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
		newBucketQuotaResource,
		newBucketTagsResource,
		newBucketRetentionResource,
		newBucketCorsResource,
		newBucketNotificationResource,
		newBucketObjectLockConfigurationResource,
		newBucketEncryptionResource,
		newBucketVersioningResource,
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
		newBucketReplicationResource,
		newSiteReplicationResource,
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
	}
}

// DataSources returns the list of data sources
func (p *minioFrameworkProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newS3BucketDataSource,
	}
}
