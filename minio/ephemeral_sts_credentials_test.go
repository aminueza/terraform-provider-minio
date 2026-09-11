package minio

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	ephemeralschema "github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const stsResponseTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>%s</AccessKeyId>
      <SecretAccessKey>%s</SecretAccessKey>
      <SessionToken>%s</SessionToken>
      <Expiration>%s</Expiration>
    </Credentials>
  </AssumeRoleResult>
</AssumeRoleResponse>`

func TestRequestSTSCredentialsReturnsServerValues(t *testing.T) {
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parsing the STS request body: %s", err)
		}
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, stsResponseTemplate, "TEMPACCESS", "TEMPSECRET", "TEMPTOKEN", expiry.Format(time.RFC3339))
	}))
	defer srv.Close()

	config := &S3MinioConfig{
		S3HostPort:   strings.TrimPrefix(srv.URL, "http://"),
		S3UserAccess: "accesskey",
		S3UserSecret: "secretkey",
		S3Region:     "us-east-1",
	}

	value, err := requestSTSCredentials(context.Background(), config, stsRequest{
		RoleARN:         "arn:aws:iam:::role/reader",
		SessionName:     "tfacc",
		DurationSeconds: 7200,
		Policy:          `{"Version":"2012-10-17"}`,
		ExternalID:      "external",
	})
	if err != nil {
		t.Fatalf("requesting STS credentials: %s", err)
	}

	if value.AccessKeyID != "TEMPACCESS" {
		t.Errorf("AccessKeyID = %q, want %q", value.AccessKeyID, "TEMPACCESS")
	}
	if value.SecretAccessKey != "TEMPSECRET" {
		t.Errorf("SecretAccessKey = %q, want %q", value.SecretAccessKey, "TEMPSECRET")
	}
	if value.SessionToken != "TEMPTOKEN" {
		t.Errorf("SessionToken = %q, want %q", value.SessionToken, "TEMPTOKEN")
	}
	if !value.Expiration.Equal(expiry) {
		t.Errorf("Expiration = %s, want %s", value.Expiration, expiry)
	}

	if got := gotForm.Get("Action"); got != "AssumeRole" {
		t.Errorf("Action = %q, want %q", got, "AssumeRole")
	}
	if got := gotForm.Get("RoleSessionName"); got != "tfacc" {
		t.Errorf("RoleSessionName = %q, want %q", got, "tfacc")
	}
	if got := gotForm.Get("DurationSeconds"); got != "7200" {
		t.Errorf("DurationSeconds = %q, want %q", got, "7200")
	}
	if got := gotForm.Get("Policy"); got != `{"Version":"2012-10-17"}` {
		t.Errorf("Policy = %q, want the scoped-down policy", got)
	}
}

func TestRequestSTSCredentialsPropagatesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `<ErrorResponse><Error><Code>AccessDenied</Code><Message>not allowed</Message></Error></ErrorResponse>`)
	}))
	defer srv.Close()

	config := &S3MinioConfig{
		S3HostPort:   strings.TrimPrefix(srv.URL, "http://"),
		S3UserAccess: "accesskey",
		S3UserSecret: "secretkey",
		S3Region:     "us-east-1",
	}

	if _, err := requestSTSCredentials(context.Background(), config, stsRequest{SessionName: "tfacc", DurationSeconds: 3600}); err == nil {
		t.Fatal("expected an error when the server rejects the request")
	}
}

func TestSTSCredentialsEphemeralResourceMetadata(t *testing.T) {
	resource := NewSTSCredentialsEphemeralResource()

	var resp ephemeral.MetadataResponse
	resource.Metadata(context.Background(), ephemeral.MetadataRequest{ProviderTypeName: "minio"}, &resp)

	if resp.TypeName != "minio_sts_credentials" {
		t.Errorf("TypeName = %q, want %q", resp.TypeName, "minio_sts_credentials")
	}
}

func TestSTSCredentialsEphemeralResourceSchemaKeepsSecretsSensitive(t *testing.T) {
	resource := NewSTSCredentialsEphemeralResource()

	var resp ephemeral.SchemaResponse
	resource.Schema(context.Background(), ephemeral.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("building the schema: %v", resp.Diagnostics)
	}

	for _, name := range []string{"secret_key", "session_token"} {
		attribute, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("attribute %q is missing from the schema", name)
			continue
		}
		if !attribute.IsSensitive() {
			t.Errorf("attribute %q must be marked sensitive", name)
		}
	}
}

func TestSTSCredentialsEphemeralResourceConfigureRejectsWrongProviderData(t *testing.T) {
	resource := NewSTSCredentialsEphemeralResource().(*stsCredentialsEphemeralResource)

	var resp ephemeral.ConfigureResponse
	resource.Configure(context.Background(), ephemeral.ConfigureRequest{ProviderData: "not a config"}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for unexpected provider data")
	}
	if resource.config != nil {
		t.Error("the resource kept a configuration after rejecting the provider data")
	}
}

func TestSTSCredentialsEphemeralResourceConfigureAcceptsClient(t *testing.T) {
	resource := NewSTSCredentialsEphemeralResource().(*stsCredentialsEphemeralResource)
	config := &S3MinioConfig{S3HostPort: "localhost:9000"}

	var resp ephemeral.ConfigureResponse
	resource.Configure(context.Background(), ephemeral.ConfigureRequest{ProviderData: config}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if resource.config != config {
		t.Error("the resource did not keep the configured provider configuration")
	}
}

func TestRequestSTSCredentialsCannotShortenBelowOneHour(t *testing.T) {
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		sent = r.PostForm.Get("DurationSeconds")
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, stsResponseTemplate, "A", "B", "C", time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	}))
	defer srv.Close()

	config := &S3MinioConfig{
		S3HostPort:   strings.TrimPrefix(srv.URL, "http://"),
		S3UserAccess: "accesskey",
		S3UserSecret: "secretkey",
		S3Region:     "us-east-1",
	}

	if _, err := requestSTSCredentials(context.Background(), config, stsRequest{SessionName: "tfacc", DurationSeconds: 900}); err != nil {
		t.Fatalf("requesting STS credentials: %s", err)
	}

	if sent != "3600" {
		t.Fatalf("DurationSeconds = %q; minio-go is expected to replace values below 3600 with 3600, which is why the schema validator rejects them", sent)
	}
}

func openSTSEphemeral(t *testing.T, config *S3MinioConfig, model stsCredentialsModel) ephemeral.OpenResponse {
	t.Helper()

	res := NewSTSCredentialsEphemeralResource().(*stsCredentialsEphemeralResource)

	var configureResp ephemeral.ConfigureResponse
	res.Configure(context.Background(), ephemeral.ConfigureRequest{ProviderData: config}, &configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatalf("configuring the ephemeral resource: %v", configureResp.Diagnostics)
	}

	var schemaResp ephemeral.SchemaResponse
	res.Schema(context.Background(), ephemeral.SchemaRequest{}, &schemaResp)

	raw := mustObjectValue(t, schemaResp.Schema, model)

	openResp := ephemeral.OpenResponse{
		Result: tfsdk.EphemeralResultData{Schema: schemaResp.Schema, Raw: raw},
	}
	res.Open(context.Background(), ephemeral.OpenRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
	}, &openResp)

	return openResp
}

func mustObjectValue(t *testing.T, s ephemeralschema.Schema, model stsCredentialsModel) tftypes.Value {
	t.Helper()

	object, diags := types.ObjectValueFrom(context.Background(), s.Type().(types.ObjectType).AttrTypes, model)
	if diags.HasError() {
		t.Fatalf("converting the model: %v", diags)
	}

	raw, err := object.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("converting the model to a Terraform value: %s", err)
	}
	return raw
}

func nullSTSModel() stsCredentialsModel {
	return stsCredentialsModel{
		RoleARN:         types.StringNull(),
		SessionName:     types.StringNull(),
		DurationSeconds: types.Int64Null(),
		Policy:          types.StringNull(),
		ExternalID:      types.StringNull(),
		AccessKey:       types.StringNull(),
		SecretKey:       types.StringNull(),
		SessionToken:    types.StringNull(),
		Expiration:      types.StringNull(),
	}
}

func TestSTSCredentialsOpenReturnsCredentials(t *testing.T) {
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, stsResponseTemplate, "OPENACCESS", "OPENSECRET", "OPENTOKEN", expiry.Format(time.RFC3339))
	}))
	defer srv.Close()

	resp := openSTSEphemeral(t, &S3MinioConfig{
		S3HostPort:   strings.TrimPrefix(srv.URL, "http://"),
		S3UserAccess: "accesskey",
		S3UserSecret: "secretkey",
		S3Region:     "us-east-1",
	}, nullSTSModel())

	if resp.Diagnostics.HasError() {
		t.Fatalf("opening the ephemeral resource: %v", resp.Diagnostics)
	}

	var result stsCredentialsModel
	if diags := resp.Result.Get(context.Background(), &result); diags.HasError() {
		t.Fatalf("reading the result: %v", diags)
	}

	if result.AccessKey.ValueString() != "OPENACCESS" {
		t.Errorf("access_key = %q, want %q", result.AccessKey.ValueString(), "OPENACCESS")
	}
	if result.SecretKey.ValueString() != "OPENSECRET" {
		t.Errorf("secret_key = %q, want %q", result.SecretKey.ValueString(), "OPENSECRET")
	}
	if result.SessionToken.ValueString() != "OPENTOKEN" {
		t.Errorf("session_token = %q, want %q", result.SessionToken.ValueString(), "OPENTOKEN")
	}
	if result.Expiration.ValueString() != expiry.Format(time.RFC3339) {
		t.Errorf("expiration = %q, want %q", result.Expiration.ValueString(), expiry.Format(time.RFC3339))
	}
}

func TestSTSCredentialsOpenRefusesAssumedProviderCredentials(t *testing.T) {
	cases := []struct {
		name   string
		config *S3MinioConfig
	}{
		{"assume_role", &S3MinioConfig{S3HostPort: "localhost:9000", AssumeRoleARN: "arn:aws:iam:::role/reader"}},
		{"assume_role session name only", &S3MinioConfig{S3HostPort: "localhost:9000", AssumeRoleSessionName: "terraform"}},
		{"web identity token", &S3MinioConfig{S3HostPort: "localhost:9000", WebIdentityToken: "jwt"}},
		{"web identity token file", &S3MinioConfig{S3HostPort: "localhost:9000", WebIdentityTokenFile: "/token"}},
		{"session token", &S3MinioConfig{S3HostPort: "localhost:9000", S3SessionToken: "temporary"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := openSTSEphemeral(t, tc.config, nullSTSModel())

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected the ephemeral resource to refuse a provider that does not use static credentials")
			}
		})
	}
}

func newSTSTLSServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, stsResponseTemplate, "TEMPACCESS", "TEMPSECRET", "TEMPTOKEN", time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	}))
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatalf("writing the CA certificate: %s", err)
	}

	return srv, path
}

func TestRequestSTSCredentialsUsesTheProviderTLSSettings(t *testing.T) {
	srv, caFile := newSTSTLSServer(t)
	host := strings.TrimPrefix(srv.URL, "https://")

	for _, tc := range []struct {
		name    string
		config  *S3MinioConfig
		wantErr string
	}{
		{
			name:   "minio_insecure skips verification",
			config: &S3MinioConfig{S3SSL: true, S3SSLSkipVerify: true},
		},
		{
			name:   "minio_cacert_file trusts the private authority",
			config: &S3MinioConfig{S3SSL: true, S3SSLCACertFile: caFile},
		},
		{
			name:    "neither option leaves the certificate untrusted",
			config:  &S3MinioConfig{S3SSL: true},
			wantErr: "certificate",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.config.S3HostPort = host
			tc.config.S3UserAccess = "accesskey"
			tc.config.S3UserSecret = "secretkey"
			tc.config.S3Region = "us-east-1"

			value, err := requestSTSCredentials(context.Background(), tc.config, stsRequest{SessionName: "tfacc", DurationSeconds: 3600})

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("the request succeeded against an untrusted certificate, so the TLS settings are not reaching the STS call")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want it to mention %q", err, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("requesting STS credentials: %s", err)
			}
			if value.AccessKeyID != "TEMPACCESS" {
				t.Errorf("AccessKeyID = %q, want %q", value.AccessKeyID, "TEMPACCESS")
			}
		})
	}
}

func TestRequestSTSCredentialsHonoursContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Second)
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, stsResponseTemplate, "A", "B", "C", time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	}))
	defer srv.Close()

	config := &S3MinioConfig{
		S3HostPort:   strings.TrimPrefix(srv.URL, "http://"),
		S3UserAccess: "accesskey",
		S3UserSecret: "secretkey",
		S3Region:     "us-east-1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	started := time.Now()
	if _, err := requestSTSCredentials(ctx, config, stsRequest{SessionName: "tfacc", DurationSeconds: 3600}); err == nil {
		t.Fatal("expected the expired context to stop the STS request")
	}

	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Errorf("the request took %s: the caller context must reach the STS call, not only the transport timeout", elapsed)
	}
}

func TestRequestSTSCredentialsReportsAnUnusableTransport(t *testing.T) {
	config := &S3MinioConfig{
		S3HostPort:      "localhost:9000",
		S3UserAccess:    "accesskey",
		S3UserSecret:    "secretkey",
		S3Region:        "us-east-1",
		S3SSL:           true,
		S3SSLCACertFile: filepath.Join(t.TempDir(), "missing.pem"),
	}

	_, err := requestSTSCredentials(context.Background(), config, stsRequest{SessionName: "tfacc", DurationSeconds: 3600})
	if err == nil {
		t.Fatal("expected an error when the TLS settings cannot be turned into a transport")
	}
	if !strings.Contains(err.Error(), "failed to configure transport") {
		t.Errorf("error = %q, want it to say the transport could not be built", err)
	}
}
