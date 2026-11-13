package minio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestParseConfigParamsQuotedValues(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]string
	}{
		{
			name: "plain values",
			in:   "endpoint=http://hook:8080 queue_limit=10",
			want: map[string]string{"endpoint": "http://hook:8080", "queue_limit": "10"},
		},
		{
			name: "quoted value with spaces",
			in:   `endpoint=http://hook:8080 queue_limit=10 comment="primary webhook"`,
			want: map[string]string{"endpoint": "http://hook:8080", "queue_limit": "10", "comment": "primary webhook"},
		},
		{
			name: "quoted value in the middle",
			in:   `comment="primary webhook" queue_limit=10`,
			want: map[string]string{"comment": "primary webhook", "queue_limit": "10"},
		},
		{
			name: "empty values",
			in:   "queue_dir= queue_limit=0",
			want: map[string]string{"queue_dir": "", "queue_limit": "0"},
		},
		{
			name: "value containing an equals sign",
			in:   "auth_token=a=b queue_limit=1",
			want: map[string]string{"auth_token": "a=b", "queue_limit": "1"},
		},
		{
			name: "empty input",
			in:   "",
			want: map[string]string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseConfigParams(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseConfigParams(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestNotifyReadKeepsQuotedComment(t *testing.T) {
	const serverConfig = `notify_webhook:primary endpoint=http://hook:8080 queue_limit=10 comment="primary webhook"`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/get-config-kv") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
		body, err := madmin.EncryptData("secretkey", []byte(serverConfig))
		if err != nil {
			t.Errorf("encrypting config: %v", err)
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	client, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}

	res := resourceMinioNotifyWebhook()
	d := res.TestResourceData()
	d.SetId("primary")

	if diags := res.ReadContext(context.Background(), d, &S3MinioClient{S3Admin: client}); diags.HasError() {
		t.Fatalf("read failed: %v", diags)
	}

	if got := d.Get("comment").(string); got != "primary webhook" {
		t.Errorf("comment = %q, want %q", got, "primary webhook")
	}
	if got := d.Get("endpoint").(string); got != "http://hook:8080" {
		t.Errorf("endpoint = %q, want %q", got, "http://hook:8080")
	}
	if got := d.Get("queue_limit").(int); got != 10 {
		t.Errorf("queue_limit = %d, want 10", got)
	}
}
