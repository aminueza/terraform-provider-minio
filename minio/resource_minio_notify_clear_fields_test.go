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

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// notifyConfigStub keeps one target's settings and merges every set-config-kv
// into them, as MinIO's Config.SetKVS does.
type notifyConfigStub struct {
	mu   sync.Mutex
	key  string
	kvs  map[string]string
	sets []string
}

func (s *notifyConfigStub) line() string {
	keys := make([]string, 0, len(s.kvs))
	for k := range s.kvs {
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
	stub := &notifyConfigStub{key: key, kvs: map[string]string{}}

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
