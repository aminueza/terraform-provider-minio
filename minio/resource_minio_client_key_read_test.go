package minio

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestResourceReadStoresClientKeyPathsAndDetectsDrift(t *testing.T) {
	cases := []struct {
		name           string
		stubKey        string
		id             string
		resource       func() *schema.Resource
		keyAttr        string
		initialKVs     map[string]string
		rawConfig      map[string]interface{}
		wantImported   string
		externalChange string
	}{
		{
			name:     "minio_audit_webhook",
			stubKey:  "audit_webhook:primary",
			id:       "primary",
			resource: resourceMinioAuditWebhook,
			keyAttr:  "client_key",
			initialKVs: map[string]string{
				"endpoint":    "http://audit.example.com/ingest",
				"client_cert": "/srv/tls/client.crt",
				"client_key":  "/srv/tls/client.key",
			},
			rawConfig: map[string]interface{}{
				"name":        "primary",
				"endpoint":    "http://audit.example.com/ingest",
				"client_cert": "/srv/tls/client.crt",
				"client_key":  "/srv/tls/client.key",
			},
			wantImported:   "/srv/tls/client.key",
			externalChange: "/srv/tls/rotated.key",
		},
		{
			name:     "minio_notify_webhook",
			stubKey:  "notify_webhook:primary",
			id:       "primary",
			resource: resourceMinioNotifyWebhook,
			keyAttr:  "client_key",
			initialKVs: map[string]string{
				"endpoint":    "http://hook.example.com",
				"client_cert": "/srv/tls/client.crt",
				"client_key":  "/srv/tls/client.key",
			},
			rawConfig: map[string]interface{}{
				"name":        "primary",
				"endpoint":    "http://hook.example.com",
				"client_cert": "/srv/tls/client.crt",
				"client_key":  "/srv/tls/client.key",
			},
			wantImported:   "/srv/tls/client.key",
			externalChange: "/srv/tls/rotated.key",
		},
		{
			name:     "minio_logger_webhook",
			stubKey:  "logger_webhook:primary",
			id:       "primary",
			resource: resourceMinioLoggerWebhook,
			keyAttr:  "client_key",
			initialKVs: map[string]string{
				"endpoint":    "http://logger.example.com",
				"client_cert": "/srv/tls/client.crt",
				"client_key":  "/srv/tls/client.key",
			},
			rawConfig: map[string]interface{}{
				"name":        "primary",
				"endpoint":    "http://logger.example.com",
				"client_cert": "/srv/tls/client.crt",
				"client_key":  "/srv/tls/client.key",
			},
			wantImported:   "/srv/tls/client.key",
			externalChange: "/srv/tls/rotated.key",
		},
		{
			name:     "minio_notify_nats",
			stubKey:  "notify_nats:primary",
			id:       "primary",
			resource: resourceMinioNotifyNats,
			keyAttr:  "client_key",
			initialKVs: map[string]string{
				"address":     "nats://nats:4222",
				"subject":     "events",
				"client_cert": "/srv/tls/client.crt",
				"client_key":  "/srv/tls/client.key",
			},
			rawConfig: map[string]interface{}{
				"name":        "primary",
				"address":     "nats://nats:4222",
				"subject":     "events",
				"client_cert": "/srv/tls/client.crt",
				"client_key":  "/srv/tls/client.key",
			},
			wantImported:   "/srv/tls/client.key",
			externalChange: "/srv/tls/rotated.key",
		},
		{
			name:     "minio_notify_kafka",
			stubKey:  "notify_kafka:primary",
			id:       "primary",
			resource: resourceMinioNotifyKafka,
			keyAttr:  "client_tls_key",
			initialKVs: map[string]string{
				"brokers":         "kafka:9092",
				"topic":           "events",
				"client_tls_cert": "/srv/tls/client.crt",
				"client_tls_key":  "/srv/tls/client.key",
			},
			rawConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "events",
				"client_tls_cert": "/srv/tls/client.crt",
				"client_tls_key":  "/srv/tls/client.key",
			},
			wantImported:   "/srv/tls/client.key",
			externalChange: "/srv/tls/rotated.key",
		},
		{
			name:     "minio_audit_kafka",
			stubKey:  "audit_kafka:primary",
			id:       "primary",
			resource: resourceMinioAuditKafka,
			keyAttr:  "client_tls_key",
			initialKVs: map[string]string{
				"brokers":         "kafka:9092",
				"topic":           "audit",
				"client_tls_cert": "/srv/tls/client.crt",
				"client_tls_key":  "/srv/tls/client.key",
			},
			rawConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "audit",
				"client_tls_cert": "/srv/tls/client.crt",
				"client_tls_key":  "/srv/tls/client.key",
			},
			wantImported:   "/srv/tls/client.key",
			externalChange: "/srv/tls/rotated.key",
		},
		{
			name:     "minio_server_config_etcd",
			stubKey:  "etcd",
			id:       "etcd",
			resource: resourceMinioServerConfigEtcd,
			keyAttr:  "client_cert_key",
			initialKVs: map[string]string{
				"endpoints":       "http://127.0.0.1:2379",
				"client_cert":     "/srv/tls/client.crt",
				"client_cert_key": "/srv/tls/client.key",
			},
			rawConfig: map[string]interface{}{
				"endpoints":       "http://127.0.0.1:2379",
				"client_cert":     "/srv/tls/client.crt",
				"client_cert_key": "/srv/tls/client.key",
			},
			wantImported:   "/srv/tls/client.key",
			externalChange: "/srv/tls/rotated.key",
		},
	}

	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name+"/import_drift_and_omitted_key", func(t *testing.T) {
			stub, client := newNotifyConfigStub(t, tc.stubKey)
			for k, v := range tc.initialKVs {
				stub.kvs[k] = v
			}
			meta := &S3MinioClient{S3Admin: client}
			res := tc.resource()

			// Import / Refresh reads the key path from the server response.
			state, diags := res.RefreshWithoutUpgrade(ctx, &terraform.InstanceState{
				ID:         tc.id,
				Attributes: map[string]string{"id": tc.id},
			}, meta)
			if diags.HasError() {
				t.Fatalf("refresh: %v", diags)
			}
			if got := state.Attributes[tc.keyAttr]; got != tc.wantImported {
				t.Fatalf("%s after import = %q, want %q", tc.keyAttr, got, tc.wantImported)
			}

			// Mutating the key path on the server produces a non-empty plan.
			stub.mu.Lock()
			stub.kvs[tc.keyAttr] = tc.externalChange
			stub.mu.Unlock()

			refreshed, diags := res.RefreshWithoutUpgrade(ctx, state, meta)
			if diags.HasError() {
				t.Fatalf("refresh after external change: %v", diags)
			}
			if got := refreshed.Attributes[tc.keyAttr]; got != tc.externalChange {
				t.Fatalf("%s after external change = %q, want %q", tc.keyAttr, got, tc.externalChange)
			}
			diff, err := res.Diff(ctx, refreshed, terraform.NewResourceConfigRaw(tc.rawConfig), meta)
			if err != nil {
				t.Fatalf("diff after external change: %v", err)
			}
			if diff == nil || diff.Attributes[tc.keyAttr] == nil {
				t.Fatalf("expected non-empty plan diff on %s after external change, got: %#v", tc.keyAttr, diff)
			}

			// When the server omits the key, Read keeps the state value intact.
			stub.mu.Lock()
			stub.kvs[tc.keyAttr] = tc.wantImported
			stub.hideOnRead[tc.keyAttr] = true
			stub.mu.Unlock()

			d := res.TestResourceData()
			d.SetId(tc.id)
			_ = d.Set(tc.keyAttr, "/kept/local.key")
			if diags := res.ReadContext(ctx, d, meta); diags.HasError() {
				t.Fatalf("read with omitted key: %v", diags)
			}
			if got := d.Get(tc.keyAttr).(string); got != "/kept/local.key" {
				t.Fatalf("%s when omitted from response = %q, want %q", tc.keyAttr, got, "/kept/local.key")
			}
		})
	}
}

func TestDataSourceNotifyReadsClientKeyPaths(t *testing.T) {
	cases := []struct {
		name       string
		stubKey    string
		dataSource func() *schema.Resource
		keyAttr    string
		kvs        map[string]string
	}{
		{
			name:       "data.minio_notify_webhook",
			stubKey:    "notify_webhook:primary",
			dataSource: dataSourceMinioNotifyWebhook,
			keyAttr:    "client_key",
			kvs: map[string]string{
				"endpoint":    "http://hook.example.com",
				"client_cert": "/srv/tls/client.crt",
				"client_key":  "/srv/tls/webhook.key",
			},
		},
		{
			name:       "data.minio_notify_kafka",
			stubKey:    "notify_kafka:primary",
			dataSource: dataSourceMinioNotifyKafka,
			keyAttr:    "client_tls_key",
			kvs: map[string]string{
				"brokers":         "kafka:9092",
				"topic":           "events",
				"client_tls_cert": "/srv/tls/client.crt",
				"client_tls_key":  "/srv/tls/kafka.key",
			},
		},
		{
			name:       "data.minio_notify_nats",
			stubKey:    "notify_nats:primary",
			dataSource: dataSourceMinioNotifyNats,
			keyAttr:    "client_key",
			kvs: map[string]string{
				"address":     "nats://nats:4222",
				"subject":     "events",
				"client_cert": "/srv/tls/client.crt",
				"client_key":  "/srv/tls/nats.key",
			},
		},
	}

	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newNotifyConfigStub(t, tc.stubKey)
			for k, v := range tc.kvs {
				stub.kvs[k] = v
			}

			ds := tc.dataSource()
			d := ds.TestResourceData()
			_ = d.Set("name", "primary")
			if diags := ds.ReadContext(ctx, d, &S3MinioClient{S3Admin: client}); diags.HasError() {
				t.Fatalf("reading data source %s: %v", tc.name, diags)
			}
			if got := d.Get(tc.keyAttr).(string); got != tc.kvs[tc.keyAttr] {
				t.Fatalf("%s.%s = %q, want %q", tc.name, tc.keyAttr, got, tc.kvs[tc.keyAttr])
			}
		})
	}
}

func TestNotifyClientKeyPathsRespectRestartGate(t *testing.T) {
	stub, client := newNotifyStaleConfigStub(t, "notify_webhook:primary", webhookStubDefaults)
	meta := &S3MinioClient{S3Admin: client}
	res := resourceMinioNotifyWebhook()
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

	// Create and apply initial mTLS config across restart.
	state := apply(nil, map[string]interface{}{
		"name":        "primary",
		"endpoint":    "http://hook:8080",
		"client_cert": "/certs/v1.crt",
		"client_key":  "/certs/v1.key",
	})
	stub.restart()

	state, diags := res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh after initial restart: %v", diags)
	}
	if got := state.Attributes["restart_required"]; got != "false" {
		t.Fatalf("restart_required after restart = %q, want false", got)
	}

	// Remove client_cert and client_key; before restart, get-config-kv still
	// serves /certs/v1.key, so the restart gate must hold restart_required=true
	// and keep client_key="" instead of restoring the pre-restart key.
	cleared := map[string]interface{}{
		"name":     "primary",
		"endpoint": "http://hook:8080",
	}
	state = apply(state, cleared)
	if got := state.Attributes["client_key"]; got != "" {
		t.Fatalf("client_key right after removal = %q, want empty", got)
	}
	if got := state.Attributes["restart_required"]; got != "true" {
		t.Fatalf("restart_required right after removal = %q, want true", got)
	}

	beforeRestart, diags := res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh before restart: %v", diags)
	}
	if got := beforeRestart.Attributes["client_key"]; got != "" {
		t.Fatalf("pre-restart refresh restored client_key = %q, want empty", got)
	}
	if got := beforeRestart.Attributes["restart_required"]; got != "true" {
		t.Fatalf("restart_required before restart = %q, want true", got)
	}

	// Once the server restarts, get-config-kv shows empty client_key and
	// restart_required clears cleanly.
	stub.restart()
	afterRestart, diags := res.RefreshWithoutUpgrade(ctx, beforeRestart, meta)
	if diags.HasError() {
		t.Fatalf("refresh after restart: %v", diags)
	}
	if got := afterRestart.Attributes["restart_required"]; got != "false" {
		t.Fatalf("restart_required after restart = %q, want false; pending=%v running=%v", got, stub.pending, stub.running)
	}
	if got := afterRestart.Attributes["client_key"]; got != "" {
		t.Fatalf("client_key after restart = %q, want empty", got)
	}
	if !strings.EqualFold(stub.running["client_key"], "") {
		t.Fatalf("stub running client_key = %q, want empty", stub.running["client_key"])
	}
}

func testAccMinioClientKeyDriftCase(t *testing.T, resourceType, subsystem, key, fieldsFmt, configured, outside string) {
	resourceName := resourceType + ".test"
	name := "tfacc-" + acctest.RandString(6)
	config := testAccMinioDisabledTarget(resourceType, name, fmt.Sprintf(fieldsFmt, configured))

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget(subsystem, name),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, key, configured),
					testAccCheckServerStoredTarget(subsystem, name, map[string]string{key: configured}),
				),
			},
			{
				PreConfig: func() {
					// enable=off keeps the target disabled: a write without it
					// enables the target, and MinIO then dials the endpoint.
					changed := notifyConfigKey(subsystem, name) + " " + buildConfigParam(key, outside) + " enable=off"
					if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), changed); err != nil {
						t.Fatalf("changing %s of %s outside Terraform: %v", key, name, err)
					}
					if err := testAccRestartPrimaryServer(); err != nil {
						t.Fatalf("restarting the server: %v", err)
					}
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check:              resource.TestCheckResourceAttr(resourceName, key, outside),
			},
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, key, configured),
					testAccCheckServerStoredTarget(subsystem, name, map[string]string{key: configured}),
				),
			},
			{Config: config, PlanOnly: true},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"restart_required"},
			},
		},
	})
}

func TestAccMinioAuditWebhook_clientKeyImportAndDrift(t *testing.T) {
	testAccMinioClientKeyDriftCase(t, "minio_audit_webhook", "audit_webhook", "client_key", `  endpoint    = "http://audit.example.com/ingest"
  client_cert = "/srv/tls/client-cert"
  client_key  = %q`,
		"/srv/tls/client-privkey", "/srv/tls/rotated-privkey")
}

func TestAccMinioLoggerWebhook_clientKeyImportAndDrift(t *testing.T) {
	testAccMinioClientKeyDriftCase(t, "minio_logger_webhook", "logger_webhook", "client_key", `  endpoint    = "http://log-collector.example.com/logs"
  client_cert = "/srv/tls/client-cert"
  client_key  = %q`,
		"/srv/tls/client-privkey", "/srv/tls/rotated-privkey")
}

func TestAccMinioNotifyWebhook_clientKeyImportAndDrift(t *testing.T) {
	testAccMinioClientKeyDriftCase(t, "minio_notify_webhook", "notify_webhook", "client_key", fmt.Sprintf(`  endpoint    = %q
  client_cert = "/srv/tls/client-cert"
  client_key  = %%q`, testAccEndpointURL("")),
		"/srv/tls/client-privkey", "/srv/tls/rotated-privkey")
}

func TestAccMinioNotifyNats_clientKeyImportAndDrift(t *testing.T) {
	testAccMinioClientKeyDriftCase(t, "minio_notify_nats", "notify_nats", "client_key", `  address         = "localhost:4222"
  subject         = "events"
  tls             = true
  tls_skip_verify = true
  client_cert     = "/srv/tls/client-cert"
  client_key      = %q`,
		"/srv/tls/client-privkey", "/srv/tls/rotated-privkey")
}

func TestAccMinioNotifyKafka_clientTLSKeyImportAndDrift(t *testing.T) {
	testAccMinioClientKeyDriftCase(t, "minio_notify_kafka", "notify_kafka", "client_tls_key", `  brokers         = "localhost:9092"
  topic           = "events"
  tls             = true
  tls_skip_verify = true
  client_tls_cert = "/srv/tls/client-cert"
  client_tls_key  = %q`,
		"/srv/tls/client-privkey", "/srv/tls/rotated-privkey")
}

func TestAccMinioAuditKafka_clientTLSKeyImportAndDrift(t *testing.T) {
	testAccMinioClientKeyDriftCase(t, "minio_audit_kafka", "audit_kafka", "client_tls_key", `  brokers         = "localhost:9092"
  topic           = "minio-audit"
  tls             = true
  tls_skip_verify = true
  client_tls_cert = "/srv/tls/client-cert"
  client_tls_key  = %q`,
		"/srv/tls/client-privkey", "/srv/tls/rotated-privkey")
}

func TestAccDataSourceMinioNotify_clientKeyPaths(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	baseConfig := fmt.Sprintf(`
resource "minio_notify_webhook" "test" {
  name        = %[1]q
  endpoint    = %[2]q
  enable      = false
  client_cert = "/srv/tls/client-cert"
  client_key  = "/srv/tls/client-privkey"
}

resource "minio_notify_kafka" "test" {
  name            = %[1]q
  brokers         = "localhost:9092"
  topic           = "events"
  enable          = false
  tls             = true
  tls_skip_verify = true
  client_tls_cert = "/srv/tls/client-cert"
  client_tls_key  = "/srv/tls/client-privkey"
}

resource "minio_notify_nats" "test" {
  name            = %[1]q
  address         = "localhost:4222"
  subject         = "events"
  enable          = false
  tls             = true
  tls_skip_verify = true
  client_cert     = "/srv/tls/client-cert"
  client_key      = "/srv/tls/client-privkey"
}
`, name, testAccEndpointURL(""))

	withDataSources := baseConfig + `
data "minio_notify_webhook" "test" {
  name = minio_notify_webhook.test.name
}

data "minio_notify_kafka" "test" {
  name = minio_notify_kafka.test.name
}

data "minio_notify_nats" "test" {
  name = minio_notify_nats.test.name
}
`

	// Sequential on purpose: restarts the primary test server so get-config-kv
	// returns the newly created targets to the data sources.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy: func(s *terraform.State) error {
			if err := testAccRestartPrimaryServer(); err != nil {
				return err
			}
			client := testAccClient()
			for _, subsystem := range []string{"notify_webhook", "notify_kafka", "notify_nats"} {
				target := notifyConfigKey(subsystem, name)
				data, err := client.S3Admin.GetConfigKV(context.Background(), target)
				if err == nil {
					if _, found := notifyTargetConfigParams(string(data), target); !found {
						continue
					}
					return fmt.Errorf("%s is still stored on the server after destroy: %s", target, string(data))
				}
				if !notifyConfigTargetMissing(err) {
					return fmt.Errorf("checking that %s is gone from the server: %w", target, err)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: baseConfig},
			{
				PreConfig: func() {
					if err := testAccRestartPrimaryServer(); err != nil {
						t.Fatalf("restarting primary server: %v", err)
					}
				},
				Config: withDataSources,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_notify_webhook.test", "client_key", "/srv/tls/client-privkey"),
					resource.TestCheckResourceAttr("data.minio_notify_kafka.test", "client_tls_key", "/srv/tls/client-privkey"),
					resource.TestCheckResourceAttr("data.minio_notify_nats.test", "client_key", "/srv/tls/client-privkey"),
				),
			},
		},
	})
}
