package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// notifyStaleConfigStub models a MinIO server that has a pending restart: every
// set-config-kv lands in pending and answers with "restart required", while
// get-config-kv keeps serving the running config until restart() is called.
// Like Config.SetKVS it fills the subsystem defaults for keys never sent, and
// like SubsysInfo.WriteTo the served line omits empty values and enable=on.
// Unlike the real server it does not omit secret keys; tests that set one only
// assert on non-secret keys.
// See issue #1299.
type notifyStaleConfigStub struct {
	mu        sync.Mutex
	key       string
	subsystem string
	name      string
	defaults  map[string]string
	pending   map[string]string
	running   map[string]string
	visible   bool
}

func (s *notifyStaleConfigStub) line() string {
	keys := make([]string, 0, len(s.running))
	for k := range s.running {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{s.key}
	for _, k := range keys {
		v := s.running[k]
		if v == "" || (k == "enable" && v == "on") {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, " ")
}

func (s *notifyStaleConfigStub) restart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = make(map[string]string, len(s.pending))
	for k, v := range s.pending {
		s.running[k] = v
	}
	s.visible = len(s.running) > 0
}

func newNotifyStaleConfigStub(t *testing.T, key string, defaults map[string]string) (*notifyStaleConfigStub, *madmin.AdminClient) {
	t.Helper()
	subsystem, name, _ := strings.Cut(key, ":")
	stub := &notifyStaleConfigStub{
		key:       key,
		subsystem: subsystem,
		name:      name,
		defaults:  defaults,
		pending:   map[string]string{},
		running:   map[string]string{},
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
			_, rest, _ := strings.Cut(string(plain), " ")
			for k, v := range stub.defaults {
				if _, ok := stub.pending[k]; !ok {
					stub.pending[k] = v
				}
			}
			for k, v := range parseConfigParams(rest) {
				stub.pending[k] = v
			}
			w.Header().Set(madmin.ConfigAppliedHeader, "false")
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/get-config-kv"):
			if !stub.visible {
				body, err := json.Marshal(map[string]string{
					"Code":    "XMinioConfigTargetNotFound",
					"Message": fmt.Sprintf("there is no target `%s` for subsystem `%s`", stub.name, stub.subsystem),
				})
				if err != nil {
					t.Errorf("marshaling no-target error: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write(body)
				return
			}
			body, err := madmin.EncryptData("secretkey", []byte(stub.line()))
			if err != nil {
				t.Errorf("encrypting config: %v", err)
			}
			_, _ = w.Write(body)
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

// webhookStubDefaults and kafkaStubDefaults mirror DefaultWebhookKVS and
// DefaultKafkaKVS from MinIO's internal/config/notify, the defaults
// Config.SetKVS seeds into every target.
var webhookStubDefaults = map[string]string{
	"enable": "off", "endpoint": "", "auth_token": "", "queue_limit": "0",
	"queue_dir": "", "client_cert": "", "client_key": "",
}

var kafkaStubDefaults = map[string]string{
	"enable": "off", "topic": "", "brokers": "", "sasl_username": "",
	"sasl_password": "", "sasl_mechanism": "plain", "client_tls_cert": "",
	"client_tls_key": "", "tls_client_auth": "0", "sasl": "off", "tls": "off",
	"tls_skip_verify": "off", "queue_limit": "0", "queue_dir": "", "version": "",
	"batch_size": "0", "batch_commit_timeout": "0s", "compression_codec": "",
	"compression_level": "",
}

func TestNotifyUpdateKeepsPlannedValuesUntilRestart(t *testing.T) {
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
		"name":     "primary",
		"endpoint": "http://hook:8080",
		"comment":  "primary",
		"enable":   false,
	})
	if got := state.Attributes["comment"]; got != "primary" {
		t.Fatalf("comment after create = %q, want %q", got, "primary")
	}
	if got := state.Attributes["restart_required"]; got != "true" {
		t.Fatalf("restart_required after create = %q, want %q", got, "true")
	}

	stub.restart()

	state = apply(state, map[string]interface{}{
		"name":     "primary",
		"endpoint": "http://hook:8080",
		"comment":  "secondary",
		"enable":   false,
	})
	if got := state.Attributes["comment"]; got != "secondary" {
		t.Fatalf("comment after update = %q, want %q", got, "secondary")
	}
	if got := state.Attributes["restart_required"]; got != "true" {
		t.Fatalf("restart_required after update = %q, want %q", got, "true")
	}

	state, diags := res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh before restart: %v", diags)
	}
	diff, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(map[string]interface{}{
		"name":     "primary",
		"endpoint": "http://hook:8080",
		"comment":  "secondary",
		"enable":   false,
	}), meta)
	if err != nil {
		t.Fatalf("diff after refresh: %v", err)
	}
	if diff != nil && len(diff.Attributes) > 0 {
		t.Errorf("plan after refresh before restart is not empty: %v", diff.Attributes)
	}

	stub.restart()

	state, diags = res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh after restart: %v", diags)
	}
	if got := state.Attributes["comment"]; got != "secondary" {
		t.Errorf("comment after restart = %q, want %q", got, "secondary")
	}
	if got := state.Attributes["restart_required"]; got != "false" {
		t.Errorf("restart_required after restart = %q, want %q", got, "false")
	}
	if got := stub.running["enable"]; got != "off" {
		t.Errorf("enable on the server after restart = %q, want %q (a disabled target must be written as off)", got, "off")
	}
}

func TestNotifyCreateKeepsPlannedValuesWhileTargetInvisible(t *testing.T) {
	stub, client := newNotifyStaleConfigStub(t, "notify_webhook:primary", webhookStubDefaults)
	meta := &S3MinioClient{S3Admin: client}
	res := resourceMinioNotifyWebhook()
	ctx := context.Background()

	raw := map[string]interface{}{
		"name":     "primary",
		"endpoint": "http://hook:8080",
		"comment":  "primary",
		"enable":   false,
	}

	diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(raw), meta)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	state, diags := res.Apply(ctx, nil, diff, meta)
	if diags.HasError() {
		t.Fatalf("apply: %v", diags)
	}
	if got := state.Attributes["comment"]; got != "primary" {
		t.Fatalf("comment after create = %q, want %q", got, "primary")
	}
	if got := state.Attributes["restart_required"]; got != "true" {
		t.Fatalf("restart_required after create = %q, want %q", got, "true")
	}

	state, diags = res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh: %v", diags)
	}
	if got := state.Attributes["comment"]; got != "primary" {
		t.Errorf("comment after refresh = %q, want %q", got, "primary")
	}
	if got := state.Attributes["restart_required"]; got != "true" {
		t.Errorf("restart_required after refresh = %q, want %q", got, "true")
	}

	diff, err = res.Diff(ctx, state, terraform.NewResourceConfigRaw(raw), meta)
	if err != nil {
		t.Fatalf("diff after refresh: %v", err)
	}
	if diff != nil && len(diff.Attributes) > 0 {
		t.Errorf("plan after refresh is not empty: %v", diff.Attributes)
	}

	stub.restart()

	state, diags = res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh after restart: %v", diags)
	}
	if got := state.Attributes["comment"]; got != "primary" {
		t.Errorf("comment after restart = %q, want %q", got, "primary")
	}
	if got := state.Attributes["restart_required"]; got != "false" {
		t.Errorf("restart_required after restart = %q, want %q", got, "false")
	}
}

// TestNotifyKafkaRestartClearsDespiteSeededDefaults covers the defaults
// Config.SetKVS seeds into a target the provider never sent, such as
// sasl_mechanism=plain: they must not keep the restart-pending gate closed.
func TestNotifyKafkaRestartClearsDespiteSeededDefaults(t *testing.T) {
	stub, client := newNotifyStaleConfigStub(t, "notify_kafka:primary", kafkaStubDefaults)
	meta := &S3MinioClient{S3Admin: client}
	res := resourceMinioNotifyKafka()
	ctx := context.Background()

	raw := map[string]interface{}{
		"name":    "primary",
		"brokers": "kafka:9092",
		"topic":   "events",
		"enable":  false,
	}

	diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(raw), meta)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	state, diags := res.Apply(ctx, nil, diff, meta)
	if diags.HasError() {
		t.Fatalf("apply: %v", diags)
	}
	if got := state.Attributes["restart_required"]; got != "true" {
		t.Fatalf("restart_required after create = %q, want %q", got, "true")
	}

	stub.restart()

	state, diags = res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh after restart: %v", diags)
	}
	if got := state.Attributes["restart_required"]; got != "false" {
		t.Errorf("restart_required after restart = %q, want %q (seeded defaults like sasl_mechanism=plain must not hold the gate closed)", got, "false")
	}
	if got := state.Attributes["brokers"]; got != "kafka:9092" {
		t.Errorf("brokers after restart = %q, want %q", got, "kafka:9092")
	}
	if got := state.Attributes["enable"]; got != "false" {
		t.Errorf("enable after restart = %q, want %q", got, "false")
	}
}

// TestNotifyUpdateKeepsRemovedCommentUntilRestart covers the removal case of
// issue #1299: while the restart that applies a removed comment is pending,
// the server still shows the old value and must not put it back into state.
func TestNotifyUpdateKeepsRemovedCommentUntilRestart(t *testing.T) {
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
		"name":     "primary",
		"endpoint": "http://hook:8080",
		"comment":  "primary",
	})
	stub.restart()

	raw := map[string]interface{}{
		"name":     "primary",
		"endpoint": "http://hook:8080",
	}
	state = apply(state, raw)
	if got := state.Attributes["comment"]; got != "" {
		t.Fatalf("comment after removing it = %q, want empty", got)
	}
	if got := state.Attributes["restart_required"]; got != "true" {
		t.Fatalf("restart_required after removing comment = %q, want %q", got, "true")
	}

	state, diags := res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh before restart: %v", diags)
	}
	if got := state.Attributes["comment"]; got != "" {
		t.Errorf("comment after refresh before restart = %q, want empty (the server still shows the pre-restart value)", got)
	}
	diff, err := res.Diff(ctx, state, terraform.NewResourceConfigRaw(raw), meta)
	if err != nil {
		t.Fatalf("diff after refresh: %v", err)
	}
	if diff != nil && len(diff.Attributes) > 0 {
		t.Errorf("plan after refresh before restart is not empty: %v", diff.Attributes)
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

// TestNotifyEnableFlipKeepsPlannedValuesUntilRestart covers the upgrade path:
// a target the older provider wrote as enabled while the config disables it.
// The server prints enable only when off, so an enabled target shows no enable
// key and the gate must read that as still-on while the fix is pending.
func TestNotifyEnableFlipKeepsPlannedValuesUntilRestart(t *testing.T) {
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

	rawOn := map[string]interface{}{
		"name":     "primary",
		"endpoint": "http://hook:8080",
	}
	state := apply(nil, rawOn)
	stub.restart()

	state, diags := res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh after create restart: %v", diags)
	}
	if got := state.Attributes["enable"]; got != "true" {
		t.Fatalf("enable after restart with no enable key on the server = %q, want %q (missing means on)", got, "true")
	}
	if got := state.Attributes["restart_required"]; got != "false" {
		t.Fatalf("restart_required after restart = %q, want %q", got, "false")
	}

	rawOff := map[string]interface{}{
		"name":     "primary",
		"endpoint": "http://hook:8080",
		"enable":   false,
	}
	state = apply(state, rawOff)
	if got := state.Attributes["enable"]; got != "false" {
		t.Fatalf("enable after disabling = %q, want %q", got, "false")
	}

	state, diags = res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh before second restart: %v", diags)
	}
	if got := state.Attributes["enable"]; got != "false" {
		t.Errorf("enable after refresh before restart = %q, want %q (the server shows no enable key while still on)", got, "false")
	}
	if got := state.Attributes["restart_required"]; got != "true" {
		t.Errorf("restart_required before second restart = %q, want %q", got, "true")
	}

	stub.restart()

	state, diags = res.RefreshWithoutUpgrade(ctx, state, meta)
	if diags.HasError() {
		t.Fatalf("refresh after second restart: %v", diags)
	}
	if got := state.Attributes["enable"]; got != "false" {
		t.Errorf("enable after second restart = %q, want %q", got, "false")
	}
	if got := state.Attributes["restart_required"]; got != "false" {
		t.Errorf("restart_required after second restart = %q, want %q", got, "false")
	}
}

// TestNotifyReadRemovesStateWhenTargetGone covers issue #1306: with
// restart_required already cleared, a "there is no target" answer means the
// target was removed outside Terraform, and the read must drop it from state.
func TestNotifyReadRemovesStateWhenTargetGone(t *testing.T) {
	_, client := newNotifyStaleConfigStub(t, "notify_webhook:primary", webhookStubDefaults)
	meta := &S3MinioClient{S3Admin: client}
	res := resourceMinioNotifyWebhook()
	ctx := context.Background()

	d := res.TestResourceData()
	d.SetId("primary")
	_ = d.Set("name", "primary")
	_ = d.Set("endpoint", "http://hook:8080")
	_ = d.Set("enable", false)
	_ = d.Set("restart_required", false)

	// The stub starts invisible, so get-config-kv answers "there is no target".
	if diags := res.ReadContext(ctx, d, meta); diags.HasError() {
		t.Fatalf("read: %v", diags)
	}
	if d.Id() != "" {
		t.Fatalf("target id after read = %q, want empty (a removed target must leave the state once restart_required is false)", d.Id())
	}
}

// TestNotifyDataSourceReadKeepsNameWhileTargetInvisible covers the data source
// path of issue #1306: it carries no restart_required state, so a "there is no
// target" answer is always "restart pending" and the read must keep the name.
func TestNotifyDataSourceReadKeepsNameWhileTargetInvisible(t *testing.T) {
	_, client := newNotifyStaleConfigStub(t, "notify_webhook:primary", webhookStubDefaults)
	meta := &S3MinioClient{S3Admin: client}
	ds := dataSourceMinioNotifyWebhook()
	ctx := context.Background()

	d := ds.TestResourceData()
	_ = d.Set("name", "primary")

	// The stub starts invisible, so get-config-kv answers "there is no target".
	if diags := ds.ReadContext(ctx, d, meta); diags.HasError() {
		t.Fatalf("data source read: %v", diags)
	}
	if got := d.Id(); got != "primary" {
		t.Fatalf("data source id after read = %q, want %q (a data source must not drop an invisible target)", got, "primary")
	}
	if got := d.Get("name").(string); got != "primary" {
		t.Errorf("name after data source read = %q, want %q", got, "primary")
	}
}

// TestNotifyReadKeepsStateOnEmptyResponse covers a response that carries no
// keys for the target: the read must keep the state values, not store schema
// defaults for keys the response never had.
func TestNotifyReadKeepsStateOnEmptyResponse(t *testing.T) {
	stub, client := newNotifyStaleConfigStub(t, "notify_webhook:primary", webhookStubDefaults)
	meta := &S3MinioClient{S3Admin: client}
	res := resourceMinioNotifyWebhook()
	ctx := context.Background()

	d := res.TestResourceData()
	d.SetId("primary")
	_ = d.Set("name", "primary")
	_ = d.Set("endpoint", "http://hook:8080")
	_ = d.Set("comment", "primary")
	_ = d.Set("enable", false)

	stub.mu.Lock()
	stub.running = map[string]string{}
	stub.visible = true
	stub.mu.Unlock()

	if diags := res.ReadContext(ctx, d, meta); diags.HasError() {
		t.Fatalf("read: %v", diags)
	}
	if got := d.Get("comment").(string); got != "primary" {
		t.Errorf("comment after empty response = %q, want %q", got, "primary")
	}
	if got := d.Get("enable").(bool); got {
		t.Errorf("enable after empty response = %v, want false (an empty response must not flip enable to on)", got)
	}
}

func TestNotifyTargetConfigParams(t *testing.T) {
	tests := []struct {
		name     string
		response string
		key      string
		want     map[string]string
		wantOK   bool
	}{
		{
			name:     "bare single line",
			response: "endpoint=http://example.com enable=off",
			key:      "notify_webhook:x",
			want:     map[string]string{"endpoint": "http://example.com", "enable": "off"},
			wantOK:   true,
		},
		{
			name:     "named line among other targets",
			response: "notify_webhook:x endpoint=http://x enable=off\nnotify_webhook:y endpoint=http://y",
			key:      "notify_webhook:x",
			want:     map[string]string{"endpoint": "http://x", "enable": "off"},
			wantOK:   true,
		},
		{
			name:     "key with no value",
			response: "notify_webhook:x",
			key:      "notify_webhook:x",
			want:     map[string]string{},
			wantOK:   true,
		},
		{
			name:     "several lines without the target",
			response: "notify_webhook:y endpoint=http://y\nnotify_webhook:z endpoint=http://z",
			key:      "notify_webhook:x",
			want:     map[string]string{},
			wantOK:   false,
		},
		{
			name:     "quoted value with a space",
			response: `comment="primary webhook" enable=off`,
			key:      "notify_webhook:x",
			want:     map[string]string{"comment": "primary webhook", "enable": "off"},
			wantOK:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := notifyTargetConfigParams(tc.response, tc.key)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("params = %v, want %v", got, tc.want)
			}
		})
	}
}
