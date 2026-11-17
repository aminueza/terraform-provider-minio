package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
// See issue #1299.
type notifyStaleConfigStub struct {
	mu        sync.Mutex
	key       string
	subsystem string
	name      string
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
		parts = append(parts, fmt.Sprintf("%s=%s", k, s.running[k]))
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

func newNotifyStaleConfigStub(t *testing.T, key string) (*notifyStaleConfigStub, *madmin.AdminClient) {
	t.Helper()
	subsystem, name, _ := strings.Cut(key, ":")
	stub := &notifyStaleConfigStub{
		key:       key,
		subsystem: subsystem,
		name:      name,
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

func TestNotifyUpdateKeepsPlannedValuesUntilRestart(t *testing.T) {
	stub, client := newNotifyStaleConfigStub(t, "notify_webhook:primary")
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
	_, client := newNotifyStaleConfigStub(t, "notify_webhook:primary")
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
}
