package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// helpJSON builds a help-config-kv response for a subsystem carrying the
// given keys, the shape the server prints (see the KeysHelp field in
// madmin-go config-help-commands.go).
func helpJSON(subsystem string, keys ...string) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf(`{"key":%q,"description":"","optional":true,"type":""}`, k))
	}
	return fmt.Sprintf(`{"subSys":%q,"multipleTargets":true,"keysHelp":[%s]}`, subsystem, strings.Join(parts, ","))
}

func amqpHelpJSON(keys ...string) string {
	return helpJSON("notify_amqp", keys...)
}

// gatedKeysTestServer answers help-config-kv with the given JSON and counts
// the help calls it received.
func gatedKeysTestServer(t *testing.T, helpBody string) (*madmin.AdminClient, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/help-config-kv") {
			hits++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, helpBody)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating the admin client: %v", err)
	}
	return admin, &hits
}

func TestNotifyCheckGatedKeys(t *testing.T) {
	ctx := context.Background()
	base := []string{"enable", "url", "exchange", "exchange_type", "routing_key", "mandatory", "durable", "no_wait", "internal", "auto_deleted", "delivery_mode"}
	oldServer := amqpHelpJSON(append(append([]string{}, base...), "queue_dir", "queue_limit", "comment")...)
	newServer := amqpHelpJSON(append(append([]string{}, base...), "publisher_confirms", "queue_dir", "queue_limit", "comment")...)

	t.Run("old server refuses the gated key and names the release", func(t *testing.T) {
		admin, hits := gatedKeysTestServer(t, oldServer)
		_, err := notifyCheckGatedKeys(ctx, admin, "notify_amqp", "url=http://x publisher_confirms=on")
		if err == nil {
			t.Fatal("expected an error for a server without publisher_confirms")
		}
		// The literal release is the one verified against minio/minio
		// (commit ed6cc66c, first tag RELEASE.2021-06-17T00-10-46Z), so a
		// typo in the map fails here.
		for _, want := range []string{"publisher_confirms", "notify_amqp", "RELEASE.2021-06-17T00-10-46Z"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the error must contain %q, got: %v", want, err)
			}
		}
		if *hits != 1 {
			t.Errorf("expected exactly one help call, got %d", *hits)
		}
	})

	t.Run("old server drops the explicit off", func(t *testing.T) {
		admin, _ := gatedKeysTestServer(t, oldServer)
		got, err := notifyCheckGatedKeys(ctx, admin, "notify_amqp", "url=http://x publisher_confirms=off exchange=logs")
		if err != nil {
			t.Fatalf("expected no error for an explicit off, got: %v", err)
		}
		if want := "url=http://x exchange=logs"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("old server drops the off next to a literal quote in a value", func(t *testing.T) {
		admin, _ := gatedKeysTestServer(t, oldServer)
		got, err := notifyCheckGatedKeys(ctx, admin, "notify_amqp", `url=http://x routing_key=ev"ents publisher_confirms=off`)
		if err != nil {
			t.Fatalf("expected no error for an explicit off, got: %v", err)
		}
		if want := `url=http://x routing_key=ev"ents`; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("fail loud when the strip cannot drop the key", func(t *testing.T) {
		admin, _ := gatedKeysTestServer(t, oldServer)
		// A quoted off parses as off but its field text does not match the
		// token the strip looks for, so the strip misses it: the guard must
		// refuse instead of sending a key the server does not have.
		_, err := notifyCheckGatedKeys(ctx, admin, "notify_amqp", `url=x publisher_confirms="off"`)
		if err == nil {
			t.Fatal("expected the fail-loud guard to refuse")
		}
		if !strings.Contains(err.Error(), "failed to drop") {
			t.Errorf("error = %v, want it to name the failed strip", err)
		}
	})

	t.Run("new server accepts the gated key unchanged", func(t *testing.T) {
		admin, _ := gatedKeysTestServer(t, newServer)
		cfg := "url=http://x publisher_confirms=on"
		got, err := notifyCheckGatedKeys(ctx, admin, "notify_amqp", cfg)
		if err != nil {
			t.Fatalf("expected no error for a server with publisher_confirms, got: %v", err)
		}
		if got != cfg {
			t.Errorf("the config must pass through unchanged, got: %q", got)
		}
	})

	t.Run("target not using the gated key never pays the help call", func(t *testing.T) {
		admin, hits := gatedKeysTestServer(t, oldServer)
		if _, err := notifyCheckGatedKeys(ctx, admin, "notify_amqp", "url=http://x exchange=logs"); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if *hits != 0 {
			t.Errorf("expected no help call, got %d", *hits)
		}
	})

	t.Run("subsystem without gated keys never pays the help call", func(t *testing.T) {
		admin, hits := gatedKeysTestServer(t, oldServer)
		if _, err := notifyCheckGatedKeys(ctx, admin, "notify_webhook", "url=http://x"); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if *hits != 0 {
			t.Errorf("expected no help call, got %d", *hits)
		}
	})

	t.Run("a failing help call fails closed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/help-config-kv") {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)
		admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
			Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
			Secure: false,
		})
		if err != nil {
			t.Fatalf("creating the admin client: %v", err)
		}
		if _, err := notifyCheckGatedKeys(ctx, admin, "notify_amqp", "url=http://x publisher_confirms=on"); err == nil {
			t.Fatal("expected the failed help call to fail the check closed")
		} else if !strings.Contains(err.Error(), "asking the server") {
			t.Errorf("error = %v, want it to name the failed help call", err)
		}
	})
}

func TestNotifyStripParam(t *testing.T) {
	if got := notifyStripParam("url=http://x publisher_confirms=off comment=ok", "publisher_confirms=off"); got != "url=http://x comment=ok" {
		t.Errorf("got %q", got)
	}
	// A token inside a quoted value must survive: only whole fields match.
	if got := notifyStripParam(`comment="keep publisher_confirms=off here" publisher_confirms=off`, "publisher_confirms=off"); got != `comment="keep publisher_confirms=off here"` {
		t.Errorf("got %q", got)
	}
	// A literal quote inside an unquoted value does not open a quoted value
	// (validateConfigValue allows a"b), the same rule parseConfigParams uses.
	if got := notifyStripParam(`routing_key=ev"ents publisher_confirms=off enable=on`, "publisher_confirms=off"); got != `routing_key=ev"ents enable=on` {
		t.Errorf("got %q", got)
	}
	// A value holding a quote and a space is quoted by buildConfigParam in a
	// way the parser reads as a short value plus junk: the strip must still
	// remove only the gated field and leave the junk byte for byte, exactly
	// as a server that has the key would receive it.
	if got := notifyStripParam(`routing_key="a"b c" mandatory=off publisher_confirms=off enable=on`, "publisher_confirms=off"); got != `routing_key="a"b c" mandatory=off enable=on` {
		t.Errorf("got %q", got)
	}
	// Same with the junk sitting directly in front of the gated key: the
	// splice cuts from the key, so the junk survives and the neighbours do
	// not fuse.
	if got := notifyStripParam(`routing_key="a"b c" publisher_confirms=off enable=on`, "publisher_confirms=off"); got != `routing_key="a"b c" enable=on` {
		t.Errorf("got %q", got)
	}
	if got := notifyStripParam(`routing_key="a"b c" publisher_confirms=off`, "publisher_confirms=off"); got != `routing_key="a"b c"` {
		t.Errorf("got %q", got)
	}
}

// TestNotifyGatedSubsystemsHaveResources keeps notifyVersionGatedKeys honest:
// every gated subsystem must have a matching resource, so a new map entry
// cannot point at a subsystem the provider does not manage.
func TestNotifyGatedSubsystemsHaveResources(t *testing.T) {
	p := Provider()
	for subsystem := range notifyVersionGatedKeys {
		if _, ok := p.ResourcesMap["minio_"+subsystem]; !ok {
			t.Errorf("notifyVersionGatedKeys has subsystem %q but there is no minio_%s resource", subsystem, subsystem)
		}
	}
}

// TestNotifyGatedKeysHooks proves the create and update paths really call the
// guard: deleting either call from notifyCreate or notifyUpdate fails these.
func TestNotifyGatedKeysHooks(t *testing.T) {
	ctx := context.Background()
	oldHelp := []string{"enable", "url", "exchange", "exchange_type", "routing_key"}

	newMeta := func(t *testing.T, helpKeys []string) (*notifyConfigStub, *S3MinioClient) {
		t.Helper()
		stub, client := newNotifyConfigStub(t, "notify_amqp:primary")
		stub.helpKeys = helpKeys
		return stub, &S3MinioClient{S3Admin: client}
	}

	amqpRaw := func(confirms bool) map[string]interface{} {
		raw := map[string]interface{}{
			"name": "primary",
			"url":  "amqp://user:pass@rabbit:5672",
		}
		if confirms {
			raw["publisher_confirms"] = true
		}
		return raw
	}

	t.Run("create refuses on an old server and writes nothing", func(t *testing.T) {
		stub, meta := newMeta(t, oldHelp)
		res := resourceMinioNotifyAmqp()
		diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(amqpRaw(true)), meta)
		if err != nil {
			t.Fatalf("diff: %v", err)
		}
		_, diags := res.Apply(ctx, nil, diff, meta)
		if !diags.HasError() {
			t.Fatal("expected the create to refuse the gated key")
		}
		if len(stub.sets) != 0 {
			t.Errorf("expected no set-config-kv call, got %v", stub.sets)
		}
	})

	t.Run("create drops the explicit off for an old server", func(t *testing.T) {
		stub, meta := newMeta(t, oldHelp)
		res := resourceMinioNotifyAmqp()
		diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(amqpRaw(false)), meta)
		if err != nil {
			t.Fatalf("diff: %v", err)
		}
		_, diags := res.Apply(ctx, nil, diff, meta)
		if diags.HasError() {
			t.Fatalf("apply: %v", diags)
		}
		if len(stub.sets) != 1 {
			t.Fatalf("expected one set-config-kv call, got %v", stub.sets)
		}
		want := "notify_amqp:primary url=amqp://user:pass@rabbit:5672 mandatory=off durable=off no_wait=off internal=off auto_deleted=off enable=on"
		if stub.sets[0] != want {
			t.Errorf("the off must be dropped for a server without the key: got %q, want %q", stub.sets[0], want)
		}
	})

	t.Run("update drops the explicit off for an old server", func(t *testing.T) {
		stub, meta := newMeta(t, oldHelp)
		res := resourceMinioNotifyAmqp()
		diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(amqpRaw(false)), meta)
		if err != nil {
			t.Fatalf("diff: %v", err)
		}
		state, diags := res.Apply(ctx, nil, diff, meta)
		if diags.HasError() {
			t.Fatalf("create apply: %v", diags)
		}

		updated := amqpRaw(false)
		updated["url"] = "amqp://user:pass@rabbit2:5672"
		diff2, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(updated), meta)
		if err != nil {
			t.Fatalf("update diff: %v", err)
		}
		_, diags2 := res.Apply(ctx, state, diff2, meta)
		if diags2.HasError() {
			t.Fatalf("update apply: %v", diags2)
		}
		if len(stub.sets) != 2 {
			t.Fatalf("expected the create and the update set-config-kv calls, got %v", stub.sets)
		}
		want := "notify_amqp:primary url=amqp://user:pass@rabbit2:5672 mandatory=off durable=off no_wait=off internal=off auto_deleted=off enable=on"
		if stub.sets[1] != want {
			t.Errorf("the update must drop the off too: got %q, want %q", stub.sets[1], want)
		}
	})

	t.Run("create fails closed when the help call fails", func(t *testing.T) {
		stub, meta := newMeta(t, oldHelp)
		stub.helpFail = true
		res := resourceMinioNotifyAmqp()
		diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(amqpRaw(false)), meta)
		if err != nil {
			t.Fatalf("diff: %v", err)
		}
		_, diags := res.Apply(ctx, nil, diff, meta)
		if !diags.HasError() {
			t.Fatal("expected the create to fail when the help call fails")
		}
		if len(stub.sets) != 0 {
			t.Errorf("expected no set-config-kv call, got %v", stub.sets)
		}
	})

	t.Run("update refuses on an old server and writes nothing new", func(t *testing.T) {
		stub, meta := newMeta(t, oldHelp)
		res := resourceMinioNotifyAmqp()
		diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(amqpRaw(false)), meta)
		if err != nil {
			t.Fatalf("diff: %v", err)
		}
		state, diags := res.Apply(ctx, nil, diff, meta)
		if diags.HasError() {
			t.Fatalf("create apply: %v", diags)
		}

		diff2, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(amqpRaw(true)), meta)
		if err != nil {
			t.Fatalf("update diff: %v", err)
		}
		_, diags2 := res.Apply(ctx, state, diff2, meta)
		if !diags2.HasError() {
			t.Fatal("expected the update to refuse the gated key")
		}
		if len(stub.sets) != 1 {
			t.Errorf("expected only the create's set-config-kv call, got %v", stub.sets)
		}
	})
}

// TestGatedKeysRefusedByEveryResource drives each resource that sends a
// version-gated key against a server whose help-config-kv lacks it, and
// checks the create refuses with the first release that has the key and
// writes nothing. minio_audit_webhook has its own create, so this also proves
// a map entry reaches it.
func TestGatedKeysRefusedByEveryResource(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		res     func() *schema.Resource
		raw     map[string]interface{}
		release string
	}{
		{"logger_webhook max_retry", "logger_webhook:primary", resourceMinioLoggerWebhook,
			map[string]interface{}{"name": "primary", "endpoint": "http://x.example.com/log", "max_retry": 3}, "RELEASE.2024-09-09T16-59-28Z"},
		{"logger_webhook retry_interval", "logger_webhook:primary", resourceMinioLoggerWebhook,
			map[string]interface{}{"name": "primary", "endpoint": "http://x.example.com/log", "retry_interval": "10s"}, "RELEASE.2024-09-09T16-59-28Z"},
		{"logger_webhook http_timeout", "logger_webhook:primary", resourceMinioLoggerWebhook,
			map[string]interface{}{"name": "primary", "endpoint": "http://x.example.com/log", "http_timeout": "10s"}, "RELEASE.2024-09-13T20-26-02Z"},
		{"audit_webhook max_retry", "audit_webhook:primary", resourceMinioAuditWebhook,
			map[string]interface{}{"name": "primary", "endpoint": "http://x.example.com/audit", "max_retry": 3}, "RELEASE.2024-09-09T16-59-28Z"},
		{"audit_webhook retry_interval", "audit_webhook:primary", resourceMinioAuditWebhook,
			map[string]interface{}{"name": "primary", "endpoint": "http://x.example.com/audit", "retry_interval": "10s"}, "RELEASE.2024-09-09T16-59-28Z"},
		{"audit_webhook http_timeout", "audit_webhook:primary", resourceMinioAuditWebhook,
			map[string]interface{}{"name": "primary", "endpoint": "http://x.example.com/audit", "http_timeout": "10s"}, "RELEASE.2024-09-22T00-33-43Z"},
		{"notify_kafka batch_commit_timeout", "notify_kafka:primary", resourceMinioNotifyKafka,
			map[string]interface{}{"name": "primary", "brokers": "x.example.com:9092", "topic": "t", "queue_dir": "/tmp/q", "batch_size": 10, "batch_commit_timeout": "5s"}, "RELEASE.2024-09-09T16-59-28Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			stub, client := newNotifyConfigStub(t, tc.key)
			stub.helpKeys = []string{"enable", "endpoint", "brokers", "topic"}
			meta := &S3MinioClient{S3Admin: client}
			res := tc.res()
			diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(tc.raw), meta)
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			_, diags := res.Apply(ctx, nil, diff, meta)
			if !diags.HasError() {
				t.Fatalf("create succeeded, want the gated key refused; sets: %v", stub.sets)
			}
			if msg := diags[0].Summary + " " + diags[0].Detail; !strings.Contains(msg, tc.release) {
				t.Errorf("error %q does not name the first release %s", msg, tc.release)
			}
			if len(stub.sets) != 0 {
				t.Errorf("expected no set-config-kv call, got %v", stub.sets)
			}
		})
	}
}

// TestAuditWebhookUpdateRefusesGatedKey proves minio_audit_webhook's own
// update calls the guard too.
func TestAuditWebhookUpdateRefusesGatedKey(t *testing.T) {
	ctx := context.Background()
	stub, client := newNotifyConfigStub(t, "audit_webhook:primary")
	stub.helpKeys = []string{"enable", "endpoint"}
	meta := &S3MinioClient{S3Admin: client}
	res := resourceMinioAuditWebhook()
	raw := map[string]interface{}{"name": "primary", "endpoint": "http://x.example.com/audit"}
	diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(raw), meta)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	state, diags := res.Apply(ctx, nil, diff, meta)
	if diags.HasError() {
		t.Fatalf("create without gated keys: %v", diags)
	}

	raw["max_retry"] = 3
	diff2, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(raw), meta)
	if err != nil {
		t.Fatalf("update diff: %v", err)
	}
	_, diags2 := res.Apply(ctx, state, diff2, meta)
	if !diags2.HasError() {
		t.Fatalf("update succeeded, want max_retry refused; sets: %v", stub.sets)
	}
	if len(stub.sets) != 1 {
		t.Errorf("expected only the create's set-config-kv call, got %v", stub.sets)
	}
}
