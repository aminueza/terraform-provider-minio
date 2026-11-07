package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMinioReadKMSKey(t *testing.T) {
	cases := []struct {
		name        string
		statusCode  int
		body        string
		newResource bool
		wantErr     bool
		wantGone    bool
	}{
		{
			name:       "existing key stays in state",
			statusCode: http.StatusOK,
			body:       `{"key-id":"test-key"}`,
		},
		{
			name:       "key deleted from the built-in KMS or MinIO KMS",
			statusCode: http.StatusOK,
			body:       `{"key-id":"test-key","encryption-error":"key with given key ID does not exist"}`,
			wantGone:   true,
		},
		{
			name:       "key deleted from KES",
			statusCode: http.StatusOK,
			body:       `{"key-id":"test-key","encryption-error":"key does not exist"}`,
			wantGone:   true,
		},
		{
			name:       "other encryption error is reported",
			statusCode: http.StatusOK,
			body:       `{"key-id":"test-key","encryption-error":"kms is unavailable"}`,
			wantErr:    true,
		},
		{
			name:       "unrelated does-not-exist error is reported",
			statusCode: http.StatusOK,
			body:       `{"key-id":"test-key","encryption-error":"failed to generate data key with KMS key: enclave does not exist"}`,
			wantErr:    true,
		},
		{
			name:        "missing key right after create is reported",
			statusCode:  http.StatusOK,
			body:        `{"key-id":"test-key","encryption-error":"key with given key ID does not exist"}`,
			newResource: true,
			wantErr:     true,
		},
		{
			name:       "request error is reported and the key stays in state",
			statusCode: http.StatusForbidden,
			body:       `{"Code":"AccessDenied","Message":"access denied"}`,
			wantErr:    true,
		},
		{
			name:       "server error is reported and the key stays in state",
			statusCode: http.StatusInternalServerError,
			body:       `{"Code":"InternalError","Message":"internal error"}`,
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()

			adminClient, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
				Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
				Secure: false,
			})
			if err != nil {
				t.Fatalf("creating admin client: %v", err)
			}

			d := schema.TestResourceDataRaw(t, resourceMinioKMSKey().Schema, map[string]interface{}{
				"key_id": "test-key",
			})
			d.SetId("test-key")
			if tc.newResource {
				d.MarkNewResource()
			}

			diags := minioReadKMSKey(context.Background(), d, &S3MinioClient{S3Admin: adminClient})

			if tc.wantErr != diags.HasError() {
				t.Fatalf("wantErr %v, got diagnostics: %v", tc.wantErr, diags)
			}
			if gone := d.Id() == ""; gone != tc.wantGone {
				t.Fatalf("wantGone %v, id is %q", tc.wantGone, d.Id())
			}
		})
	}
}
