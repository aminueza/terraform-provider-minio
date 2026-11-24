package minio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func newIAMStubClient(t *testing.T, serviceAccountJSON, userInfoJSON string) *S3MinioClient {
	t.Helper()

	const stubSecret = "secretkey"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch adminPathWithoutVersion(r.URL.Path) {
		case "/info-service-account":
			encrypted, err := madmin.EncryptData(stubSecret, []byte(serviceAccountJSON))
			if err != nil {
				t.Errorf("encrypting service account response: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if _, err := w.Write(encrypted); err != nil {
				t.Errorf("writing service account response: %v", err)
			}
		case "/user-info":
			if _, err := w.Write([]byte(userInfoJSON)); err != nil {
				t.Errorf("writing user info response: %v", err)
			}
		case "/remove-canned-policy":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", stubSecret, ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating madmin stub client: %v", err)
	}

	return &S3MinioClient{S3Admin: admin}
}

func TestIAMResourcesSetResourceFieldReportsSchemaErrors(t *testing.T) {
	// TestResourceDataRaw panics on d.Set errors when TF_ACC is set; clear it so
	// setResourceField's error path is exercised cleanly under TF_ACC=1 CI runs.
	t.Setenv("TF_ACC", "")

	resources := []struct {
		name string
		res  *schema.Resource
		raw  map[string]interface{}
		key  string
		val  interface{}
	}{
		{
			name: "minio_accesskey",
			res:  resourceMinioAccessKey(),
			raw:  map[string]interface{}{"user": "alice"},
			key:  "description",
			val:  "ci key",
		},
		{
			name: "minio_iam_service_account",
			res:  resourceMinioServiceAccount(),
			raw:  map[string]interface{}{"target_user": "alice"},
			key:  "disable_user",
			val:  true,
		},
		{
			name: "minio_iam_user",
			res:  resourceMinioIAMUser(),
			raw:  map[string]interface{}{"name": "alice"},
			key:  "secret",
			val:  "supersecret123",
		},
		{
			name: "minio_iam_policy",
			res:  resourceMinioIAMPolicy(),
			raw:  map[string]interface{}{"name": "readonly", "policy": `{"Version":"2012-10-17","Statement":[]}`},
			key:  "policy",
			val:  "",
		},
	}

	for _, tc := range resources {
		t.Run(tc.name, func(t *testing.T) {
			d := schema.TestResourceDataRaw(t, tc.res.Schema, tc.raw)
			if diags := setResourceField(d, tc.key, tc.val, tc.name); diags != nil {
				t.Fatalf("unexpected error setting %q: %v", tc.key, diags)
			}

			errDiags := setResourceField(d, "nonexistent_schema_attribute", "value", tc.name)
			if errDiags == nil || !errDiags.HasError() {
				t.Fatalf("expected error setting nonexistent field, got %+v", errDiags)
			}
			if !strings.Contains(errDiags[0].Summary, "setting nonexistent_schema_attribute") {
				t.Fatalf("expected summary to mention setting nonexistent_schema_attribute, got %q", errDiags[0].Summary)
			}
		})
	}
}

func TestIAMReadAndDeleteDSetAgainstStubAdmin(t *testing.T) {
	ctx := context.Background()

	t.Run("accesskey_read_non_implied_and_implied_policy", func(t *testing.T) {
		policyJSON := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bucket/*"]}]}`
		client := newIAMStubClient(t, `{
			"parentUser": "alice",
			"accountStatus": "on",
			"description": "ci access key",
			"impliedPolicy": false,
			"policy": "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"s3:GetObject\"],\"Resource\":[\"arn:aws:s3:::bucket/*\"]}]}"
		}`, "")

		d := schema.TestResourceDataRaw(t, resourceMinioAccessKey().Schema, map[string]interface{}{
			"user":   "alice",
			"policy": `{"Version":"2012-10-17","Statement":[]}`,
		})
		d.SetId("AKIA12345678")
		if diags := minioReadAccessKey(ctx, d, client); diags.HasError() {
			t.Fatalf("minioReadAccessKey failed: %v", diags)
		}
		if got := d.Get("status").(string); got != "enabled" {
			t.Fatalf("status = %q, want %q", got, "enabled")
		}
		if got := d.Get("access_key").(string); got != "AKIA12345678" {
			t.Fatalf("access_key = %q, want %q", got, "AKIA12345678")
		}
		if got := d.Get("description").(string); got != "ci access key" {
			t.Fatalf("description = %q, want %q", got, "ci access key")
		}
		if got := d.Get("policy").(string); !strings.Contains(got, "s3:GetObject") {
			t.Fatalf("policy = %q, want the server policy to overwrite the stale state value", got)
		}

		impliedClient := newIAMStubClient(t, `{
			"parentUser": "alice",
			"accountStatus": "off",
			"description": "implied",
			"impliedPolicy": true
		}`, "")
		dImplied := schema.TestResourceDataRaw(t, resourceMinioAccessKey().Schema, map[string]interface{}{
			"user":   "alice",
			"policy": policyJSON,
		})
		dImplied.SetId("AKIA87654321")
		if diags := minioReadAccessKey(ctx, dImplied, impliedClient); diags.HasError() {
			t.Fatalf("minioReadAccessKey implied failed: %v", diags)
		}
		if got := dImplied.Get("status").(string); got != "disabled" {
			t.Fatalf("status = %q, want %q", got, "disabled")
		}
		if got := dImplied.Get("policy").(string); got != "" {
			t.Fatalf("implied policy = %q, want empty", got)
		}
	})

	t.Run("accesskey_read_empty_policy_clears_state", func(t *testing.T) {
		client := newIAMStubClient(t, `{
			"parentUser": "alice",
			"accountStatus": "on",
			"impliedPolicy": false,
			"policy": "{}"
		}`, "")
		d := schema.TestResourceDataRaw(t, resourceMinioAccessKey().Schema, map[string]interface{}{
			"user":   "alice",
			"policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bucket/*"]}]}`,
		})
		d.SetId("AKIA55555555")
		if diags := minioReadAccessKey(ctx, d, client); diags.HasError() {
			t.Fatalf("minioReadAccessKey failed: %v", diags)
		}
		if got := d.Get("policy").(string); got != "" {
			t.Fatalf("policy = %q, want empty for an empty server policy", got)
		}
	})

	t.Run("service_account_read", func(t *testing.T) {
		client := newIAMStubClient(t, `{
			"parentUser": "uid=bob,ou=people,dc=example,dc=com",
			"accountStatus": "off",
			"impliedPolicy": false,
			"policy": "{\"Version\":\"2012-10-17\",\"Statement\":[]}",
			"name": "svc",
			"description": "svc desc"
		}`, "")

		d := schema.TestResourceDataRaw(t, resourceMinioServiceAccount().Schema, map[string]interface{}{
			"target_user": "bob",
		})
		d.SetId("SVCKEY123456")
		if diags := minioReadServiceAccount(ctx, d, client); diags.HasError() {
			t.Fatalf("minioReadServiceAccount failed: %v", diags)
		}
		if got := d.Get("access_key").(string); got != "SVCKEY123456" {
			t.Fatalf("access_key = %q, want %q", got, "SVCKEY123456")
		}
		if !d.Get("disable_user").(bool) {
			t.Fatal("expected disable_user=true for off accountStatus")
		}
		if got := d.Get("name").(string); got != "svc" {
			t.Fatalf("name = %q, want %q", got, "svc")
		}
	})

	t.Run("iam_user_read_and_policy_delete", func(t *testing.T) {
		client := newIAMStubClient(t, "", `{"status":"enabled"}`)
		// No "name" in the raw config on purpose: the read must populate it
		// from the resource id.
		dUser := schema.TestResourceDataRaw(t, resourceMinioIAMUser().Schema, map[string]interface{}{})
		dUser.SetId("carol")
		if diags := minioReadUser(ctx, dUser, client); diags.HasError() {
			t.Fatalf("minioReadUser failed: %v", diags)
		}
		if got := dUser.Get("name").(string); got != "carol" {
			t.Fatalf("name = %q, want %q", got, "carol")
		}
		if got := dUser.Get("status").(string); got != "enabled" {
			t.Fatalf("status = %q, want %q", got, "enabled")
		}

		dPolicy := schema.TestResourceDataRaw(t, resourceMinioIAMPolicy().Schema, map[string]interface{}{
			"name":   "readonly",
			"policy": `{"Version":"2012-10-17","Statement":[]}`,
		})
		dPolicy.SetId("readonly")
		if diags := minioDeletePolicy(ctx, dPolicy, client); diags.HasError() {
			t.Fatalf("minioDeletePolicy failed: %v", diags)
		}
	})
}
