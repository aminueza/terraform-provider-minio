package minio

import (
	"context"

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

// Schema returns the provider schema
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
			"minio_access_key": schema.StringAttribute{
				Optional:           true,
				Description:        "MinIO access key (deprecated: use minio_user instead)",
				DeprecationMessage: "use minio_user instead",
			},
			"minio_secret_key": schema.StringAttribute{
				Optional:           true,
				Sensitive:          true,
				Description:        "MinIO secret key (deprecated: use minio_password instead)",
				DeprecationMessage: "use minio_password instead",
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
			"assume_role": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Use STS AssumeRole to obtain temporary credentials. When configured, the provider exchanges the static credentials for short-lived session credentials.",
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
			"assume_role_with_web_identity": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Use STS AssumeRoleWithWebIdentity to obtain credentials from an OIDC token (e.g., GitHub Actions, GitLab CI).",
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
	}
}

// Configure sets up the provider
func (p *minioFrameworkProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerModel

	// Read provider configuration
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build S3MinioConfig from framework data
	cfg := &S3MinioConfig{
		S3HostPort:        getStringOrDefault(data.MinioServer, ""),
		S3Region:          getStringOrDefault(data.MinioRegion, "us-east-1"),
		S3UserAccess:      getStringOrDefault(data.MinioUser, ""),
		S3UserSecret:      getStringOrDefault(data.MinioPassword, ""),
		S3SessionToken:    getStringOrDefault(data.MinioSessionToken, ""),
		S3APISignature:    getStringOrDefault(data.MinioAPIVersion, "v4"),
		S3SSL:             getBoolOrDefault(data.MinioSSL, false),
		S3SSLCACertFile:   getStringOrDefault(data.MinioCACertFile, ""),
		S3SSLCertFile:     getStringOrDefault(data.MinioCertFile, ""),
		S3SSLKeyFile:      getStringOrDefault(data.MinioKeyFile, ""),
		S3SSLSkipVerify:   getBoolOrDefault(data.MinioInsecure, false),
		SkipBucketTagging: getBoolOrDefault(data.SkipBucketTagging, false),
		S3CompatMode:      getBoolOrDefault(data.S3CompatMode, false),
	}

	// Handle legacy credentials
	if cfg.S3UserAccess == "" {
		cfg.S3UserAccess = getStringOrDefault(data.MinioAccessKey, "")
	}
	if cfg.S3UserSecret == "" {
		cfg.S3UserSecret = getStringOrDefault(data.MinioSecretKey, "")
	}

	// Handle assume_role
	if data.AssumeRole != nil {
		cfg.AssumeRoleARN = getStringOrDefault(data.AssumeRole.RoleARN, "")
		cfg.AssumeRoleSessionName = getStringOrDefault(data.AssumeRole.SessionName, "")
		cfg.AssumeRoleDuration = int(getInt64OrDefault(data.AssumeRole.DurationSeconds, 3600))
		cfg.AssumeRolePolicy = getStringOrDefault(data.AssumeRole.Policy, "")
		cfg.AssumeRoleExternalID = getStringOrDefault(data.AssumeRole.ExternalID, "")
	}

	// Handle assume_role_with_web_identity
	if data.AssumeRoleWithWebIdentity != nil {
		cfg.WebIdentityToken = getStringOrDefault(data.AssumeRoleWithWebIdentity.WebIdentityToken, "")
		cfg.WebIdentityTokenFile = getStringOrDefault(data.AssumeRoleWithWebIdentity.WebIdentityTokenFile, "")
		cfg.WebIdentityDuration = int(getInt64OrDefault(data.AssumeRoleWithWebIdentity.DurationSeconds, 3600))
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

// Resources returns the list of resources
func (p *minioFrameworkProvider) Resources(ctx context.Context) []func() resource.Resource {
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
		newBucketPolicyResource,
		newBucketVersioningResource,
		newBucketEncryptionResource,
		newBucketObjectLockConfigurationResource,
		newBucketQuotaResource,
		newBucketTagsResource,
		newBucketCorsResource,
		newBucketRetentionResource,
		newBucketNotificationResource,
	}
}

// DataSources returns the list of data sources
func (p *minioFrameworkProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newS3BucketDataSource,
	}
}

// providerModel represents the provider configuration data
type providerModel struct {
	MinioServer               types.String                `tfsdk:"minio_server"`
	MinioRegion               types.String                `tfsdk:"minio_region"`
	MinioUser                 types.String                `tfsdk:"minio_user"`
	MinioPassword             types.String                `tfsdk:"minio_password"`
	MinioAccessKey            types.String                `tfsdk:"minio_access_key"`
	MinioSecretKey            types.String                `tfsdk:"minio_secret_key"`
	MinioSessionToken         types.String                `tfsdk:"minio_session_token"`
	MinioAPIVersion           types.String                `tfsdk:"minio_api_version"`
	MinioSSL                  types.Bool                  `tfsdk:"minio_ssl"`
	MinioInsecure             types.Bool                  `tfsdk:"minio_insecure"`
	MinioCACertFile           types.String                `tfsdk:"minio_cacert_file"`
	MinioCertFile             types.String                `tfsdk:"minio_cert_file"`
	MinioKeyFile              types.String                `tfsdk:"minio_key_file"`
	MinioDebug                types.Bool                  `tfsdk:"minio_debug"`
	SkipBucketTagging         types.Bool                  `tfsdk:"skip_bucket_tagging"`
	S3CompatMode              types.Bool                  `tfsdk:"s3_compat_mode"`
	AssumeRole                *assumeRoleModel            `tfsdk:"assume_role"`
	AssumeRoleWithWebIdentity *assumeRoleWebIdentityModel `tfsdk:"assume_role_with_web_identity"`
}

type assumeRoleModel struct {
	RoleARN         types.String `tfsdk:"role_arn"`
	SessionName     types.String `tfsdk:"session_name"`
	DurationSeconds types.Int64  `tfsdk:"duration_seconds"`
	Policy          types.String `tfsdk:"policy"`
	ExternalID      types.String `tfsdk:"external_id"`
}

type assumeRoleWebIdentityModel struct {
	WebIdentityToken     types.String `tfsdk:"web_identity_token"`
	WebIdentityTokenFile types.String `tfsdk:"web_identity_token_file"`
	DurationSeconds      types.Int64  `tfsdk:"duration_seconds"`
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
