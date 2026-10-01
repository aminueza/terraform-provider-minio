package minio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func lifecycleStubClient(t *testing.T, handler http.HandlerFunc) *minio.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client, err := minio.New(strings.TrimPrefix(srv.URL, "http://"), &minio.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
		Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("creating S3 client: %v", err)
	}
	return client
}

func TestIsLifecycleNotFoundErrorOnBackendResponses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{
			name:   "AWS and MinIO answer 404 NoSuchLifecycleConfiguration",
			status: http.StatusNotFound,
			body:   `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchLifecycleConfiguration</Code><Message>The lifecycle configuration does not exist</Message></Error>`,
			want:   true,
		},
		{
			name:   "Garage answers 204 No Content with an empty body",
			status: http.StatusNoContent,
			want:   true,
		},
		{
			name:   "access denied is not a missing configuration",
			status: http.StatusForbidden,
			body:   `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied.</Message></Error>`,
			want:   false,
		},
		{
			name:   "a server error is not a missing configuration",
			status: http.StatusInternalServerError,
			body:   `<?xml version="1.0" encoding="UTF-8"?><Error><Code>InternalError</Code><Message>We encountered an internal error.</Message></Error>`,
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				if tc.body != "" {
					w.Header().Set("Content-Type", "application/xml")
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})

			_, err := client.GetBucketLifecycle(context.Background(), "test-bucket")
			if err == nil {
				t.Fatalf("GetBucketLifecycle returned no error for status %d", tc.status)
			}
			if got := isLifecycleNotFoundError(err); got != tc.want {
				t.Errorf("isLifecycleNotFoundError(%v) = %v, want %v", err, got, tc.want)
			}
		})
	}
}

func TestCreateS3BucketLifecycleOnBackendAnswering204(t *testing.T) {
	var mu sync.Mutex
	var stored string
	var sawPut bool

	client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if !r.URL.Query().Has("lifecycle") {
			w.WriteHeader(http.StatusOK)
			return
		}

		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			stored = string(body)
			sawPut = true
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if stored == "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, stored)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	d := schema.TestResourceDataRaw(t, resourceMinioS3BucketLifecycle().Schema, map[string]interface{}{
		"bucket": "test-bucket",
		"rule": []interface{}{
			map[string]interface{}{
				"id":         "expire-logs",
				"status":     "Enabled",
				"filter":     []interface{}{map[string]interface{}{"prefix": "logs/"}},
				"expiration": []interface{}{map[string]interface{}{"days": 30}},
			},
		},
	})

	diags := minioCreateS3BucketLifecycle(context.Background(), d, &S3MinioClient{S3Client: client})
	if diags.HasError() {
		t.Fatalf("create on a backend that answers 204 for a missing configuration failed: %v", diags)
	}
	if !sawPut {
		t.Fatal("create returned without writing the lifecycle configuration")
	}
	if d.Id() != "test-bucket" {
		t.Errorf("id = %q, want %q", d.Id(), "test-bucket")
	}
	if !strings.Contains(stored, "<ID>expire-logs</ID>") || !strings.Contains(stored, "<Days>30</Days>") {
		t.Errorf("the stored configuration does not carry the rule: %s", stored)
	}
}
