package minio

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func frameworkString(v types.String, envKeys []string, fallback string) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	for _, key := range envKeys {
		if env := os.Getenv(key); env != "" {
			return env
		}
	}
	return fallback
}

func frameworkBool(v types.Bool, envKeys []string, fallback bool) bool {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueBool()
	}
	for _, key := range envKeys {
		if env := os.Getenv(key); env != "" {
			parsed, err := strconv.ParseBool(env)
			if err == nil {
				return parsed
			}
		}
	}
	return fallback
}

func frameworkInt(v types.Int64, envKeys []string, fallback int, diags *diag.Diagnostics) int {
	if !v.IsNull() && !v.IsUnknown() {
		return int(v.ValueInt64())
	}
	for _, key := range envKeys {
		env := os.Getenv(key)
		if env == "" {
			continue
		}
		parsed, err := strconv.Atoi(env)
		if err != nil {
			diags.AddError(
				"Invalid "+key,
				fmt.Sprintf("%s must be a whole number, got %q.", key, env),
			)
			return fallback
		}
		return parsed
	}
	return fallback
}

func frameworkConfig(ctx context.Context, model frameworkProviderModel, diags *diag.Diagnostics) *S3MinioConfig {
	user := frameworkString(model.MinioUser, []string{"MINIO_USER"}, "")
	if user == "" {
		user = frameworkString(model.MinioAccessKey, []string{"MINIO_ACCESS_KEY"}, "")
	}

	password := frameworkString(model.MinioPassword, []string{"MINIO_PASSWORD"}, "")
	if password == "" {
		password = frameworkString(model.MinioSecretKey, []string{"MINIO_SECRET_KEY"}, "")
	}

	config := &S3MinioConfig{
		S3HostPort:            frameworkString(model.MinioServer, []string{"MINIO_ENDPOINT"}, ""),
		S3Region:              frameworkString(model.MinioRegion, nil, "us-east-1"),
		S3UserAccess:          user,
		S3UserSecret:          password,
		S3SessionToken:        frameworkString(model.MinioSessionToken, []string{"MINIO_SESSION_TOKEN"}, ""),
		S3APISignature:        frameworkString(model.MinioAPIVersion, nil, "v4"),
		S3SSL:                 frameworkBool(model.MinioSSL, []string{"MINIO_ENABLE_HTTPS"}, false),
		S3SSLCACertFile:       frameworkString(model.MinioCACertFile, []string{"MINIO_CACERT_FILE"}, ""),
		S3SSLCertFile:         frameworkString(model.MinioCertFile, []string{"MINIO_CERT_FILE"}, ""),
		S3SSLKeyFile:          frameworkString(model.MinioKeyFile, []string{"MINIO_KEY_FILE"}, ""),
		S3SSLSkipVerify:       frameworkBool(model.MinioInsecure, []string{"MINIO_INSECURE"}, false),
		SkipBucketTagging:     frameworkBool(model.SkipBucketTagging, []string{"MINIO_SKIP_BUCKET_TAGGING"}, false),
		S3CompatMode:          frameworkBool(model.S3CompatMode, []string{"MINIO_S3_COMPAT_MODE"}, false),
		Edition:               frameworkString(model.MinioEdition, []string{"MINIO_EDITION"}, ""),
		RequestTimeoutSeconds: frameworkInt(model.RequestTimeoutSeconds, []string{"MINIO_REQUEST_TIMEOUT_SECONDS"}, defaultRequestTimeoutSeconds, diags),
		MaxRetries:            frameworkInt(model.MaxRetries, []string{"MINIO_MAX_RETRIES"}, defaultMaxRetries, diags),
		RetryDelayMs:          frameworkInt(model.RetryDelayMs, []string{"MINIO_RETRY_DELAY_MS"}, defaultRetryDelayMs, diags),
	}

	if !model.AssumeRole.IsNull() && !model.AssumeRole.IsUnknown() {
		var blocks []frameworkAssumeRoleModel
		diags.Append(model.AssumeRole.ElementsAs(ctx, &blocks, false)...)
		if diags.HasError() {
			return nil
		}
		if len(blocks) > 0 {
			block := blocks[0]
			config.AssumeRoleARN = frameworkString(block.RoleARN, []string{"MINIO_ASSUME_ROLE_ARN"}, "")
			config.AssumeRoleSessionName = frameworkString(block.SessionName, nil, "terraform")
			config.AssumeRoleDuration = frameworkInt(block.DurationSeconds, nil, 3600, diags)
			config.AssumeRolePolicy = frameworkString(block.Policy, nil, "")
			config.AssumeRoleExternalID = frameworkString(block.ExternalID, nil, "")
		}
	}

	if !model.AssumeRoleWebIdentity.IsNull() && !model.AssumeRoleWebIdentity.IsUnknown() {
		var blocks []frameworkWebIdentityModel
		diags.Append(model.AssumeRoleWebIdentity.ElementsAs(ctx, &blocks, false)...)
		if diags.HasError() {
			return nil
		}
		if len(blocks) > 0 {
			block := blocks[0]
			config.WebIdentityToken = frameworkString(block.WebIdentityToken, []string{"MINIO_WEB_IDENTITY_TOKEN"}, "")
			config.WebIdentityTokenFile = frameworkString(block.WebIdentityTokenFile, []string{"MINIO_WEB_IDENTITY_TOKEN_FILE"}, "")
			config.WebIdentityDuration = frameworkInt(block.DurationSeconds, nil, 3600, diags)
		}
	}

	return config
}
