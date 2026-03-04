package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TestNotifyResourceRegistration verifies that all notification resource
// constructor functions can be called without panicking.
func TestNotifyResourceRegistration(t *testing.T) {
	resources := map[string]func() *schema.Resource{
		"minio_notify_amqp":          resourceMinioNotifyAmqp,
		"minio_notify_kafka":         resourceMinioNotifyKafka,
		"minio_notify_mqtt":          resourceMinioNotifyMqtt,
		"minio_notify_nats":          resourceMinioNotifyNats,
		"minio_notify_nsq":           resourceMinioNotifyNsq,
		"minio_notify_mysql":         resourceMinioNotifyMysql,
		"minio_notify_postgres":      resourceMinioNotifyPostgres,
		"minio_notify_elasticsearch": resourceMinioNotifyElasticsearch,
		"minio_notify_redis":         resourceMinioNotifyRedis,
	}

	for name, fn := range resources {
		t.Run(name, func(t *testing.T) {
			r := fn()
			if r == nil {
				t.Fatalf("%s returned nil resource", name)
			}
			if r.Schema == nil {
				t.Fatalf("%s has nil schema", name)
			}
		})
	}
}

// TestNotifyResourceSchemas verifies that every notification resource schema
// contains the expected common fields plus the type-specific required fields.
func TestNotifyResourceSchemas(t *testing.T) {
	tests := []struct {
		name           string
		resource       func() *schema.Resource
		requiredFields []string
	}{
		{"amqp", resourceMinioNotifyAmqp, []string{"name", "url"}},
		{"kafka", resourceMinioNotifyKafka, []string{"name", "brokers", "topic"}},
		{"mqtt", resourceMinioNotifyMqtt, []string{"name", "broker", "topic"}},
		{"nats", resourceMinioNotifyNats, []string{"name", "address", "subject"}},
		{"nsq", resourceMinioNotifyNsq, []string{"name", "nsqd_address", "topic"}},
		{"mysql", resourceMinioNotifyMysql, []string{"name", "connection_string", "table", "format"}},
		{"postgres", resourceMinioNotifyPostgres, []string{"name", "connection_string", "table", "format"}},
		{"elasticsearch", resourceMinioNotifyElasticsearch, []string{"name", "url", "index", "format"}},
		{"redis", resourceMinioNotifyRedis, []string{"name", "address", "key", "format"}},
	}

	commonFields := []string{"enable", "queue_dir", "queue_limit", "comment", "restart_required"}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.resource()
			if r == nil {
				t.Fatalf("%s returned nil resource", tc.name)
			}

			for _, field := range tc.requiredFields {
				if _, ok := r.Schema[field]; !ok {
					t.Errorf("missing required field %q in %s schema", field, tc.name)
				}
			}

			for _, field := range commonFields {
				if _, ok := r.Schema[field]; !ok {
					t.Errorf("missing common field %q in %s schema", field, tc.name)
				}
			}
		})
	}
}

// TestNotifyResourceSchemaFieldTypes verifies that key schema fields have the
// correct types and attributes (Required, Optional, Computed, Sensitive).
func TestNotifyResourceSchemaFieldTypes(t *testing.T) {
	tests := []struct {
		name     string
		resource func() *schema.Resource
		checks   []fieldCheck
	}{
		{
			"amqp",
			resourceMinioNotifyAmqp,
			[]fieldCheck{
				{field: "name", wantType: schema.TypeString, wantRequired: true, wantForceNew: true},
				{field: "url", wantType: schema.TypeString, wantRequired: true, wantSensitive: true},
				{field: "exchange", wantType: schema.TypeString, wantOptional: true},
				{field: "exchange_type", wantType: schema.TypeString, wantOptional: true},
				{field: "routing_key", wantType: schema.TypeString, wantOptional: true},
				{field: "mandatory", wantType: schema.TypeBool, wantOptional: true},
				{field: "durable", wantType: schema.TypeBool, wantOptional: true},
				{field: "no_wait", wantType: schema.TypeBool, wantOptional: true},
				{field: "internal", wantType: schema.TypeBool, wantOptional: true},
				{field: "auto_deleted", wantType: schema.TypeBool, wantOptional: true},
				{field: "delivery_mode", wantType: schema.TypeInt, wantOptional: true},
			},
		},
		{
			"kafka",
			resourceMinioNotifyKafka,
			[]fieldCheck{
				{field: "name", wantType: schema.TypeString, wantRequired: true, wantForceNew: true},
				{field: "brokers", wantType: schema.TypeString, wantRequired: true},
				{field: "topic", wantType: schema.TypeString, wantRequired: true},
				{field: "sasl_password", wantType: schema.TypeString, wantOptional: true, wantSensitive: true},
				{field: "client_tls_key", wantType: schema.TypeString, wantOptional: true, wantSensitive: true},
				{field: "tls", wantType: schema.TypeBool, wantOptional: true},
				{field: "batch_size", wantType: schema.TypeInt, wantOptional: true, wantComputed: true},
			},
		},
		{
			"mqtt",
			resourceMinioNotifyMqtt,
			[]fieldCheck{
				{field: "name", wantType: schema.TypeString, wantRequired: true, wantForceNew: true},
				{field: "broker", wantType: schema.TypeString, wantRequired: true},
				{field: "topic", wantType: schema.TypeString, wantRequired: true},
				{field: "password", wantType: schema.TypeString, wantOptional: true, wantSensitive: true},
				{field: "qos", wantType: schema.TypeInt, wantOptional: true},
			},
		},
		{
			"nats",
			resourceMinioNotifyNats,
			[]fieldCheck{
				{field: "name", wantType: schema.TypeString, wantRequired: true, wantForceNew: true},
				{field: "address", wantType: schema.TypeString, wantRequired: true},
				{field: "subject", wantType: schema.TypeString, wantRequired: true},
				{field: "password", wantType: schema.TypeString, wantOptional: true, wantSensitive: true},
				{field: "token", wantType: schema.TypeString, wantOptional: true, wantSensitive: true},
				{field: "client_key", wantType: schema.TypeString, wantOptional: true, wantSensitive: true},
				{field: "jetstream", wantType: schema.TypeBool, wantOptional: true},
				{field: "streaming", wantType: schema.TypeBool, wantOptional: true},
			},
		},
		{
			"nsq",
			resourceMinioNotifyNsq,
			[]fieldCheck{
				{field: "name", wantType: schema.TypeString, wantRequired: true, wantForceNew: true},
				{field: "nsqd_address", wantType: schema.TypeString, wantRequired: true},
				{field: "topic", wantType: schema.TypeString, wantRequired: true},
				{field: "tls", wantType: schema.TypeBool, wantOptional: true},
				{field: "tls_skip_verify", wantType: schema.TypeBool, wantOptional: true},
			},
		},
		{
			"mysql",
			resourceMinioNotifyMysql,
			[]fieldCheck{
				{field: "name", wantType: schema.TypeString, wantRequired: true, wantForceNew: true},
				{field: "connection_string", wantType: schema.TypeString, wantRequired: true, wantSensitive: true},
				{field: "table", wantType: schema.TypeString, wantRequired: true},
				{field: "format", wantType: schema.TypeString, wantRequired: true},
				{field: "max_open_connections", wantType: schema.TypeInt, wantOptional: true, wantComputed: true},
			},
		},
		{
			"postgres",
			resourceMinioNotifyPostgres,
			[]fieldCheck{
				{field: "name", wantType: schema.TypeString, wantRequired: true, wantForceNew: true},
				{field: "connection_string", wantType: schema.TypeString, wantRequired: true, wantSensitive: true},
				{field: "table", wantType: schema.TypeString, wantRequired: true},
				{field: "format", wantType: schema.TypeString, wantRequired: true},
				{field: "max_open_connections", wantType: schema.TypeInt, wantOptional: true, wantComputed: true},
			},
		},
		{
			"elasticsearch",
			resourceMinioNotifyElasticsearch,
			[]fieldCheck{
				{field: "name", wantType: schema.TypeString, wantRequired: true, wantForceNew: true},
				{field: "url", wantType: schema.TypeString, wantRequired: true, wantSensitive: true},
				{field: "index", wantType: schema.TypeString, wantRequired: true},
				{field: "format", wantType: schema.TypeString, wantRequired: true},
				{field: "password", wantType: schema.TypeString, wantOptional: true, wantSensitive: true},
			},
		},
		{
			"redis",
			resourceMinioNotifyRedis,
			[]fieldCheck{
				{field: "name", wantType: schema.TypeString, wantRequired: true, wantForceNew: true},
				{field: "address", wantType: schema.TypeString, wantRequired: true},
				{field: "key", wantType: schema.TypeString, wantRequired: true},
				{field: "format", wantType: schema.TypeString, wantRequired: true},
				{field: "password", wantType: schema.TypeString, wantOptional: true, wantSensitive: true},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.resource()
			for _, fc := range tc.checks {
				s, ok := r.Schema[fc.field]
				if !ok {
					t.Errorf("field %q not found in schema", fc.field)
					continue
				}
				if s.Type != fc.wantType {
					t.Errorf("field %q: got type %v, want %v", fc.field, s.Type, fc.wantType)
				}
				if s.Required != fc.wantRequired {
					t.Errorf("field %q: got Required=%v, want %v", fc.field, s.Required, fc.wantRequired)
				}
				if s.Optional != fc.wantOptional {
					t.Errorf("field %q: got Optional=%v, want %v", fc.field, s.Optional, fc.wantOptional)
				}
				if s.Sensitive != fc.wantSensitive {
					t.Errorf("field %q: got Sensitive=%v, want %v", fc.field, s.Sensitive, fc.wantSensitive)
				}
				if s.Computed != fc.wantComputed {
					t.Errorf("field %q: got Computed=%v, want %v", fc.field, s.Computed, fc.wantComputed)
				}
				if s.ForceNew != fc.wantForceNew {
					t.Errorf("field %q: got ForceNew=%v, want %v", fc.field, s.ForceNew, fc.wantForceNew)
				}
			}
		})
	}
}

// TestNotifyCommonSchemaDefaults verifies the common schema field defaults.
func TestNotifyCommonSchemaDefaults(t *testing.T) {
	common := notifyCommonSchema()

	// "enable" should default to true
	enableSchema, ok := common["enable"]
	if !ok {
		t.Fatal("missing 'enable' in common schema")
	}
	if enableSchema.Default != true {
		t.Errorf("enable default: got %v, want true", enableSchema.Default)
	}

	// "restart_required" should be Computed-only
	restartSchema, ok := common["restart_required"]
	if !ok {
		t.Fatal("missing 'restart_required' in common schema")
	}
	if !restartSchema.Computed {
		t.Error("restart_required should be Computed")
	}
	if restartSchema.Required || restartSchema.Optional {
		t.Error("restart_required should not be Required or Optional")
	}

	// "name" should be Required and ForceNew
	nameSchema, ok := common["name"]
	if !ok {
		t.Fatal("missing 'name' in common schema")
	}
	if !nameSchema.Required {
		t.Error("name should be Required")
	}
	if !nameSchema.ForceNew {
		t.Error("name should be ForceNew")
	}
}

// fieldCheck describes the expected attributes of a single schema field.
type fieldCheck struct {
	field         string
	wantType      schema.ValueType
	wantRequired  bool
	wantOptional  bool
	wantSensitive bool
	wantComputed  bool
	wantForceNew  bool
}
