package minio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// notifyConfigStub keeps one target's settings and merges every set-config-kv
// into them, as MinIO's Config.SetKVS does.
type notifyConfigStub struct {
	mu         sync.Mutex
	key        string
	kvs        map[string]string
	hideOnRead map[string]bool
	sets       []string
	// helpKeys is what the stub answers help-config-kv with. It defaults to
	// a server that has every version-gated key the provider knows; tests
	// overwrite it to simulate an older server.
	helpKeys []string
	// helpFail makes help-config-kv answer 500, to test the fail-closed path.
	helpFail bool
}

func (s *notifyConfigStub) line() string {
	keys := make([]string, 0, len(s.kvs))
	for k := range s.kvs {
		if s.hideOnRead[k] {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{s.key}
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, s.kvs[k]))
	}
	return strings.Join(parts, " ")
}

func newNotifyConfigStub(t *testing.T, key string) (*notifyConfigStub, *madmin.AdminClient) {
	t.Helper()
	stub := &notifyConfigStub{
		key:        key,
		kvs:        map[string]string{},
		hideOnRead: map[string]bool{},
		helpKeys:   []string{"publisher_confirms"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/set-config-kv"):
			plain, err := madmin.DecryptData("secretkey", r.Body)
			if err != nil {
				t.Errorf("decrypting set-config-kv body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			cfg := string(plain)
			stub.sets = append(stub.sets, cfg)
			_, rest, _ := strings.Cut(cfg, " ")
			for k, v := range parseConfigParams(rest) {
				stub.kvs[k] = v
			}
			w.Header().Set(madmin.ConfigAppliedHeader, madmin.ConfigAppliedTrue)
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/get-config-kv"):
			// A server answers this way for a target it does not have, which
			// is what Create of a dynamic target checks for (issue #1438).
			if len(stub.sets) == 0 && len(stub.kvs) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprintf(w, `{"Code":"XMinioConfigError","Message":"there is no target %s"}`, stub.key)
				return
			}
			body, err := madmin.EncryptData("secretkey", []byte(stub.line()))
			if err != nil {
				t.Errorf("encrypting config: %v", err)
			}
			_, _ = w.Write(body)
		case strings.HasSuffix(r.URL.Path, "/help-config-kv"):
			if stub.helpFail {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			sub := r.URL.Query().Get("subSys")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(helpJSON(sub, stub.helpKeys...)))
		default:
			_, _ = io.Copy(io.Discard, r.Body)
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}
	return stub, client
}

func TestNotifyUpdateClearsRemovedCommonFields(t *testing.T) {
	stub, client := newNotifyConfigStub(t, "notify_webhook:primary")
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

	state := apply(nil, map[string]interface{}{
		"name":      "primary",
		"endpoint":  "http://hook:8080",
		"comment":   "primary",
		"queue_dir": "/tmp/events",
	})
	if got := state.Attributes["comment"]; got != "primary" {
		t.Fatalf("comment after create = %q, want %q", got, "primary")
	}

	state = apply(state, map[string]interface{}{
		"name":     "primary",
		"endpoint": "http://hook:8080",
	})

	if got := stub.kvs["comment"]; got != "" {
		t.Errorf("server comment after removing it = %q, want empty; last set-config-kv: %q", got, stub.sets[len(stub.sets)-1])
	}
	if got := stub.kvs["queue_dir"]; got != "" {
		t.Errorf("server queue_dir after removing it = %q, want empty", got)
	}
	if got := state.Attributes["comment"]; got != "" {
		t.Errorf("comment in state = %q, want empty", got)
	}
	if got := state.Attributes["queue_dir"]; got != "" {
		t.Errorf("queue_dir in state = %q, want empty", got)
	}

	diff, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(map[string]interface{}{
		"name":     "primary",
		"endpoint": "http://hook:8080",
	}), meta)
	if err != nil {
		t.Fatalf("diff after update: %v", err)
	}
	if diff != nil && len(diff.Attributes) > 0 {
		t.Errorf("plan after update is not empty: %v", diff.Attributes)
	}
}

func TestNotifyReadPicksUpClearedCommentOnServer(t *testing.T) {
	stub, client := newNotifyConfigStub(t, "notify_webhook:primary")
	stub.kvs = map[string]string{"endpoint": "http://hook:8080", "comment": "", "queue_dir": ""}

	res := resourceMinioNotifyWebhook()
	d := res.TestResourceData()
	d.SetId("primary")
	_ = d.Set("comment", "primary")
	_ = d.Set("queue_dir", "/tmp/events")

	if diags := res.ReadContext(context.Background(), d, &S3MinioClient{S3Admin: client}); diags.HasError() {
		t.Fatalf("read: %v", diags)
	}
	if got := d.Get("comment").(string); got != "" {
		t.Errorf("comment = %q, want empty (cleared on the server)", got)
	}
	if got := d.Get("queue_dir").(string); got != "" {
		t.Errorf("queue_dir = %q, want empty (cleared on the server)", got)
	}
}

func TestNotifyUpdateClearsRemovedOptionalStrings(t *testing.T) {
	cases := []struct {
		name      string
		stubKey   string
		resource  func() *schema.Resource
		initial   map[string]interface{}
		minimal   map[string]interface{}
		clearKeys []string
	}{
		{
			name:     "notify_webhook",
			stubKey:  "notify_webhook:primary",
			resource: resourceMinioNotifyWebhook,
			initial: map[string]interface{}{
				"name":        "primary",
				"endpoint":    "http://hook:8080",
				"auth_token":  "Bearer abc",
				"client_cert": "/certs/client.crt",
				"client_key":  "/certs/client.key",
			},
			minimal: map[string]interface{}{
				"name":     "primary",
				"endpoint": "http://hook:8080",
			},
			clearKeys: []string{"auth_token", "client_cert", "client_key"},
		},
		{
			name:     "logger_webhook",
			stubKey:  "logger_webhook:primary",
			resource: resourceMinioLoggerWebhook,
			initial: map[string]interface{}{
				"name":        "primary",
				"endpoint":    "http://logger:8080",
				"auth_token":  "Bearer abc",
				"client_cert": "/certs/client.crt",
				"client_key":  "/certs/client.key",
				"proxy":       "http://proxy.internal:3128",
			},
			minimal: map[string]interface{}{
				"name":     "primary",
				"endpoint": "http://logger:8080",
			},
			clearKeys: []string{"auth_token", "client_cert", "client_key", "proxy"},
		},
		{
			name:     "notify_nats",
			stubKey:  "notify_nats:primary",
			resource: resourceMinioNotifyNats,
			initial: map[string]interface{}{
				"name":                 "primary",
				"address":              "nats://nats:4222",
				"subject":              "events",
				"username":             "natsuser",
				"password":             "natspass",
				"token":                "natstoken",
				"streaming_cluster_id": "test-cluster",
				"cert_authority":       "/certs/ca.crt",
				"client_cert":          "/certs/client.crt",
				"client_key":           "/certs/client.key",
			},
			minimal: map[string]interface{}{
				"name":    "primary",
				"address": "nats://nats:4222",
				"subject": "events",
			},
			clearKeys: []string{"username", "password", "token", "streaming_cluster_id", "cert_authority", "client_cert", "client_key"},
		},
		{
			name:     "notify_kafka",
			stubKey:  "notify_kafka:primary",
			resource: resourceMinioNotifyKafka,
			initial: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "events",
				"sasl":            true,
				"sasl_username":   "kuser",
				"sasl_password":   "kpass",
				"client_tls_cert": "/certs/client.crt",
				"client_tls_key":  "/certs/client.key",
				"version":         "2.8.0",
			},
			minimal: map[string]interface{}{
				"name":    "primary",
				"brokers": "kafka:9092",
				"topic":   "events",
			},
			clearKeys: []string{"sasl_username", "sasl_password", "client_tls_cert", "client_tls_key", "version"},
		},
		{
			name:     "audit_kafka",
			stubKey:  "audit_kafka:primary",
			resource: resourceMinioAuditKafka,
			initial: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "audit",
				"sasl":            true,
				"sasl_username":   "kuser",
				"sasl_password":   "kpass",
				"client_tls_cert": "/certs/client.crt",
				"client_tls_key":  "/certs/client.key",
				"version":         "2.8.0",
			},
			minimal: map[string]interface{}{
				"name":    "primary",
				"brokers": "kafka:9092",
				"topic":   "audit",
			},
			clearKeys: []string{"sasl_username", "sasl_password", "client_tls_cert", "client_tls_key", "version"},
		},
		{
			name:     "notify_mqtt",
			stubKey:  "notify_mqtt:primary",
			resource: resourceMinioNotifyMqtt,
			initial: map[string]interface{}{
				"name":     "primary",
				"broker":   "tcp://mqtt:1883",
				"topic":    "events",
				"username": "muser",
				"password": "mpass",
			},
			minimal: map[string]interface{}{
				"name":   "primary",
				"broker": "tcp://mqtt:1883",
				"topic":  "events",
			},
			clearKeys: []string{"username", "password"},
		},
		{
			name:     "notify_elasticsearch",
			stubKey:  "notify_elasticsearch:primary",
			resource: resourceMinioNotifyElasticsearch,
			initial: map[string]interface{}{
				"name":     "primary",
				"url":      "http://es:9200",
				"index":    "minio",
				"format":   "namespace",
				"username": "esuser",
				"password": "espass",
			},
			minimal: map[string]interface{}{
				"name":   "primary",
				"url":    "http://es:9200",
				"index":  "minio",
				"format": "namespace",
			},
			clearKeys: []string{"username", "password"},
		},
		{
			name:     "notify_redis",
			stubKey:  "notify_redis:primary",
			resource: resourceMinioNotifyRedis,
			initial: map[string]interface{}{
				"name":     "primary",
				"address":  "redis:6379",
				"key":      "events",
				"format":   "namespace",
				"user":     "ruser",
				"password": "rpass",
			},
			minimal: map[string]interface{}{
				"name":    "primary",
				"address": "redis:6379",
				"key":     "events",
				"format":  "namespace",
			},
			clearKeys: []string{"user", "password"},
		},
		{
			name:     "notify_amqp",
			stubKey:  "notify_amqp:primary",
			resource: resourceMinioNotifyAmqp,
			initial: map[string]interface{}{
				"name":          "primary",
				"url":           "amqp://user:pass@rabbit:5672",
				"exchange":      "events",
				"exchange_type": "fanout",
				"routing_key":   "s3.put",
			},
			minimal: map[string]interface{}{
				"name": "primary",
				"url":  "amqp://user:pass@rabbit:5672",
			},
			clearKeys: []string{"exchange", "exchange_type", "routing_key"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newNotifyConfigStub(t, tc.stubKey)
			meta := &S3MinioClient{S3Admin: client}
			res := tc.resource()
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

			state := apply(nil, tc.initial)
			for _, key := range tc.clearKeys {
				want := tc.initial[key].(string)
				if got := stub.kvs[key]; got != want {
					t.Fatalf("server %s after create = %q, want %q", key, got, want)
				}
			}

			state = apply(state, tc.minimal)
			for _, key := range tc.clearKeys {
				if got := stub.kvs[key]; got != "" {
					t.Errorf("server %s after removing it = %q, want empty; last set-config-kv: %q", key, got, stub.sets[len(stub.sets)-1])
				}
				if got := state.Attributes[key]; got != "" {
					t.Errorf("state %s after removing it = %q, want empty", key, got)
				}
			}

			diff, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(tc.minimal), meta)
			if err != nil {
				t.Fatalf("diff after update: %v", err)
			}
			if diff != nil && len(diff.Attributes) > 0 {
				t.Errorf("plan after update is not empty: %v", diff.Attributes)
			}
		})
	}
}

func TestNotifyUpdatePairSecretsAfterImport(t *testing.T) {
	cases := []struct {
		name           string
		stubKey        string
		resource       func() *schema.Resource
		serverInitial  map[string]string
		readableKey    string
		secretKey      string
		rotatedVal     string
		keepConfig     map[string]interface{}
		rotateConfig   map[string]interface{}
		removeConfig   map[string]interface{}
		keepHiddenKeys map[string]string
	}{
		{
			name:     "notify_webhook client_cert pair",
			stubKey:  "notify_webhook:primary",
			resource: resourceMinioNotifyWebhook,
			serverInitial: map[string]string{
				"endpoint":    "http://hook:8080",
				"auth_token":  "Bearer secret",
				"client_cert": "/certs/client.crt",
				"client_key":  "/certs/client.key",
			},
			readableKey: "client_cert",
			secretKey:   "client_key",
			rotatedVal:  "/certs/rotated.crt",
			keepConfig: map[string]interface{}{
				"name":        "primary",
				"endpoint":    "http://hook:8080",
				"client_cert": "/certs/client.crt",
				"comment":     "unrelated",
			},
			rotateConfig: map[string]interface{}{
				"name":        "primary",
				"endpoint":    "http://hook:8080",
				"client_cert": "/certs/rotated.crt",
			},
			removeConfig: map[string]interface{}{
				"name":     "primary",
				"endpoint": "http://hook:8080",
			},
			keepHiddenKeys: map[string]string{"auth_token": "Bearer secret"},
		},
		{
			name:     "logger_webhook client_cert pair",
			stubKey:  "logger_webhook:primary",
			resource: resourceMinioLoggerWebhook,
			serverInitial: map[string]string{
				"endpoint":    "http://logger:8080",
				"auth_token":  "Bearer secret",
				"client_cert": "/certs/client.crt",
				"client_key":  "/certs/client.key",
			},
			readableKey: "client_cert",
			secretKey:   "client_key",
			rotatedVal:  "/certs/rotated.crt",
			keepConfig: map[string]interface{}{
				"name":        "primary",
				"endpoint":    "http://logger:8080",
				"client_cert": "/certs/client.crt",
				"batch_size":  10,
			},
			rotateConfig: map[string]interface{}{
				"name":        "primary",
				"endpoint":    "http://logger:8080",
				"client_cert": "/certs/rotated.crt",
			},
			removeConfig: map[string]interface{}{
				"name":     "primary",
				"endpoint": "http://logger:8080",
			},
			keepHiddenKeys: map[string]string{"auth_token": "Bearer secret"},
		},
		{
			name:     "notify_nats client_cert pair",
			stubKey:  "notify_nats:primary",
			resource: resourceMinioNotifyNats,
			serverInitial: map[string]string{
				"address":     "nats://nats:4222",
				"subject":     "events",
				"username":    "natsuser",
				"password":    "natspass",
				"token":       "natstoken",
				"client_cert": "/certs/client.crt",
				"client_key":  "/certs/client.key",
			},
			readableKey: "client_cert",
			secretKey:   "client_key",
			rotatedVal:  "/certs/rotated.crt",
			keepConfig: map[string]interface{}{
				"name":        "primary",
				"address":     "nats://nats:4222",
				"subject":     "events",
				"username":    "natsuser",
				"client_cert": "/certs/client.crt",
				"comment":     "unrelated",
			},
			rotateConfig: map[string]interface{}{
				"name":        "primary",
				"address":     "nats://nats:4222",
				"subject":     "events",
				"username":    "natsuser",
				"client_cert": "/certs/rotated.crt",
			},
			removeConfig: map[string]interface{}{
				"name":     "primary",
				"address":  "nats://nats:4222",
				"subject":  "events",
				"username": "natsuser",
			},
			keepHiddenKeys: map[string]string{"password": "natspass", "token": "natstoken"},
		},
		{
			name:     "notify_nats username password pair",
			stubKey:  "notify_nats:primary",
			resource: resourceMinioNotifyNats,
			serverInitial: map[string]string{
				"address":     "nats://nats:4222",
				"subject":     "events",
				"username":    "natsuser",
				"password":    "natspass",
				"token":       "natstoken",
				"client_cert": "/certs/client.crt",
				"client_key":  "/certs/client.key",
			},
			readableKey: "username",
			secretKey:   "password",
			rotatedVal:  "otheruser",
			keepConfig: map[string]interface{}{
				"name":        "primary",
				"address":     "nats://nats:4222",
				"subject":     "events",
				"username":    "natsuser",
				"client_cert": "/certs/client.crt",
				"comment":     "unrelated",
			},
			rotateConfig: map[string]interface{}{
				"name":        "primary",
				"address":     "nats://nats:4222",
				"subject":     "events",
				"username":    "otheruser",
				"client_cert": "/certs/client.crt",
			},
			removeConfig: map[string]interface{}{
				"name":        "primary",
				"address":     "nats://nats:4222",
				"subject":     "events",
				"client_cert": "/certs/client.crt",
			},
			keepHiddenKeys: map[string]string{"client_key": "/certs/client.key", "token": "natstoken"},
		},
		{
			name:     "notify_elasticsearch username password pair",
			stubKey:  "notify_elasticsearch:primary",
			resource: resourceMinioNotifyElasticsearch,
			serverInitial: map[string]string{
				"url":      "http://es:9200",
				"index":    "minio",
				"format":   "namespace",
				"username": "esuser",
				"password": "espass",
			},
			readableKey: "username",
			secretKey:   "password",
			rotatedVal:  "otheruser",
			keepConfig: map[string]interface{}{
				"name":     "primary",
				"url":      "http://es:9200",
				"index":    "minio",
				"format":   "namespace",
				"username": "esuser",
				"comment":  "unrelated",
			},
			rotateConfig: map[string]interface{}{
				"name":     "primary",
				"url":      "http://es:9200",
				"index":    "minio",
				"format":   "namespace",
				"username": "otheruser",
			},
			removeConfig: map[string]interface{}{
				"name":   "primary",
				"url":    "http://es:9200",
				"index":  "minio",
				"format": "namespace",
			},
		},
		{
			name:     "notify_kafka sasl pair",
			stubKey:  "notify_kafka:primary",
			resource: resourceMinioNotifyKafka,
			serverInitial: map[string]string{
				"brokers":         "kafka:9092",
				"topic":           "events",
				"sasl":            "on",
				"sasl_username":   "kafkauser",
				"sasl_password":   "kafkapass",
				"tls":             "on",
				"client_tls_cert": "/certs/client.crt",
				"client_tls_key":  "/certs/client.key",
			},
			readableKey: "sasl_username",
			secretKey:   "sasl_password",
			rotatedVal:  "otheruser",
			keepConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "events",
				"sasl":            true,
				"sasl_username":   "kafkauser",
				"tls":             true,
				"client_tls_cert": "/certs/client.crt",
				"comment":         "unrelated",
			},
			rotateConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "events",
				"sasl":            true,
				"sasl_username":   "otheruser",
				"tls":             true,
				"client_tls_cert": "/certs/client.crt",
			},
			removeConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "events",
				"sasl":            true,
				"tls":             true,
				"client_tls_cert": "/certs/client.crt",
			},
			keepHiddenKeys: map[string]string{"client_tls_key": "/certs/client.key"},
		},
		{
			name:     "notify_kafka client_tls_cert pair",
			stubKey:  "notify_kafka:primary",
			resource: resourceMinioNotifyKafka,
			serverInitial: map[string]string{
				"brokers":         "kafka:9092",
				"topic":           "events",
				"sasl":            "on",
				"sasl_username":   "kafkauser",
				"sasl_password":   "kafkapass",
				"tls":             "on",
				"client_tls_cert": "/certs/client.crt",
				"client_tls_key":  "/certs/client.key",
			},
			readableKey: "client_tls_cert",
			secretKey:   "client_tls_key",
			rotatedVal:  "/certs/rotated.crt",
			keepConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "events",
				"sasl":            true,
				"sasl_username":   "kafkauser",
				"tls":             true,
				"client_tls_cert": "/certs/client.crt",
				"comment":         "unrelated",
			},
			rotateConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "events",
				"sasl":            true,
				"sasl_username":   "kafkauser",
				"tls":             true,
				"client_tls_cert": "/certs/rotated.crt",
			},
			removeConfig: map[string]interface{}{
				"name":          "primary",
				"brokers":       "kafka:9092",
				"topic":         "events",
				"sasl":          true,
				"sasl_username": "kafkauser",
				"tls":           true,
			},
			keepHiddenKeys: map[string]string{"sasl_password": "kafkapass"},
		},
		{
			name:     "audit_kafka sasl pair",
			stubKey:  "audit_kafka:primary",
			resource: resourceMinioAuditKafka,
			serverInitial: map[string]string{
				"brokers":         "kafka:9092",
				"topic":           "audit",
				"sasl":            "on",
				"sasl_username":   "kafkauser",
				"sasl_password":   "kafkapass",
				"tls":             "on",
				"client_tls_cert": "/certs/client.crt",
				"client_tls_key":  "/certs/client.key",
			},
			readableKey: "sasl_username",
			secretKey:   "sasl_password",
			rotatedVal:  "otheruser",
			keepConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "audit",
				"sasl":            true,
				"sasl_username":   "kafkauser",
				"tls":             true,
				"client_tls_cert": "/certs/client.crt",
				"comment":         "unrelated",
			},
			rotateConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "audit",
				"sasl":            true,
				"sasl_username":   "otheruser",
				"tls":             true,
				"client_tls_cert": "/certs/client.crt",
			},
			removeConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "audit",
				"sasl":            true,
				"tls":             true,
				"client_tls_cert": "/certs/client.crt",
			},
			keepHiddenKeys: map[string]string{"client_tls_key": "/certs/client.key"},
		},
		{
			name:     "audit_kafka client_tls_cert pair",
			stubKey:  "audit_kafka:primary",
			resource: resourceMinioAuditKafka,
			serverInitial: map[string]string{
				"brokers":         "kafka:9092",
				"topic":           "audit",
				"sasl":            "on",
				"sasl_username":   "kafkauser",
				"sasl_password":   "kafkapass",
				"tls":             "on",
				"client_tls_cert": "/certs/client.crt",
				"client_tls_key":  "/certs/client.key",
			},
			readableKey: "client_tls_cert",
			secretKey:   "client_tls_key",
			rotatedVal:  "/certs/rotated.crt",
			keepConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "audit",
				"sasl":            true,
				"sasl_username":   "kafkauser",
				"tls":             true,
				"client_tls_cert": "/certs/client.crt",
				"comment":         "unrelated",
			},
			rotateConfig: map[string]interface{}{
				"name":            "primary",
				"brokers":         "kafka:9092",
				"topic":           "audit",
				"sasl":            true,
				"sasl_username":   "kafkauser",
				"tls":             true,
				"client_tls_cert": "/certs/rotated.crt",
			},
			removeConfig: map[string]interface{}{
				"name":          "primary",
				"brokers":       "kafka:9092",
				"topic":         "audit",
				"sasl":          true,
				"sasl_username": "kafkauser",
				"tls":           true,
			},
			keepHiddenKeys: map[string]string{"sasl_password": "kafkapass"},
		},
		{
			name:     "notify_mqtt username password pair",
			stubKey:  "notify_mqtt:primary",
			resource: resourceMinioNotifyMqtt,
			serverInitial: map[string]string{
				"broker":   "tcp://mqtt:1883",
				"topic":    "events",
				"username": "mqttuser",
				"password": "mqttpass",
			},
			readableKey: "username",
			secretKey:   "password",
			rotatedVal:  "otheruser",
			keepConfig: map[string]interface{}{
				"name":     "primary",
				"broker":   "tcp://mqtt:1883",
				"topic":    "events",
				"username": "mqttuser",
				"comment":  "unrelated",
			},
			rotateConfig: map[string]interface{}{
				"name":     "primary",
				"broker":   "tcp://mqtt:1883",
				"topic":    "events",
				"username": "otheruser",
			},
			removeConfig: map[string]interface{}{
				"name":   "primary",
				"broker": "tcp://mqtt:1883",
				"topic":  "events",
			},
		},
	}

	importState := func(t *testing.T, tcStubKey string, resourceFn func() *schema.Resource, serverInitial map[string]string, hideKeys ...string) (*notifyConfigStub, *schema.Resource, *S3MinioClient, *terraform.InstanceState) {
		t.Helper()
		stub, client := newNotifyConfigStub(t, tcStubKey)
		for k, v := range serverInitial {
			stub.kvs[k] = v
		}
		for _, k := range hideKeys {
			stub.hideOnRead[k] = true
		}
		meta := &S3MinioClient{S3Admin: client}
		res := resourceFn()
		state, diags := res.RefreshWithoutUpgrade(context.Background(), &terraform.InstanceState{
			ID:         "primary",
			Attributes: map[string]string{"id": "primary"},
		}, meta)
		if diags.HasError() {
			t.Fatalf("import: %v", diags)
		}
		return stub, res, meta, state
	}

	applyRaw := func(t *testing.T, res *schema.Resource, meta *S3MinioClient, state *terraform.InstanceState, raw map[string]interface{}) {
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

	for _, tc := range cases {
		hideKeys := []string{tc.secretKey}
		for k := range tc.keepHiddenKeys {
			hideKeys = append(hideKeys, k)
		}

		t.Run(tc.name+"/unrelated update keeps hidden secrets", func(t *testing.T) {
			stub, res, meta, state := importState(t, tc.stubKey, tc.resource, tc.serverInitial, hideKeys...)
			applyRaw(t, res, meta, state, tc.keepConfig)

			wantSecret := tc.serverInitial[tc.secretKey]
			if got := stub.kvs[tc.secretKey]; got != wantSecret {
				t.Errorf("server %s after unrelated update = %q, want %q", tc.secretKey, got, wantSecret)
			}
			for k, want := range tc.keepHiddenKeys {
				if got := stub.kvs[k]; got != want {
					t.Errorf("server %s after unrelated update = %q, want %q", k, got, want)
				}
			}
		})

		t.Run(tc.name+"/changing readable field keeps hidden half", func(t *testing.T) {
			stub, res, meta, state := importState(t, tc.stubKey, tc.resource, tc.serverInitial, hideKeys...)
			applyRaw(t, res, meta, state, tc.rotateConfig)

			if got := stub.kvs[tc.readableKey]; got != tc.rotatedVal {
				t.Errorf("server %s = %q, want %q", tc.readableKey, got, tc.rotatedVal)
			}
			wantSecret := tc.serverInitial[tc.secretKey]
			if got := stub.kvs[tc.secretKey]; got != wantSecret {
				t.Errorf("server %s after changing %s = %q, want %q", tc.secretKey, tc.readableKey, got, wantSecret)
			}
		})

		t.Run(tc.name+"/removing readable field clears hidden half", func(t *testing.T) {
			stub, res, meta, state := importState(t, tc.stubKey, tc.resource, tc.serverInitial, hideKeys...)
			applyRaw(t, res, meta, state, tc.removeConfig)

			for _, key := range []string{tc.readableKey, tc.secretKey} {
				if got := stub.kvs[key]; got != "" {
					t.Errorf("server %s after removing %s = %q, want empty; last set-config-kv: %q", key, tc.readableKey, got, stub.sets[len(stub.sets)-1])
				}
			}
			for k, want := range tc.keepHiddenKeys {
				if got := stub.kvs[k]; got != want {
					t.Errorf("server %s = %q, want %q: unrelated secret was not in config", k, got, want)
				}
			}
		})
	}
}

func TestNotifyRedisUserRemovalKeepsPasswordAfterImport(t *testing.T) {
	// Redis authenticates with `AUTH <password>` when no user is set, so a
	// password is valid on its own and `user` is not the readable half of a
	// pair: removing `user` after an import must clear only `user` and leave
	// the server's password in place.
	stub, client := newNotifyConfigStub(t, "notify_redis:primary")
	stub.kvs["address"] = "redis:6379"
	stub.kvs["key"] = "events"
	stub.kvs["format"] = "namespace"
	stub.kvs["user"] = "redisuser"
	stub.kvs["password"] = "redispass"
	meta := &S3MinioClient{S3Admin: client}
	res := resourceMinioNotifyRedis()
	ctx := context.Background()

	state, diags := res.RefreshWithoutUpgrade(ctx, &terraform.InstanceState{
		ID:         "primary",
		Attributes: map[string]string{"id": "primary"},
	}, meta)
	if diags.HasError() {
		t.Fatalf("import: %v", diags)
	}

	raw := map[string]interface{}{
		"name":    "primary",
		"address": "redis:6379",
		"key":     "events",
		"format":  "namespace",
	}
	diff, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(raw), meta)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if _, diags := res.Apply(ctx, state, diff, meta); diags.HasError() {
		t.Fatalf("apply: %v", diags)
	}

	if got := stub.kvs["user"]; got != "" {
		t.Errorf("server user after removing it = %q, want empty; last set-config-kv: %q", got, stub.sets[len(stub.sets)-1])
	}
	if got := stub.kvs["password"]; got != "redispass" {
		t.Errorf("server password after removing user = %q, want it kept; last set-config-kv: %q", got, stub.sets[len(stub.sets)-1])
	}
}

func TestNotifyReadPicksUpClearedFields(t *testing.T) {
	cases := []struct {
		name     string
		stubKey  string
		resource func() *schema.Resource
		required map[string]string
		cleared  map[string]string
	}{
		{
			name:     "notify_webhook",
			stubKey:  "notify_webhook:primary",
			resource: resourceMinioNotifyWebhook,
			required: map[string]string{"endpoint": "http://hook:8080"},
			cleared:  map[string]string{"client_cert": "/certs/client.crt"},
		},
		{
			name:     "logger_webhook",
			stubKey:  "logger_webhook:primary",
			resource: resourceMinioLoggerWebhook,
			required: map[string]string{"endpoint": "http://logger:8080"},
			cleared:  map[string]string{"client_cert": "/certs/client.crt", "proxy": "http://proxy:3128"},
		},
		{
			name:     "notify_nats",
			stubKey:  "notify_nats:primary",
			resource: resourceMinioNotifyNats,
			required: map[string]string{"address": "nats://nats:4222", "subject": "events"},
			cleared: map[string]string{
				"username":             "natsuser",
				"streaming_cluster_id": "cluster",
				"cert_authority":       "/certs/ca.crt",
				"client_cert":          "/certs/client.crt",
			},
		},
		{
			name:     "notify_kafka",
			stubKey:  "notify_kafka:primary",
			resource: resourceMinioNotifyKafka,
			required: map[string]string{"brokers": "kafka:9092", "topic": "events"},
			cleared:  map[string]string{"sasl_username": "kuser", "client_tls_cert": "/certs/client.crt", "version": "2.8.0"},
		},
		{
			name:     "audit_kafka",
			stubKey:  "audit_kafka:primary",
			resource: resourceMinioAuditKafka,
			required: map[string]string{"brokers": "kafka:9092", "topic": "audit"},
			cleared:  map[string]string{"sasl_username": "kuser", "client_tls_cert": "/certs/client.crt", "version": "2.8.0"},
		},
		{
			name:     "notify_mqtt",
			stubKey:  "notify_mqtt:primary",
			resource: resourceMinioNotifyMqtt,
			required: map[string]string{"broker": "tcp://mqtt:1883", "topic": "events"},
			cleared:  map[string]string{"username": "muser"},
		},
		{
			name:     "notify_elasticsearch",
			stubKey:  "notify_elasticsearch:primary",
			resource: resourceMinioNotifyElasticsearch,
			required: map[string]string{"url": "http://es:9200", "index": "minio", "format": "namespace"},
			cleared:  map[string]string{"username": "esuser"},
		},
		{
			name:     "notify_redis",
			stubKey:  "notify_redis:primary",
			resource: resourceMinioNotifyRedis,
			required: map[string]string{"address": "redis:6379", "key": "events", "format": "namespace"},
			cleared:  map[string]string{"user": "ruser"},
		},
		{
			name:     "notify_amqp",
			stubKey:  "notify_amqp:primary",
			resource: resourceMinioNotifyAmqp,
			required: map[string]string{"url": "amqp://user:pass@rabbit:5672"},
			cleared:  map[string]string{"exchange": "events", "exchange_type": "fanout", "routing_key": "s3.put"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newNotifyConfigStub(t, tc.stubKey)
			for k, v := range tc.required {
				stub.kvs[k] = v
			}
			res := tc.resource()
			d := res.TestResourceData()
			d.SetId("primary")
			for k, prev := range tc.cleared {
				stub.kvs[k] = ""
				_ = d.Set(k, prev)
			}

			if diags := res.ReadContext(context.Background(), d, &S3MinioClient{S3Admin: client}); diags.HasError() {
				t.Fatalf("read: %v", diags)
			}
			for k := range tc.cleared {
				if got := d.Get(k).(string); got != "" {
					t.Errorf("%s = %q, want empty (cleared on the server)", k, got)
				}
			}
		})
	}
}

func TestNotifyUpdateKeepsRemovedOptionalStringUntilRestart(t *testing.T) {
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

	state := apply(nil, map[string]interface{}{
		"name":        "primary",
		"endpoint":    "http://hook:8080",
		"client_cert": "/certs/client.crt",
		"client_key":  "/certs/client.key",
	})
	stub.restart()

	raw := map[string]interface{}{
		"name":     "primary",
		"endpoint": "http://hook:8080",
	}
	state = apply(state, raw)
	if got := state.Attributes["client_cert"]; got != "" {
		t.Fatalf("client_cert after removing it = %q, want empty", got)
	}
	if got := state.Attributes["restart_required"]; got != "true" {
		t.Fatalf("restart_required after removing client_cert = %q, want %q", got, "true")
	}

	state, diags := res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh before restart: %v", diags)
	}
	if got := state.Attributes["client_cert"]; got != "" {
		t.Errorf("client_cert after refresh before restart = %q, want empty (server still shows pre-restart value)", got)
	}
	if got := state.Attributes["restart_required"]; got != "true" {
		t.Errorf("restart_required after refresh before restart = %q, want %q", got, "true")
	}

	stub.restart()

	state, diags = res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh after restart: %v", diags)
	}
	if got := state.Attributes["restart_required"]; got != "false" {
		t.Errorf("restart_required after restart = %q, want %q", got, "false")
	}
}

func TestAccMinioNotifyWebhook_clearRemovedFields(t *testing.T) {
	resourceName := "minio_notify_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	withFields := testAccMinioNotifyDisabledTarget("webhook", name, `  endpoint    = "http://hook.example.com/in"
  auth_token  = "Bearer tfacc"
  client_cert = "/srv/tls/client-cert"
  client_key  = "/srv/tls/client-privkey"`)
	cleared := testAccMinioNotifyDisabledTarget("webhook", name, `  endpoint = "http://hook.example.com/in"`)

	// Sequential on purpose: the server check restarts the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("notify_webhook", name),
		Steps: []resource.TestStep{
			{
				Config: withFields,
				Check:  resource.TestCheckResourceAttr(resourceName, "client_cert", "/srv/tls/client-cert"),
			},
			{
				Config: cleared,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "client_cert", ""),
					testAccCheckServerStoredTarget("notify_webhook", name, map[string]string{
						"auth_token":  "",
						"client_cert": "",
						"client_key":  "",
					}),
				),
			},
			{Config: cleared, PlanOnly: true},
		},
	})
}

func TestAccMinioLoggerWebhook_clearRemovedFields(t *testing.T) {
	resourceName := "minio_logger_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	withFields := testAccMinioDisabledTarget("minio_logger_webhook", name, `  endpoint    = "http://logger.example.com/in"
  auth_token  = "Bearer tfacc"
  client_cert = "/srv/tls/client-cert"
  client_key  = "/srv/tls/client-privkey"
  proxy       = "http://proxy.example.com:3128"`)
	cleared := testAccMinioDisabledTarget("minio_logger_webhook", name, `  endpoint = "http://logger.example.com/in"`)

	// Sequential on purpose: the server check restarts the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("logger_webhook", name),
		Steps: []resource.TestStep{
			{
				Config: withFields,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "client_cert", "/srv/tls/client-cert"),
					resource.TestCheckResourceAttr(resourceName, "proxy", "http://proxy.example.com:3128"),
				),
			},
			{
				Config: cleared,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "client_cert", ""),
					resource.TestCheckResourceAttr(resourceName, "proxy", ""),
					testAccCheckServerStoredTarget("logger_webhook", name, map[string]string{
						"auth_token":  "",
						"client_cert": "",
						"client_key":  "",
						"proxy":       "",
					}),
				),
			},
			{Config: cleared, PlanOnly: true},
		},
	})
}

func TestAccMinioNotifyAmqp_clearRemovedFields(t *testing.T) {
	resourceName := "minio_notify_amqp.test"
	name := "tfacc-" + acctest.RandString(6)

	withFields := testAccMinioNotifyDisabledTarget("amqp", name, `  url           = "amqp://user:pass@localhost:5672"
  exchange      = "events"
  exchange_type = "fanout"
  routing_key   = "s3.put"`)
	cleared := testAccMinioNotifyDisabledTarget("amqp", name, `  url = "amqp://user:pass@localhost:5672"`)

	// Sequential on purpose: the server check restarts the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("notify_amqp", name),
		Steps: []resource.TestStep{
			{
				Config: withFields,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "exchange", "events"),
					resource.TestCheckResourceAttr(resourceName, "exchange_type", "fanout"),
					resource.TestCheckResourceAttr(resourceName, "routing_key", "s3.put"),
				),
			},
			{
				Config: cleared,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "exchange", ""),
					resource.TestCheckResourceAttr(resourceName, "exchange_type", ""),
					resource.TestCheckResourceAttr(resourceName, "routing_key", ""),
					testAccCheckServerStoredTarget("notify_amqp", name, map[string]string{
						"exchange":      "",
						"exchange_type": "",
						"routing_key":   "",
					}),
				),
			},
			{Config: cleared, PlanOnly: true},
		},
	})
}
