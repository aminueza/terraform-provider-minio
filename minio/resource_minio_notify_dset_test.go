package minio

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestNotifyCommonAndQueueSizeErrorPropagation(t *testing.T) {
	// terraform-plugin-sdk panics on d.Set schema errors when TF_ACC is set.
	t.Setenv("TF_ACC", "")

	dInvalid := resourceMinioKMSKey().TestResourceData()
	dInvalid.SetId("t1")

	diags := notifyReadCommonFields(map[string]string{"enable": "off"}, dInvalid)
	if !diags.HasError() || !strings.Contains(fmt.Sprint(diags), "setting enable (t1)") {
		t.Fatalf("notifyReadCommonFields() diags = %v, want fatal error setting enable", diags)
	}

	diags = notifyReadQueueSize(map[string]string{"queue_size": "50"}, dInvalid)
	if !diags.HasError() || !strings.Contains(fmt.Sprint(diags), "setting queue_size (t1)") {
		t.Fatalf("notifyReadQueueSize() diags = %v, want fatal error setting queue_size", diags)
	}

	dValid := resourceMinioAuditKafka().TestResourceData()
	dValid.SetId("audit1")
	if diags := notifyReadQueueSize(map[string]string{"queue_size": "250"}, dValid); diags.HasError() {
		t.Fatalf("notifyReadQueueSize(valid) failed: %v", diags)
	}
	if diags := notifyReadCommonFields(map[string]string{"queue_dir": "/tmp/q", "comment": "ok"}, dValid); diags.HasError() {
		t.Fatalf("notifyReadCommonFields(valid) failed: %v", diags)
	}
	if !dValid.Get("enable").(bool) {
		t.Error("expected enable=true when server omits enable key")
	}
	if got := dValid.Get("queue_size").(int); got != 250 {
		t.Errorf("queue_size = %d, want 250", got)
	}
	if got := dValid.Get("queue_limit").(int); got != 250 {
		t.Errorf("queue_limit = %d, want 250", got)
	}
	if got := dValid.Get("queue_dir").(string); got != "/tmp/q" {
		t.Errorf("queue_dir = %q, want %q", got, "/tmp/q")
	}
	if got := dValid.Get("comment").(string); got != "ok" {
		t.Errorf("comment = %q, want %q", got, "ok")
	}
}

func TestNotifyAuditAndLoggerReadFieldsCheckState(t *testing.T) {
	cases := []struct {
		name     string
		res      *schema.Resource
		read     func(map[string]string, *schema.ResourceData) diag.Diagnostics
		cfgMap   map[string]string
		wantStrs map[string]string
		wantInt  map[string]int
		wantBool map[string]bool
	}{
		{
			name: "audit_kafka",
			res:  resourceMinioAuditKafka(),
			read: readAuditKafkaFields,
			cfgMap: map[string]string{
				"brokers":         "b1:9092",
				"topic":           "audit",
				"sasl_username":   "u",
				"sasl_mechanism":  "plain",
				"sasl":            "on",
				"client_tls_cert": "/c.pem",
				"client_tls_key":  "/k.pem",
				"version":         "3.6.0",
				"queue_size":      "128",
				"comment":         "ak",
			},
			wantStrs: map[string]string{
				"brokers":         "b1:9092",
				"topic":           "audit",
				"sasl_username":   "u",
				"sasl_mechanism":  "plain",
				"client_tls_cert": "/c.pem",
				"client_tls_key":  "/k.pem",
				"version":         "3.6.0",
				"comment":         "ak",
			},
			wantInt:  map[string]int{"queue_size": 128, "queue_limit": 128},
			wantBool: map[string]bool{"sasl": true, "tls": false, "enable": true},
		},
		{
			name: "logger_webhook",
			res:  resourceMinioLoggerWebhook(),
			read: readLoggerWebhookFields,
			cfgMap: map[string]string{
				"endpoint":    "https://log.local",
				"client_cert": "/c.pem",
				"client_key":  "/k.pem",
				"proxy":       "http://p:8080",
				"batch_size":  "64",
				"queue_size":  "512",
				"enable":      "off",
			},
			wantStrs: map[string]string{
				"endpoint":    "https://log.local",
				"client_cert": "/c.pem",
				"client_key":  "/k.pem",
				"proxy":       "http://p:8080",
			},
			wantInt:  map[string]int{"batch_size": 64, "queue_size": 512},
			wantBool: map[string]bool{"enable": false},
		},
		{
			name: "notify_amqp",
			res:  resourceMinioNotifyAmqp(),
			read: readNotifyAmqpFields,
			cfgMap: map[string]string{
				"url":                "amqp://rmq:5672",
				"exchange":           "ex",
				"exchange_type":      "topic",
				"routing_key":        "rk",
				"mandatory":          "on",
				"durable":            "on",
				"delivery_mode":      "2",
				"publisher_confirms": "on",
			},
			wantStrs: map[string]string{"url": "amqp://rmq:5672", "exchange": "ex", "exchange_type": "topic", "routing_key": "rk"},
			wantInt:  map[string]int{"delivery_mode": 2},
			wantBool: map[string]bool{"mandatory": true, "durable": true, "no_wait": false, "internal": false, "auto_deleted": false, "publisher_confirms": true},
		},
		{
			name: "notify_elasticsearch",
			res:  resourceMinioNotifyElasticsearch(),
			read: readNotifyElasticsearchFields,
			cfgMap: map[string]string{
				"url":      "https://es:9200",
				"index":    "events",
				"format":   "access",
				"username": "esuser",
			},
			wantStrs: map[string]string{"url": "https://es:9200", "index": "events", "format": "access", "username": "esuser"},
		},
		{
			name: "notify_kafka",
			res:  resourceMinioNotifyKafka(),
			read: readNotifyKafkaFields,
			cfgMap: map[string]string{
				"brokers":         "k1:9092",
				"topic":           "events",
				"sasl_username":   "ku",
				"sasl_mechanism":  "sha256",
				"sasl":            "on",
				"tls":             "on",
				"tls_client_auth": "2",
				"client_tls_cert": "/kc.pem",
				"client_tls_key":  "/kk.pem",
				"version":         "3.5.0",
				"batch_size":      "32",
			},
			wantStrs: map[string]string{"brokers": "k1:9092", "topic": "events", "sasl_username": "ku", "sasl_mechanism": "sha256", "client_tls_cert": "/kc.pem", "client_tls_key": "/kk.pem", "version": "3.5.0"},
			wantInt:  map[string]int{"tls_client_auth": 2, "batch_size": 32},
			wantBool: map[string]bool{"sasl": true, "tls": true, "tls_skip_verify": false},
		},
		{
			name: "notify_mqtt",
			res:  resourceMinioNotifyMqtt(),
			read: readNotifyMqttFields,
			cfgMap: map[string]string{
				"broker":              "tcp://mqtt:1883",
				"topic":               "t",
				"username":            "mu",
				"qos":                 "1",
				"keep_alive_interval": "60s",
				"reconnect_interval":  "5s",
			},
			wantStrs: map[string]string{"broker": "tcp://mqtt:1883", "topic": "t", "username": "mu", "keep_alive_interval": "60s", "reconnect_interval": "5s"},
			wantInt:  map[string]int{"qos": 1},
		},
		{
			name: "notify_mysql",
			res:  resourceMinioNotifyMysql(),
			read: readNotifyMysqlFields,
			cfgMap: map[string]string{
				"dsn_string":           "user:pw@tcp(db:3306)/m",
				"table":                "events",
				"format":               "namespace",
				"max_open_connections": "10",
			},
			wantStrs: map[string]string{"connection_string": "user:pw@tcp(db:3306)/m", "table": "events", "format": "namespace"},
			wantInt:  map[string]int{"max_open_connections": 10},
		},
		{
			name: "notify_nats",
			res:  resourceMinioNotifyNats(),
			read: readNotifyNatsFields,
			cfgMap: map[string]string{
				"address":                          "nats://n:4222",
				"subject":                          "s3",
				"username":                         "nu",
				"tls":                              "on",
				"ping_interval":                    "10s",
				"jetstream":                        "on",
				"streaming":                        "on",
				"streaming_max_pub_acks_in_flight": "42",
				"streaming_cluster_id":             "cid",
				"cert_authority":                   "/ca.pem",
				"client_cert":                      "/nc.pem",
				"client_key":                       "/nk.pem",
			},
			wantStrs: map[string]string{"address": "nats://n:4222", "subject": "s3", "username": "nu", "ping_interval": "10s", "streaming_cluster_id": "cid", "cert_authority": "/ca.pem", "client_cert": "/nc.pem", "client_key": "/nk.pem"},
			wantInt:  map[string]int{"streaming_max_pub_acks_in_flight": 42},
			wantBool: map[string]bool{"tls": true, "tls_skip_verify": false, "jetstream": true, "streaming": true, "streaming_async": false},
		},
		{
			name: "notify_nsq",
			res:  resourceMinioNotifyNsq(),
			read: readNotifyNsqFields,
			cfgMap: map[string]string{
				"nsqd_address": "127.0.0.1:4150",
				"topic":        "nsq-t",
				"tls":          "on",
			},
			wantStrs: map[string]string{"nsqd_address": "127.0.0.1:4150", "topic": "nsq-t"},
			wantBool: map[string]bool{"tls": true, "tls_skip_verify": false},
		},
		{
			name: "notify_postgres",
			res:  resourceMinioNotifyPostgres(),
			read: readNotifyPostgresFields,
			cfgMap: map[string]string{
				"connection_string":    "postgres://pg/db",
				"table":                "pg_t",
				"format":               "access",
				"max_open_connections": "16",
			},
			wantStrs: map[string]string{"connection_string": "postgres://pg/db", "table": "pg_t", "format": "access"},
			wantInt:  map[string]int{"max_open_connections": 16},
		},
		{
			name: "notify_redis",
			res:  resourceMinioNotifyRedis(),
			read: readNotifyRedisFields,
			cfgMap: map[string]string{
				"address": "redis:6379",
				"key":     "rk",
				"format":  "namespace",
				"user":    "ru",
			},
			wantStrs: map[string]string{"address": "redis:6379", "key": "rk", "format": "namespace", "user": "ru"},
		},
		{
			name: "notify_webhook",
			res:  resourceMinioNotifyWebhook(),
			read: readNotifyWebhookFields,
			cfgMap: map[string]string{
				"endpoint":    "https://wh.local",
				"client_cert": "/wc.pem",
				"client_key":  "/wk.pem",
			},
			wantStrs: map[string]string{"endpoint": "https://wh.local", "client_cert": "/wc.pem", "client_key": "/wk.pem"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.res.TestResourceData()
			d.SetId("target-1")
			if diags := tc.read(tc.cfgMap, d); diags.HasError() {
				t.Fatalf("read failed: %v", diags)
			}
			for k, want := range tc.wantStrs {
				if got := d.Get(k).(string); got != want {
					t.Errorf("%s = %q, want %q", k, got, want)
				}
			}
			for k, want := range tc.wantInt {
				if got := d.Get(k).(int); got != want {
					t.Errorf("%s = %d, want %d", k, got, want)
				}
			}
			for k, want := range tc.wantBool {
				if got := d.Get(k).(bool); got != want {
					t.Errorf("%s = %v, want %v", k, got, want)
				}
			}
		})
	}
}

func TestNotifyBoolAndEmptyStringSemantics(t *testing.T) {
	// Bools are seeded true on purpose: always-written fields must reset to
	// false when the server omits the key, while present-only fields must keep
	// the previous state value. Fresh TestResourceData would read false either
	// way and prove nothing.
	t.Run("kafka_always_written_bools_reset_when_absent", func(t *testing.T) {
		d := resourceMinioNotifyKafka().TestResourceData()
		d.SetId("k1")
		if err := d.Set("tls", true); err != nil {
			t.Fatal(err)
		}
		if err := d.Set("tls_skip_verify", true); err != nil {
			t.Fatal(err)
		}
		if diags := readNotifyKafkaFields(map[string]string{"brokers": "b:9092"}, d); diags.HasError() {
			t.Fatalf("read failed: %v", diags)
		}
		if d.Get("tls").(bool) {
			t.Error("tls = true, want false: kafka tls is written even when the server omits it")
		}
		if d.Get("tls_skip_verify").(bool) {
			t.Error("tls_skip_verify = true, want false: kafka tls_skip_verify is written even when the server omits it")
		}
	})

	t.Run("audit_kafka_present_only_bools_keep_state_when_absent", func(t *testing.T) {
		d := resourceMinioAuditKafka().TestResourceData()
		d.SetId("ak1")
		if err := d.Set("tls", true); err != nil {
			t.Fatal(err)
		}
		if err := d.Set("tls_skip_verify", true); err != nil {
			t.Fatal(err)
		}
		if diags := readAuditKafkaFields(map[string]string{"brokers": "b:9092"}, d); diags.HasError() {
			t.Fatalf("read failed: %v", diags)
		}
		if !d.Get("tls").(bool) {
			t.Error("tls = false, want true kept: audit_kafka tls is written only when the server returns it")
		}
		if !d.Get("tls_skip_verify").(bool) {
			t.Error("tls_skip_verify = false, want true kept: audit_kafka tls_skip_verify is written only when the server returns it")
		}
	})

	t.Run("amqp_always_written_flags_reset_when_absent", func(t *testing.T) {
		flags := []string{"mandatory", "durable", "no_wait", "internal", "auto_deleted", "publisher_confirms"}
		d := resourceMinioNotifyAmqp().TestResourceData()
		d.SetId("am1")
		for _, k := range flags {
			if err := d.Set(k, true); err != nil {
				t.Fatalf("seeding %s: %v", k, err)
			}
		}
		if diags := readNotifyAmqpFields(map[string]string{"url": "amqp://x"}, d); diags.HasError() {
			t.Fatalf("read failed: %v", diags)
		}
		for _, k := range flags {
			if d.Get(k).(bool) {
				t.Errorf("%s = true, want false: amqp flags are written even when the server omits them", k)
			}
		}
	})

	t.Run("redis_empty_string_skips_and_mysql_present_empty_writes", func(t *testing.T) {
		d := resourceMinioNotifyRedis().TestResourceData()
		d.SetId("r1")
		if err := d.Set("address", "old:6379"); err != nil {
			t.Fatal(err)
		}
		if diags := readNotifyRedisFields(map[string]string{"address": ""}, d); diags.HasError() {
			t.Fatalf("read failed: %v", diags)
		}
		if got := d.Get("address").(string); got != "old:6379" {
			t.Errorf("address = %q, want the old value kept: redis address skips empty strings", got)
		}

		dMy := resourceMinioNotifyMysql().TestResourceData()
		dMy.SetId("m1")
		if err := dMy.Set("table", "old_t"); err != nil {
			t.Fatal(err)
		}
		if diags := readNotifyMysqlFields(map[string]string{"table": ""}, dMy); diags.HasError() {
			t.Fatalf("read failed: %v", diags)
		}
		if got := dMy.Get("table").(string); got != "" {
			t.Errorf("table = %q, want empty: mysql table is written even when empty", got)
		}
	})

	t.Run("unparseable_int_is_skipped_without_error", func(t *testing.T) {
		d := resourceMinioLoggerWebhook().TestResourceData()
		d.SetId("lw1")
		if diags := readLoggerWebhookFields(map[string]string{"batch_size": "abc"}, d); diags.HasError() {
			t.Fatalf("read failed: %v", diags)
		}
		if got := d.Get("batch_size").(int); got != 0 {
			t.Errorf("batch_size = %d, want 0 when the server value does not parse", got)
		}
	})
}

func TestNotifyCommonCreateReadUpdateChecksState(t *testing.T) {
	ctx := context.Background()
	res := resourceMinioNotifyWebhook()

	t.Run("create_and_read", func(t *testing.T) {
		_, admin := newNotifyConfigStub(t, "notify_webhook:wh1")
		meta := &S3MinioClient{S3Admin: admin}
		d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
			"name":     "wh1",
			"endpoint": "https://wh.local/hook",
			"enable":   true,
			"comment":  "c1",
		})
		if err := d.Set("restart_required", true); err != nil {
			t.Fatal(err)
		}
		if diags := res.CreateContext(ctx, d, meta); diags.HasError() {
			t.Fatalf("CreateContext failed: %v", diags)
		}
		if got := d.Get("name").(string); got != "wh1" {
			t.Errorf("name = %q, want %q", got, "wh1")
		}
		if got := d.Get("endpoint").(string); got != "https://wh.local/hook" {
			t.Errorf("endpoint = %q, want %q", got, "https://wh.local/hook")
		}
		if d.Get("restart_required").(bool) {
			t.Error("restart_required = true, want false after an applied create (the read normalizes it too)")
		}
	})

	t.Run("update_writes_restart_required_and_new_config", func(t *testing.T) {
		stub, admin := newNotifyConfigStub(t, "notify_webhook:wh3")
		meta := &S3MinioClient{S3Admin: admin}
		d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
			"name":     "wh3",
			"endpoint": "https://wh3.local/hook",
			"enable":   true,
			"comment":  "c1",
		})
		if diags := res.CreateContext(ctx, d, meta); diags.HasError() {
			t.Fatalf("CreateContext failed: %v", diags)
		}
		if err := d.Set("comment", "c2"); err != nil {
			t.Fatal(err)
		}
		if err := d.Set("restart_required", true); err != nil {
			t.Fatal(err)
		}
		if diags := res.UpdateContext(ctx, d, meta); diags.HasError() {
			t.Fatalf("UpdateContext failed: %v", diags)
		}
		if got := d.Get("comment").(string); got != "c2" {
			t.Errorf("comment = %q, want c2", got)
		}
		if d.Get("restart_required").(bool) {
			t.Error("restart_required = true, want false after an applied update (the read normalizes it too)")
		}
		joined := strings.Join(stub.sets, "\n")
		if !strings.Contains(joined, "comment=c2") {
			t.Errorf("stub never received comment=c2, set-config bodies:\n%s", joined)
		}
	})

	t.Run("pending_restart_read_preserves_name", func(t *testing.T) {
		_, admin := newNotifyStaleConfigStub(t, "notify_webhook:wh2", nil)
		meta := &S3MinioClient{S3Admin: admin}
		d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
			"name":     "wh2",
			"endpoint": "https://wh2.local/hook",
			"enable":   true,
		})
		if diags := res.CreateContext(ctx, d, meta); diags.HasError() {
			t.Fatalf("CreateContext with pending restart failed: %v", diags)
		}
		if got := d.Get("name").(string); got != "wh2" {
			t.Errorf("name = %q, want %q", got, "wh2")
		}
		if !d.Get("restart_required").(bool) {
			t.Error("expected restart_required=true while pending")
		}
	})
}
