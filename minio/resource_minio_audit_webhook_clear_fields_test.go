package minio

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAuditWebhookUpdateClearsRemovedFields(t *testing.T) {
	stub, client := newNotifyConfigStub(t, "audit_webhook:splunk")
	meta := &S3MinioClient{S3Admin: client}
	res := resourceMinioAuditWebhook()
	ctx := context.Background()

	apply := func(state *terraform.InstanceState, raw map[string]interface{}) *terraform.InstanceState {
		t.Helper()
		diff, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(raw), meta)
		if err != nil {
			t.Fatalf("diff: %v", err)
		}
		newState, diags := res.Apply(ctx, state, diff, meta)
		if diags.HasError() {
			t.Fatalf("apply: %v", diags)
		}
		return newState
	}

	state := apply(nil, map[string]interface{}{
		"name":        "splunk",
		"endpoint":    "https://splunk:8088",
		"auth_token":  "Bearer abc",
		"client_cert": "/certs/client.crt",
		"client_key":  "/certs/client.key",
	})
	if got := state.Attributes["client_cert"]; got != "/certs/client.crt" {
		t.Fatalf("client_cert after create = %q, want %q", got, "/certs/client.crt")
	}
	if stub.kvs["auth_token"] != "Bearer abc" || stub.kvs["client_key"] != "/certs/client.key" {
		t.Fatalf("server after create = %v, want the auth_token and client_key from the configuration", stub.kvs)
	}

	config := map[string]interface{}{
		"name":     "splunk",
		"endpoint": "https://splunk:8088",
	}
	state = apply(state, config)

	for _, key := range []string{"auth_token", "client_cert", "client_key"} {
		if got := stub.kvs[key]; got != "" {
			t.Errorf("server %s after removing it = %q, want empty; last set-config-kv: %q", key, got, stub.sets[len(stub.sets)-1])
		}
		if got := state.Attributes[key]; got != "" {
			t.Errorf("%s in state = %q, want empty", key, got)
		}
	}

	diff, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(config), meta)
	if err != nil {
		t.Fatalf("diff after update: %v", err)
	}
	if diff != nil && len(diff.Attributes) > 0 {
		t.Errorf("plan after update is not empty: %v", diff.Attributes)
	}
}

// auth_token is never returned on read, and older servers may omit client_key;
// an unrelated update when those fields are omitted on read keeps both on the
// server.
func TestAuditWebhookUpdateKeepsImportedSecrets(t *testing.T) {
	stub, res, meta, state := importAuditWebhookFromStub(t, "auth_token", "client_key")

	applyAuditWebhook(t, res, meta, state, map[string]interface{}{
		"name":        "splunk",
		"endpoint":    "https://splunk:8088",
		"client_cert": "/certs/client.crt",
		"batch_size":  10,
	})

	if got := stub.kvs["auth_token"]; got != "Bearer abc" {
		t.Errorf("server auth_token after an unrelated update = %q, want %q", got, "Bearer abc")
	}
	if got := stub.kvs["client_key"]; got != "/certs/client.key" {
		t.Errorf("server client_key after an unrelated update = %q, want %q", got, "/certs/client.key")
	}
}

func TestAuditWebhookUpdateKeepsImportedClientKeyWhenTheCertChanges(t *testing.T) {
	stub, res, meta, state := importAuditWebhookFromStub(t, "auth_token", "client_key")

	applyAuditWebhook(t, res, meta, state, map[string]interface{}{
		"name":        "splunk",
		"endpoint":    "https://splunk:8088",
		"client_cert": "/certs/rotated.crt",
	})

	if got := stub.kvs["client_cert"]; got != "/certs/rotated.crt" {
		t.Errorf("server client_cert = %q, want %q", got, "/certs/rotated.crt")
	}
	if got := stub.kvs["client_key"]; got != "/certs/client.key" {
		t.Errorf("server client_key after a certificate change = %q, want %q", got, "/certs/client.key")
	}
}

func TestAuditWebhookUpdateClearsImportedClientKeyWithTheCert(t *testing.T) {
	stub, res, meta, state := importAuditWebhookFromStub(t, "auth_token")

	applyAuditWebhook(t, res, meta, state, map[string]interface{}{
		"name":     "splunk",
		"endpoint": "https://splunk:8088",
	})

	for _, key := range []string{"client_cert", "client_key"} {
		if got := stub.kvs[key]; got != "" {
			t.Errorf("server %s after removing the certificate = %q, want empty; last set-config-kv: %q", key, got, stub.sets[len(stub.sets)-1])
		}
	}
	if got := stub.kvs["auth_token"]; got != "Bearer abc" {
		t.Errorf("server auth_token = %q, want %q: it was never in the configuration", got, "Bearer abc")
	}
}

func importAuditWebhookFromStub(t *testing.T, hideKeys ...string) (*notifyConfigStub, *schema.Resource, *S3MinioClient, *terraform.InstanceState) {
	t.Helper()
	stub, client := newNotifyConfigStub(t, "audit_webhook:splunk")
	stub.kvs = map[string]string{
		"endpoint":    "https://splunk:8088",
		"auth_token":  "Bearer abc",
		"client_cert": "/certs/client.crt",
		"client_key":  "/certs/client.key",
	}
	for _, k := range hideKeys {
		stub.hideOnRead[k] = true
	}
	meta := &S3MinioClient{S3Admin: client}
	res := resourceMinioAuditWebhook()

	state, diags := res.RefreshWithoutUpgrade(context.Background(), &terraform.InstanceState{
		ID:         "splunk",
		Attributes: map[string]string{"id": "splunk"},
	}, meta)
	if diags.HasError() {
		t.Fatalf("import: %v", diags)
	}
	return stub, res, meta, state
}

func applyAuditWebhook(t *testing.T, res *schema.Resource, meta *S3MinioClient, state *terraform.InstanceState, raw map[string]interface{}) {
	t.Helper()
	ctx := context.Background()
	diff, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(raw), meta)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if _, diags := res.Apply(ctx, state, diff, meta); diags.HasError() {
		t.Fatalf("apply: %v", diags)
	}
}

func TestAuditWebhookReadPicksUpClearedClientCert(t *testing.T) {
	stub, client := newNotifyConfigStub(t, "audit_webhook:splunk")
	stub.kvs = map[string]string{"endpoint": "https://splunk:8088", "client_cert": ""}

	res := resourceMinioAuditWebhook()
	d := res.TestResourceData()
	d.SetId("splunk")
	_ = d.Set("client_cert", "/certs/client.crt")

	if diags := res.ReadContext(context.Background(), d, &S3MinioClient{S3Admin: client}); diags.HasError() {
		t.Fatalf("read: %v", diags)
	}
	if got := d.Get("client_cert").(string); got != "" {
		t.Errorf("client_cert = %q, want empty (cleared on the server)", got)
	}
}
