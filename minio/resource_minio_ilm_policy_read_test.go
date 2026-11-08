package minio

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestILMPolicyReadErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   string
		gone   bool
	}{
		{name: "lifecycle removed", status: http.StatusNotFound, code: "NoSuchLifecycleConfiguration", gone: true},
		{name: "bucket deleted", status: http.StatusNotFound, code: "NoSuchBucket", gone: true},
		{name: "access denied", status: http.StatusForbidden, code: "AccessDenied", gone: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>error</Message></Error>`, tc.code)
			})

			d := resourceMinioILMPolicy().TestResourceData()
			d.SetId("test-bucket")
			if err := d.Set("bucket", "test-bucket"); err != nil {
				t.Fatal(err)
			}
			if err := d.Set("rule", []interface{}{map[string]interface{}{
				"id":         "expire",
				"status":     "Enabled",
				"expiration": "30d",
			}}); err != nil {
				t.Fatal(err)
			}

			diags := minioReadILMPolicy(context.Background(), d, &S3MinioClient{S3Client: client})

			if tc.gone {
				if diags.HasError() {
					t.Fatalf("expected no error, got %v", diags)
				}
				if d.Id() != "" {
					t.Fatalf("expected the policy to be removed from state, id is %q", d.Id())
				}
				return
			}
			if !diags.HasError() {
				t.Fatalf("expected an error for %s", tc.code)
			}
			if d.Id() == "" {
				t.Fatalf("expected the policy to stay in state on %s", tc.code)
			}
		})
	}
}

func TestILMPolicyDeleteWhenBucketIsGone(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		code    string
		wantErr bool
	}{
		{name: "bucket deleted", status: http.StatusNotFound, code: "NoSuchBucket"},
		{name: "access denied", status: http.StatusForbidden, code: "AccessDenied", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>error</Message></Error>`, tc.code)
			})

			d := resourceMinioILMPolicy().TestResourceData()
			d.SetId("test-bucket")

			diags := minioDeleteILMPolicy(context.Background(), d, &S3MinioClient{S3Client: client})

			if diags.HasError() != tc.wantErr {
				t.Fatalf("wantErr %v, got %v", tc.wantErr, diags)
			}
		})
	}
}
