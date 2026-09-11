package minio

import (
	"context"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func nullFrameworkModel() frameworkProviderModel {
	return frameworkProviderModel{
		MinioServer:           types.StringNull(),
		MinioRegion:           types.StringNull(),
		MinioUser:             types.StringNull(),
		MinioPassword:         types.StringNull(),
		MinioAccessKey:        types.StringNull(),
		MinioSecretKey:        types.StringNull(),
		MinioSessionToken:     types.StringNull(),
		MinioAPIVersion:       types.StringNull(),
		MinioSSL:              types.BoolNull(),
		MinioInsecure:         types.BoolNull(),
		MinioCACertFile:       types.StringNull(),
		MinioCertFile:         types.StringNull(),
		MinioKeyFile:          types.StringNull(),
		MinioDebug:            types.BoolNull(),
		SkipBucketTagging:     types.BoolNull(),
		S3CompatMode:          types.BoolNull(),
		MinioEdition:          types.StringNull(),
		RequestTimeoutSeconds: types.Int64Null(),
		MaxRetries:            types.Int64Null(),
		RetryDelayMs:          types.Int64Null(),
		AssumeRole:            types.ListNull(types.ObjectType{}),
		AssumeRoleWebIdentity: types.ListNull(types.ObjectType{}),
	}
}

func TestFrameworkConfigDefaults(t *testing.T) {
	for _, key := range []string{
		"MINIO_ENDPOINT", "MINIO_USER", "MINIO_PASSWORD", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY",
		"MINIO_SESSION_TOKEN", "MINIO_ENABLE_HTTPS", "MINIO_INSECURE", "MINIO_CACERT_FILE",
		"MINIO_CERT_FILE", "MINIO_KEY_FILE", "MINIO_SKIP_BUCKET_TAGGING", "MINIO_S3_COMPAT_MODE",
		"MINIO_EDITION", "MINIO_REQUEST_TIMEOUT_SECONDS", "MINIO_MAX_RETRIES", "MINIO_RETRY_DELAY_MS",
	} {
		t.Setenv(key, "")
	}

	var diags diag.Diagnostics
	config := frameworkConfig(context.Background(), nullFrameworkModel(), &diags)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if config.S3Region != "us-east-1" {
		t.Errorf("S3Region = %q, want %q", config.S3Region, "us-east-1")
	}
	if config.S3APISignature != "v4" {
		t.Errorf("S3APISignature = %q, want %q", config.S3APISignature, "v4")
	}
	if config.RequestTimeoutSeconds != 30 {
		t.Errorf("RequestTimeoutSeconds = %d, want 30", config.RequestTimeoutSeconds)
	}
	if config.MaxRetries != 6 {
		t.Errorf("MaxRetries = %d, want 6", config.MaxRetries)
	}
	if config.RetryDelayMs != 1000 {
		t.Errorf("RetryDelayMs = %d, want 1000", config.RetryDelayMs)
	}
	if config.S3SSL || config.S3SSLSkipVerify || config.SkipBucketTagging || config.S3CompatMode {
		t.Errorf("boolean defaults should all be false, got %+v", config)
	}
}

func TestFrameworkConfigReadsEnvironment(t *testing.T) {
	t.Setenv("MINIO_ENDPOINT", "env-host:9000")
	t.Setenv("MINIO_USER", "env-user")
	t.Setenv("MINIO_PASSWORD", "env-password")
	t.Setenv("MINIO_SESSION_TOKEN", "env-token")
	t.Setenv("MINIO_ENABLE_HTTPS", "true")
	t.Setenv("MINIO_INSECURE", "true")
	t.Setenv("MINIO_CACERT_FILE", "/env/ca.pem")
	t.Setenv("MINIO_CERT_FILE", "/env/cert.pem")
	t.Setenv("MINIO_KEY_FILE", "/env/key.pem")
	t.Setenv("MINIO_SKIP_BUCKET_TAGGING", "true")
	t.Setenv("MINIO_S3_COMPAT_MODE", "true")
	t.Setenv("MINIO_EDITION", "AIStor")
	var diags diag.Diagnostics
	config := frameworkConfig(context.Background(), nullFrameworkModel(), &diags)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"S3HostPort", config.S3HostPort, "env-host:9000"},
		{"S3UserAccess", config.S3UserAccess, "env-user"},
		{"S3UserSecret", config.S3UserSecret, "env-password"},
		{"S3SessionToken", config.S3SessionToken, "env-token"},
		{"S3SSL", config.S3SSL, true},
		{"S3SSLSkipVerify", config.S3SSLSkipVerify, true},
		{"S3SSLCACertFile", config.S3SSLCACertFile, "/env/ca.pem"},
		{"S3SSLCertFile", config.S3SSLCertFile, "/env/cert.pem"},
		{"S3SSLKeyFile", config.S3SSLKeyFile, "/env/key.pem"},
		{"SkipBucketTagging", config.SkipBucketTagging, true},
		{"S3CompatMode", config.S3CompatMode, true},
		{"Edition", config.Edition, "AIStor"},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s = %v, want %v", check.name, check.got, check.want)
		}
	}
}

func TestFrameworkConfigConfigurationWinsOverEnvironment(t *testing.T) {
	t.Setenv("MINIO_ENDPOINT", "env-host:9000")
	t.Setenv("MINIO_USER", "env-user")
	t.Setenv("MINIO_MAX_RETRIES", "9")

	model := nullFrameworkModel()
	model.MinioServer = types.StringValue("config-host:9000")
	model.MinioUser = types.StringValue("config-user")
	model.MaxRetries = types.Int64Value(2)

	var diags diag.Diagnostics
	config := frameworkConfig(context.Background(), model, &diags)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if config.S3HostPort != "config-host:9000" {
		t.Errorf("S3HostPort = %q, want the configured value", config.S3HostPort)
	}
	if config.S3UserAccess != "config-user" {
		t.Errorf("S3UserAccess = %q, want the configured value", config.S3UserAccess)
	}
	if config.MaxRetries != 2 {
		t.Errorf("MaxRetries = %d, want the configured value 2", config.MaxRetries)
	}
}

func TestFrameworkConfigFallsBackToLegacyCredentialFields(t *testing.T) {
	t.Setenv("MINIO_USER", "")
	t.Setenv("MINIO_PASSWORD", "")
	t.Setenv("MINIO_ACCESS_KEY", "legacy-access")
	t.Setenv("MINIO_SECRET_KEY", "legacy-secret")

	var diags diag.Diagnostics
	config := frameworkConfig(context.Background(), nullFrameworkModel(), &diags)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if config.S3UserAccess != "legacy-access" {
		t.Errorf("S3UserAccess = %q, want the legacy access key", config.S3UserAccess)
	}
	if config.S3UserSecret != "legacy-secret" {
		t.Errorf("S3UserSecret = %q, want the legacy secret key", config.S3UserSecret)
	}
}

func TestFrameworkConfigReadsAssumeRoleBlock(t *testing.T) {
	ctx := context.Background()

	objectType := assumeRoleObjectType()

	block, diags := types.ObjectValue(objectType.AttrTypes, map[string]attr.Value{
		"role_arn":         types.StringValue("arn:aws:iam:::role/reader"),
		"session_name":     types.StringValue("ci"),
		"duration_seconds": types.Int64Value(900),
		"policy":           types.StringValue(`{"Version":"2012-10-17"}`),
		"external_id":      types.StringValue("external"),
	})
	if diags.HasError() {
		t.Fatalf("building the assume_role object: %v", diags)
	}

	list, diags := types.ListValue(objectType, []attr.Value{block})
	if diags.HasError() {
		t.Fatalf("building the assume_role list: %v", diags)
	}

	model := nullFrameworkModel()
	model.AssumeRole = list

	var configDiags diag.Diagnostics
	config := frameworkConfig(ctx, model, &configDiags)
	if configDiags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", configDiags)
	}

	if config.AssumeRoleARN != "arn:aws:iam:::role/reader" {
		t.Errorf("AssumeRoleARN = %q", config.AssumeRoleARN)
	}
	if config.AssumeRoleSessionName != "ci" {
		t.Errorf("AssumeRoleSessionName = %q", config.AssumeRoleSessionName)
	}
	if config.AssumeRoleDuration != 900 {
		t.Errorf("AssumeRoleDuration = %d", config.AssumeRoleDuration)
	}
	if config.AssumeRoleExternalID != "external" {
		t.Errorf("AssumeRoleExternalID = %q", config.AssumeRoleExternalID)
	}
}

func TestFrameworkConfigAssumeRoleDefaults(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MINIO_ASSUME_ROLE_ARN", "")

	objectType := assumeRoleObjectType()

	block, diags := types.ObjectValue(objectType.AttrTypes, map[string]attr.Value{
		"role_arn":         types.StringValue("arn:aws:iam:::role/reader"),
		"session_name":     types.StringNull(),
		"duration_seconds": types.Int64Null(),
		"policy":           types.StringNull(),
		"external_id":      types.StringNull(),
	})
	if diags.HasError() {
		t.Fatalf("building the assume_role object: %v", diags)
	}

	list, diags := types.ListValue(objectType, []attr.Value{block})
	if diags.HasError() {
		t.Fatalf("building the assume_role list: %v", diags)
	}

	model := nullFrameworkModel()
	model.AssumeRole = list

	var configDiags diag.Diagnostics
	config := frameworkConfig(ctx, model, &configDiags)
	if configDiags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", configDiags)
	}

	if config.AssumeRoleSessionName != "terraform" {
		t.Errorf("AssumeRoleSessionName = %q, want the default %q", config.AssumeRoleSessionName, "terraform")
	}
	if config.AssumeRoleDuration != 3600 {
		t.Errorf("AssumeRoleDuration = %d, want the default 3600", config.AssumeRoleDuration)
	}
}

func TestFrameworkConfigResolvesRetryTuningLikeTheSDK(t *testing.T) {
	for _, tc := range retryTuningAttributes {
		t.Setenv(tc.envKey, strconv.Itoa(tc.envValue))
	}

	var diags diag.Diagnostics
	config := frameworkConfig(context.Background(), nullFrameworkModel(), &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	sdkConfig := NewConfig(schema.TestResourceDataRaw(t, Provider().Schema, map[string]interface{}{}))

	for _, tc := range retryTuningAttributes {
		got := retryTuning(config)[tc.attribute]
		if got != tc.envValue {
			t.Errorf("%s = %d, want %d from %s", tc.attribute, got, tc.envValue, tc.envKey)
		}
		if want := retryTuning(sdkConfig)[tc.attribute]; got != want {
			t.Errorf("%s = %d in the framework half and %d in the SDKv2 half: both must resolve this attribute the same way", tc.attribute, got, want)
		}
	}
}

func TestFrameworkConfigPrefersTheConfigurationOverTheRetryEnvironment(t *testing.T) {
	for _, tc := range retryTuningAttributes {
		t.Setenv(tc.envKey, strconv.Itoa(tc.envValue))
	}

	model := nullFrameworkModel()
	model.RequestTimeoutSeconds = types.Int64Value(12)
	model.MaxRetries = types.Int64Value(2)
	model.RetryDelayMs = types.Int64Value(75)

	var diags diag.Diagnostics
	config := frameworkConfig(context.Background(), model, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	for _, tc := range retryTuningAttributes {
		if got := retryTuning(config)[tc.attribute]; got != tc.configValue {
			t.Errorf("%s = %d, want the configured value %d", tc.attribute, got, tc.configValue)
		}
	}
}

func TestFrameworkProviderConfigurePassesConfigToEphemeralResources(t *testing.T) {
	t.Setenv("MINIO_ENDPOINT", "")

	p := NewFrameworkProvider()

	var schemaResp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building the provider schema: %v", schemaResp.Diagnostics)
	}

	model := nullFrameworkModel()
	model.MinioServer = types.StringValue("localhost:9000")
	model.MinioUser = types.StringValue("minio")
	model.MinioPassword = types.StringValue("minio123")
	model.AssumeRole = types.ListNull(assumeRoleObjectType())
	model.AssumeRoleWebIdentity = types.ListNull(webIdentityObjectType())

	object, diags := types.ObjectValueFrom(context.Background(), schemaResp.Schema.Type().(types.ObjectType).AttrTypes, model)
	if diags.HasError() {
		t.Fatalf("converting the model: %v", diags)
	}
	raw, err := object.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("converting the model to a Terraform value: %s", err)
	}

	var resp provider.ConfigureResponse
	p.Configure(context.Background(), provider.ConfigureRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("configuring the framework provider: %v", resp.Diagnostics)
	}

	config, ok := resp.EphemeralResourceData.(*S3MinioConfig)
	if !ok {
		t.Fatalf("EphemeralResourceData is %T, want *S3MinioConfig", resp.EphemeralResourceData)
	}
	if config.S3HostPort != "localhost:9000" {
		t.Errorf("S3HostPort = %q, want the configured endpoint", config.S3HostPort)
	}
	if config.S3UserAccess != "minio" {
		t.Errorf("S3UserAccess = %q, want the configured user", config.S3UserAccess)
	}
	if resp.ResourceData == nil || resp.DataSourceData == nil {
		t.Error("the provider must hand the same configuration to every kind of consumer")
	}
}

func assumeRoleObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"role_arn":         types.StringType,
		"session_name":     types.StringType,
		"duration_seconds": types.Int64Type,
		"policy":           types.StringType,
		"external_id":      types.StringType,
	}}
}

func webIdentityObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"web_identity_token":      types.StringType,
		"web_identity_token_file": types.StringType,
		"duration_seconds":        types.Int64Type,
	}}
}
