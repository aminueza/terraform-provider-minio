package minio

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestKafkaSaslCfgBuildAndRead(t *testing.T) {
	targets := []struct {
		name  string
		res   *schema.Resource
		build func(*schema.ResourceData, interface{}) string
		read  func(map[string]string, *schema.ResourceData) diag.Diagnostics
	}{
		{
			name:  "minio_notify_kafka",
			res:   resourceMinioNotifyKafka(),
			build: buildNotifyKafkaCfg,
			read:  readNotifyKafkaFields,
		},
		{
			name:  "minio_audit_kafka",
			res:   resourceMinioAuditKafka(),
			build: buildAuditKafkaCfg,
			read:  readAuditKafkaFields,
		},
	}

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				raw      map[string]interface{}
				wantSend string
			}{
				{
					name:     "omitted",
					raw:      map[string]interface{}{"name": "primary", "brokers": "localhost:9092", "topic": "events"},
					wantSend: "sasl=off",
				},
				{
					name:     "false",
					raw:      map[string]interface{}{"name": "primary", "brokers": "localhost:9092", "topic": "events", "sasl": false},
					wantSend: "sasl=off",
				},
				{
					name:     "true",
					raw:      map[string]interface{}{"name": "primary", "brokers": "localhost:9092", "topic": "events", "sasl": true},
					wantSend: "sasl=on",
				},
			} {
				t.Run("build_"+tc.name, func(t *testing.T) {
					d := schema.TestResourceDataRaw(t, target.res.Schema, tc.raw)
					got := target.build(d, nil)
					if !strings.Contains(got, tc.wantSend) {
						t.Errorf("%s build = %q, want %q", target.name, got, tc.wantSend)
					}
				})
			}

			for _, tc := range []struct {
				name     string
				cfgMap   map[string]string
				wantSasl bool
			}{
				{name: "on", cfgMap: map[string]string{"sasl": "on"}, wantSasl: true},
				{name: "off", cfgMap: map[string]string{"sasl": "off"}, wantSasl: false},
				{name: "missing", cfgMap: map[string]string{}, wantSasl: false},
			} {
				t.Run("read_"+tc.name, func(t *testing.T) {
					d := schema.TestResourceDataRaw(t, target.res.Schema, map[string]interface{}{
						"name": "primary", "brokers": "localhost:9092", "topic": "events",
					})
					if diags := target.read(tc.cfgMap, d); diags.HasError() {
						t.Fatalf("read failed: %v", diags)
					}
					if got := d.Get("sasl").(bool); got != tc.wantSasl {
						t.Errorf("%s read sasl = %t, want %t", target.name, got, tc.wantSasl)
					}
				})
			}
		})
	}

	t.Run("data.minio_notify_kafka", func(t *testing.T) {
		ds := dataSourceMinioNotifyKafka()
		s, ok := ds.Schema["sasl"]
		if !ok {
			t.Fatal("data.minio_notify_kafka schema missing sasl")
		}
		if s.Type != schema.TypeBool || !s.Computed || s.Optional || s.Required {
			t.Fatalf("data.minio_notify_kafka.sasl schema = {Type:%v Computed:%t Optional:%t Required:%t}", s.Type, s.Computed, s.Optional, s.Required)
		}
		d := schema.TestResourceDataRaw(t, ds.Schema, map[string]interface{}{"name": "primary"})
		if diags := readNotifyKafkaFields(map[string]string{"sasl": "on"}, d); diags.HasError() {
			t.Fatalf("data source read failed: %v", diags)
		}
		if got := d.Get("sasl").(bool); !got {
			t.Errorf("data source sasl = %t, want true", got)
		}
	})
}

func TestKafkaSaslCustomizeDiff(t *testing.T) {
	resources := []struct {
		name string
		res  *schema.Resource
	}{
		{"minio_notify_kafka", resourceMinioNotifyKafka()},
		{"minio_audit_kafka", resourceMinioAuditKafka()},
	}

	for _, r := range resources {
		t.Run(r.name, func(t *testing.T) {
			ctx := context.Background()

			for _, errCase := range []struct {
				name string
				raw  map[string]interface{}
			}{
				{
					name: "username_without_sasl",
					raw: map[string]interface{}{
						"name":          "primary",
						"brokers":       "localhost:9092",
						"topic":         "events",
						"sasl_username": "minio",
					},
				},
				{
					name: "password_with_sasl_false",
					raw: map[string]interface{}{
						"name":          "primary",
						"brokers":       "localhost:9092",
						"topic":         "events",
						"sasl":          false,
						"sasl_password": "secret",
					},
				},
			} {
				t.Run(errCase.name, func(t *testing.T) {
					_, err := r.res.Diff(ctx, nil, terraform.NewResourceConfigRaw(errCase.raw), nil)
					if err == nil {
						t.Fatalf("expected plan error for %s, got nil", errCase.name)
					}
					if !strings.Contains(err.Error(), "sasl = true") {
						t.Errorf("error = %q, want mention of 'sasl = true'", err.Error())
					}
				})
			}

			for _, okCase := range []struct {
				name string
				raw  map[string]interface{}
			}{
				{
					name: "no_credentials_no_sasl",
					raw: map[string]interface{}{
						"name":    "primary",
						"brokers": "localhost:9092",
						"topic":   "events",
					},
				},
				{
					name: "credentials_with_sasl_true",
					raw: map[string]interface{}{
						"name":          "primary",
						"brokers":       "localhost:9092",
						"topic":         "events",
						"sasl":          true,
						"sasl_username": "minio",
						"sasl_password": "secret",
					},
				},
			} {
				t.Run(okCase.name, func(t *testing.T) {
					if _, err := r.res.Diff(ctx, nil, terraform.NewResourceConfigRaw(okCase.raw), nil); err != nil {
						t.Fatalf("unexpected diff error for %s: %v", okCase.name, err)
					}
				})
			}

			t.Run("unknown_sasl_value", func(t *testing.T) {
				cfg := terraform.NewResourceConfigShimmed(cty.ObjectVal(map[string]cty.Value{
					"name":          cty.StringVal("primary"),
					"brokers":       cty.StringVal("localhost:9092"),
					"topic":         cty.StringVal("events"),
					"sasl":          cty.UnknownVal(cty.Bool),
					"sasl_username": cty.StringVal("minio"),
				}), r.res.CoreConfigSchema())
				if _, err := r.res.Diff(ctx, nil, cfg, nil); err != nil {
					t.Fatalf("unexpected diff error for unknown sasl: %v", err)
				}
			})
		})
	}
}

func TestNotifyKafkaRestartOnSaslChange(t *testing.T) {
	stub, client := newNotifyStaleConfigStub(t, "notify_kafka:primary", kafkaStubDefaults)
	meta := &S3MinioClient{S3Admin: client}
	res := resourceMinioNotifyKafka()
	ctx := context.Background()

	apply := func(state *terraform.InstanceState, raw map[string]interface{}) *terraform.InstanceState {
		t.Helper()
		diff, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(raw), meta)
		if err != nil {
			t.Fatalf("diff: %v", err)
		}
		next, diags := res.Apply(ctx, state, diff, meta)
		if diags.HasError() {
			t.Fatalf("apply: %v", diags)
		}
		return next
	}

	rawFalse := map[string]interface{}{
		"name":    "primary",
		"brokers": "kafka:9092",
		"topic":   "events",
		"enable":  false,
		"sasl":    false,
	}
	state := apply(nil, rawFalse)
	stub.restart()
	var diags diag.Diagnostics
	state, diags = res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh after create restart: %v", diags)
	}
	if got := state.Attributes["restart_required"]; got != "false" {
		t.Fatalf("restart_required after create restart = %q, want false", got)
	}

	rawTrue := map[string]interface{}{
		"name":    "primary",
		"brokers": "kafka:9092",
		"topic":   "events",
		"enable":  false,
		"sasl":    true,
	}
	state = apply(state, rawTrue)
	if got := state.Attributes["restart_required"]; got != "true" {
		t.Fatalf("restart_required right after sasl change = %q, want true", got)
	}
	if got := state.Attributes["sasl"]; got != "true" {
		t.Fatalf("sasl right after sasl change = %q, want true", got)
	}

	stub.restart()
	state, diags = res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh after sasl restart: %v", diags)
	}
	if got := state.Attributes["restart_required"]; got != "false" {
		t.Errorf("restart_required after restart = %q, want false", got)
	}
	if got := state.Attributes["sasl"]; got != "true" {
		t.Errorf("sasl after restart = %q, want true", got)
	}
	diff, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(rawTrue), meta)
	if err != nil {
		t.Fatalf("post-restart diff: %v", err)
	}
	if !diff.Empty() {
		t.Errorf("expected empty post-restart diff, got %+v", diff.Attributes)
	}

	// Changing sasl outside Terraform must surface in refresh + diff.
	stub.mu.Lock()
	stub.pending["sasl"] = "off"
	stub.mu.Unlock()
	stub.restart()

	state, diags = res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh after outside change: %v", diags)
	}
	diff, err = res.Diff(ctx, state, terraform.NewResourceConfigRaw(rawTrue), meta)
	if err != nil {
		t.Fatalf("drift diff: %v", err)
	}
	if diff.Empty() || diff.Attributes["sasl"] == nil {
		t.Errorf("expected sasl diff after outside change, got %+v", diff.Attributes)
	}
}

func TestAccMinioNotifyKafka_sasl(t *testing.T) {
	resourceName := "minio_notify_kafka.test"
	dataSourceName := "data.minio_notify_kafka.test"
	name := "tfacc-" + acctest.RandString(6)
	config := func(sasl bool) string {
		return testAccMinioNotifyDisabledTarget("kafka", name, fmt.Sprintf(`  brokers = "localhost:9092"
  topic   = "events"
  sasl    = %t`, sasl))
	}
	withDataSource := config(true) + `
data "minio_notify_kafka" "test" {
  name = minio_notify_kafka.test.name
}
`

	// Sequential on purpose: testAccCheckServerStoredTarget restarts the
	// primary test server, which must not run underneath parallel tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("notify_kafka", name),
		Steps: []resource.TestStep{
			{
				Config: config(true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "sasl", "true"),
					resource.TestCheckResourceAttr(resourceName, "restart_required", "true"),
					testAccCheckServerStoredTarget("notify_kafka", name, map[string]string{
						"sasl":   "on",
						"enable": "off",
					}),
				),
			},
			{
				Config: withDataSource,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "sasl", "true"),
					resource.TestCheckResourceAttr(resourceName, "restart_required", "false"),
					resource.TestCheckResourceAttr(dataSourceName, "sasl", "true"),
				),
			},
			{Config: withDataSource, PlanOnly: true},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"restart_required"},
			},
			{
				Config: config(false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "sasl", "false"),
					resource.TestCheckResourceAttr(resourceName, "restart_required", "true"),
					testAccCheckServerStoredTarget("notify_kafka", name, map[string]string{
						"sasl":   "off",
						"enable": "off",
					}),
				),
			},
			{
				Config: config(false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "sasl", "false"),
					resource.TestCheckResourceAttr(resourceName, "restart_required", "false"),
				),
			},
			{Config: config(false), PlanOnly: true},
			{
				PreConfig: func() {
					changed := notifyConfigKey("notify_kafka", name) + " sasl=on enable=off"
					if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), changed); err != nil {
						t.Fatalf("changing sasl on %s outside Terraform: %v", name, err)
					}
					if err := testAccRestartPrimaryServer(); err != nil {
						t.Fatalf("restarting primary server: %v", err)
					}
				},
				Config:             config(false),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config(false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "sasl", "false"),
					testAccCheckServerStoredTarget("notify_kafka", name, map[string]string{
						"sasl":   "off",
						"enable": "off",
					}),
				),
			},
			{Config: config(false), PlanOnly: true},
		},
	})
}

func TestAccMinioNotifyKafka_saslRequiredForCredentials(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyDisabledTarget("kafka", name, `  brokers       = "localhost:9092"
  topic         = "events"
  sasl_username = "minio"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`sasl = true`),
			},
			{
				Config: testAccMinioNotifyDisabledTarget("kafka", name, `  brokers       = "localhost:9092"
  topic         = "events"
  sasl_password = "secret"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`sasl = true`),
			},
		},
	})
}
