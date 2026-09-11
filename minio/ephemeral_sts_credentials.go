package minio

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func NewSTSCredentialsEphemeralResource() ephemeral.EphemeralResource {
	return &stsCredentialsEphemeralResource{}
}

func (r *stsCredentialsEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sts_credentials"
}

func (r *stsCredentialsEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Obtains temporary MinIO credentials through STS AssumeRole. The credentials are never written to Terraform state: use them to configure another provider or to feed a write-only attribute.",
		Attributes: map[string]schema.Attribute{
			"role_arn": schema.StringAttribute{
				Optional:    true,
				Description: "ARN of the role to assume. MinIO accepts an empty value and derives the policy from the requesting user.",
			},
			"session_name": schema.StringAttribute{
				Optional:    true,
				Description: "Session name for the assumed role. Defaults to `terraform`.",
			},
			"duration_seconds": schema.Int64Attribute{
				Optional:    true,
				Description: "Lifetime of the returned credentials in seconds. Defaults to 3600, which is also the minimum: the MinIO client library replaces any lower value with 3600, so shorter lifetimes are rejected here rather than silently ignored.",
				Validators: []validator.Int64{
					int64validator.AtLeast(3600),
				},
			},
			"policy": schema.StringAttribute{
				Optional:    true,
				Description: "IAM policy in JSON format that scopes down the permissions of the returned credentials.",
			},
			"external_id": schema.StringAttribute{
				Optional:    true,
				Description: "External ID for cross-account role assumption.",
			},
			"access_key": schema.StringAttribute{
				Computed:    true,
				Description: "Access key of the temporary credentials.",
			},
			"secret_key": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Secret key of the temporary credentials.",
			},
			"session_token": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Session token that must accompany the temporary credentials.",
			},
			"expiration": schema.StringAttribute{
				Computed:    true,
				Description: "Time at which the credentials stop being valid, in RFC 3339 format. Null when the server does not report an expiry.",
			},
		},
	}
}

func (r *stsCredentialsEphemeralResource) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	config, ok := req.ProviderData.(*S3MinioConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("Expected *S3MinioConfig, got %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	r.config = config
}

func (r *stsCredentialsEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var model stsCredentialsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.config == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The MinIO provider configuration is not available. This is a bug in the provider.",
		)
		return
	}

	if r.config.AssumeRoleARN != "" || r.config.AssumeRoleSessionName != "" || r.config.WebIdentityToken != "" || r.config.WebIdentityTokenFile != "" {
		resp.Diagnostics.AddError(
			"Provider uses assumed credentials",
			"minio_sts_credentials signs its AssumeRole request with the static credentials in the provider configuration, so it would issue credentials outside the scope the provider's assume_role or assume_role_with_web_identity block establishes. MinIO also rejects AssumeRole requests made with temporary credentials, so chaining is not possible. Use a provider configuration with static credentials for this ephemeral resource.",
		)
		return
	}

	if r.config.S3SessionToken != "" {
		resp.Diagnostics.AddError(
			"Provider uses temporary credentials",
			"minio_sts_credentials cannot derive credentials from a session token: MinIO rejects AssumeRole requests made with temporary credentials. Use a provider configuration with static credentials for this ephemeral resource.",
		)
		return
	}

	sessionName := "terraform"
	if !model.SessionName.IsNull() && model.SessionName.ValueString() != "" {
		sessionName = model.SessionName.ValueString()
	}

	duration := 3600
	if !model.DurationSeconds.IsNull() {
		duration = int(model.DurationSeconds.ValueInt64())
	}

	tflog.Debug(ctx, "Requesting STS credentials", map[string]interface{}{
		"role":     model.RoleARN.ValueString(),
		"session":  sessionName,
		"duration": duration,
	})

	value, err := requestSTSCredentials(ctx, r.config, stsRequest{
		RoleARN:         model.RoleARN.ValueString(),
		SessionName:     sessionName,
		DurationSeconds: duration,
		Policy:          model.Policy.ValueString(),
		ExternalID:      model.ExternalID.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to request STS credentials", err.Error())
		return
	}

	model.AccessKey = types.StringValue(value.AccessKeyID)
	model.SecretKey = types.StringValue(value.SecretAccessKey)
	model.SessionToken = types.StringValue(value.SessionToken)

	if value.Expiration.IsZero() {
		model.Expiration = types.StringNull()
	} else {
		model.Expiration = types.StringValue(value.Expiration.UTC().Format(time.RFC3339))
	}

	resp.Diagnostics.Append(resp.Result.Set(ctx, &model)...)
}

func requestSTSCredentials(ctx context.Context, config *S3MinioConfig, req stsRequest) (credentials.Value, error) {
	scheme := "http"
	if config.S3SSL {
		scheme = "https"
	}

	transport, err := config.customTransport(ctx)
	if err != nil {
		return credentials.Value{}, fmt.Errorf("failed to configure transport: %w", err)
	}

	stsCreds, err := credentials.NewSTSAssumeRole(fmt.Sprintf("%s://%s", scheme, config.S3HostPort), credentials.STSAssumeRoleOptions{
		AccessKey:       config.S3UserAccess,
		SecretKey:       config.S3UserSecret,
		RoleARN:         req.RoleARN,
		RoleSessionName: req.SessionName,
		DurationSeconds: req.DurationSeconds,
		Policy:          req.Policy,
		ExternalID:      req.ExternalID,
		Location:        config.S3Region,
	})
	if err != nil {
		return credentials.Value{}, err
	}

	return stsCreds.GetWithContext(&credentials.CredContext{
		Client:  &http.Client{Transport: transport},
		Context: ctx,
	})
}
