package minio

import (
	"context"
	"testing"

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

func TestAuditWebhookUpdateKeepsUnchangedSecrets(t *testing.T) {
	stub, client := newNotifyConfigStub(t, "audit_webhook:splunk")
	meta := &S3MinioClient{S3Admin: client}
	res := resourceMinioAuditWebhook()
	ctx := context.Background()

	raw := map[string]interface{}{
		"name":       "splunk",
		"endpoint":   "https://splunk:8088",
		"auth_token": "Bearer abc",
	}
	diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(raw), meta)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	state, diags := res.Apply(ctx, nil, diff, meta)
	if diags.HasError() {
		t.Fatalf("apply: %v", diags)
	}

	raw["batch_size"] = 10
	diff, err = res.Diff(ctx, state, terraform.NewResourceConfigRaw(raw), meta)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if _, diags := res.Apply(ctx, state, diff, meta); diags.HasError() {
		t.Fatalf("apply: %v", diags)
	}

	if got := stub.kvs["auth_token"]; got != "Bearer abc" {
		t.Errorf("server auth_token after an unrelated update = %q, want %q", got, "Bearer abc")
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
