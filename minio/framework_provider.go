package minio

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func NewFrameworkProvider() provider.Provider {
	return &frameworkProvider{}
}

func (p *frameworkProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "minio"
}

func (p *frameworkProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"minio_server": schema.StringAttribute{
				Required:    serverEndpointRequired(),
				Optional:    !serverEndpointRequired(),
				Description: "MinIO server endpoint in the format host:port",
			},
			"minio_region": schema.StringAttribute{
				Optional: true,
				Description: "Region used for request signing and sent to the S3 client. " +
					"Defaults to `us-east-1`. Set this to match the region configured on your " +
					"server, or to any non-empty string when using S3-compatible stores that " +
					"require a specific region (e.g. Versity Gateway, Hetzner Object Storage).",
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
			"minio_edition": schema.StringAttribute{
				Optional:    true,
				Description: "Override the auto-detected MinIO edition (e.g. `AIStor` to force AIStor-shaped behaviors). Leave empty to auto-detect from `ServerInfo`. Set this only when the provider misclassifies your server — for example an AIStor build whose `ServerInfo` response does not surface the `edition` field.",
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
		},
		Blocks: map[string]schema.Block{
			"assume_role": schema.ListNestedBlock{
				Description: "Use STS AssumeRole to obtain temporary credentials. When configured, the provider exchanges the static credentials for short-lived session credentials.",
				NestedObject: schema.NestedBlockObject{
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
			"assume_role_with_web_identity": schema.ListNestedBlock{
				Description: "Use STS AssumeRoleWithWebIdentity to obtain credentials from an OIDC token (e.g., GitHub Actions, GitLab CI).",
				NestedObject: schema.NestedBlockObject{
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

func (p *frameworkProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var model frameworkProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := frameworkConfig(ctx, model, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.EphemeralResourceData = config
	resp.ResourceData = config
	resp.DataSourceData = config
}

func (p *frameworkProvider) Resources(_ context.Context) []func() resource.Resource {
	return nil
}

func (p *frameworkProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func (p *frameworkProvider) EphemeralResources(_ context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{
		NewSTSCredentialsEphemeralResource,
	}
}

func serverEndpointRequired() bool {
	return os.Getenv("MINIO_ENDPOINT") == ""
}
