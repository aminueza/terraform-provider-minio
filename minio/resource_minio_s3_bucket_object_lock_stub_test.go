package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func newObjectLockBucketTestClient(t *testing.T, handler http.HandlerFunc) *minio.Client {
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

func TestMinioReadBucket_objectLocking(t *testing.T) {
	const (
		bucket       = "test-bucket"
		noObjectLock = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>ObjectLockConfigurationNotFoundError</Code><Message>Object Lock configuration does not exist for this bucket</Message></Error>`
		lockEnabled  = `<?xml version="1.0" encoding="UTF-8"?><ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ObjectLockEnabled>Enabled</ObjectLockEnabled></ObjectLockConfiguration>`
		noTagSet     = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchTagSet</Code><Message>There is no tag set associated with the resource.</Message></Error>`
	)

	cases := []struct {
		name        string
		initialLock bool
		skipTagging bool
		compatMode  bool
		lockStatus  int
		lockBody    string
		wantLock    bool
		wantErr     bool
	}{
		{
			name:        "enabled on server refreshes false state to true even when skipping tags",
			initialLock: false,
			skipTagging: true,
			lockStatus:  http.StatusOK,
			lockBody:    lockEnabled,
			wantLock:    true,
		},
		{
			name:        "ObjectLockConfigurationNotFoundError resets wrong true state to false",
			initialLock: true,
			lockStatus:  http.StatusNotFound,
			lockBody:    noObjectLock,
			wantLock:    false,
		},
		{
			name:        "501 NotImplemented keeps state under s3_compat_mode",
			initialLock: true,
			compatMode:  true,
			lockStatus:  http.StatusNotImplemented,
			lockBody:    `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NotImplemented</Code><Message>Not Implemented.</Message></Error>`,
			wantLock:    true,
		},
		{
			name:        "405 MethodNotAllowed keeps state under s3_compat_mode",
			initialLock: false,
			compatMode:  true,
			lockStatus:  http.StatusMethodNotAllowed,
			lockBody:    `<?xml version="1.0" encoding="UTF-8"?><Error><Code>MethodNotAllowed</Code><Message>Method Not Allowed.</Message></Error>`,
			wantLock:    false,
		},
		{
			name:        "501 NotImplemented keeps state even when s3_compat_mode is false",
			initialLock: false,
			compatMode:  false,
			lockStatus:  http.StatusNotImplemented,
			lockBody:    `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NotImplemented</Code><Message>Not Implemented.</Message></Error>`,
			wantLock:    false,
		},
		{
			name:        "403 AccessDenied keeps state value",
			initialLock: true,
			lockStatus:  http.StatusForbidden,
			lockBody:    `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied.</Message></Error>`,
			wantLock:    true,
		},
		{
			name:        "500 InternalError fails read",
			initialLock: false,
			lockStatus:  http.StatusInternalServerError,
			lockBody:    `<?xml version="1.0" encoding="UTF-8"?><Error><Code>InternalError</Code><Message>boom</Message></Error>`,
			wantErr:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s3Client := newObjectLockBucketTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusOK)
					return
				}
				if r.URL.Query().Has("object-lock") {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(tc.lockStatus)
					_, _ = fmt.Fprint(w, tc.lockBody)
					return
				}
				if r.URL.Query().Has("tagging") {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, noTagSet)
					return
				}
				w.WriteHeader(http.StatusOK)
			})

			d := schema.TestResourceDataRaw(t, resourceMinioBucket().Schema, map[string]interface{}{
				"bucket":         bucket,
				"object_locking": tc.initialLock,
			})
			d.SetId(bucket)

			diags := minioReadBucket(context.Background(), d, &S3MinioClient{
				S3Client:          s3Client,
				S3CompatMode:      tc.compatMode,
				SkipBucketTagging: tc.skipTagging,
			})
			if tc.wantErr {
				if !diags.HasError() {
					t.Fatal("expected read error, got none")
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("read failed: %v", diags)
			}
			if got := d.Get("object_locking").(bool); got != tc.wantLock {
				t.Fatalf("object_locking = %v, want %v", got, tc.wantLock)
			}
		})
	}
}

func TestCustomizeBucketDiff_objectLockingChangeRejected(t *testing.T) {
	res := resourceMinioBucket()
	ctx := context.Background()

	cases := []struct {
		name    string
		state   *terraform.InstanceState
		newLock bool
		wantErr string
	}{
		{
			name: "false to true rejected with must be recreated",
			state: &terraform.InstanceState{
				ID: "test-bucket",
				Attributes: map[string]string{
					"id":             "test-bucket",
					"bucket":         "test-bucket",
					"object_locking": "false",
				},
			},
			newLock: true,
			wantErr: "must be recreated",
		},
		{
			name: "true to false rejected with can't be disabled",
			state: &terraform.InstanceState{
				ID: "test-bucket",
				Attributes: map[string]string{
					"id":             "test-bucket",
					"bucket":         "test-bucket",
					"object_locking": "true",
				},
			},
			newLock: false,
			wantErr: "can't be disabled",
		},
		{
			name: "unchanged true allowed",
			state: &terraform.InstanceState{
				ID: "test-bucket",
				Attributes: map[string]string{
					"id":             "test-bucket",
					"bucket":         "test-bucket",
					"object_locking": "true",
				},
			},
			newLock: true,
		},
		{
			name:    "new bucket creation allowed",
			state:   nil,
			newLock: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := terraform.NewResourceConfigRaw(map[string]interface{}{
				"bucket":         "test-bucket",
				"object_locking": tc.newLock,
			})
			_, err := res.Diff(ctx, tc.state, cfg, &S3MinioClient{})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected diff error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestImportMinioS3Bucket_objectLockingErrorNotSwallowed(t *testing.T) {
	const notImplemented = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NotImplemented</Code><Message>Not Implemented.</Message></Error>`
	const noTagSet = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchTagSet</Code><Message>There is no tag set associated with the resource.</Message></Error>`

	s3Client := newObjectLockBucketTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Query().Has("object-lock") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>InternalError</Code><Message>boom</Message></Error>`)
			return
		}
		if r.URL.Query().Has("policy") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = fmt.Fprint(w, notImplemented)
			return
		}
		if r.URL.Query().Has("tagging") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, noTagSet)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	d := resourceMinioBucket().Data(nil)
	d.SetId("import-bucket")
	d.SetType("minio_s3_bucket")

	if _, err := resourceMinioS3BucketImportState(context.Background(), d, &S3MinioClient{S3Client: s3Client}); err == nil {
		t.Fatal("expected import to fail on 500 GetObjectLockConfig instead of silently setting false")
	} else if !strings.Contains(err.Error(), "object lock") {
		t.Fatalf("expected import error to name the object lock read failure, got %q", err.Error())
	}
}

func TestImportMinioS3Bucket_notImplementedPopulatesObjectLockingFalse(t *testing.T) {
	const notImplemented = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NotImplemented</Code><Message>Not Implemented.</Message></Error>`
	const noTagSet = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchTagSet</Code><Message>There is no tag set associated with the resource.</Message></Error>`

	for _, compatMode := range []bool{false, true} {
		t.Run(fmt.Sprintf("s3_compat_mode=%v", compatMode), func(t *testing.T) {
			s3Client := newObjectLockBucketTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusOK)
					return
				}
				if r.URL.Query().Has("object-lock") || r.URL.Query().Has("policy") {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusNotImplemented)
					_, _ = fmt.Fprint(w, notImplemented)
					return
				}
				if r.URL.Query().Has("tagging") {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, noTagSet)
					return
				}
				w.WriteHeader(http.StatusOK)
			})

			d := resourceMinioBucket().Data(nil)
			d.SetId("notimplemented-import-bucket")
			d.SetType("minio_s3_bucket")

			imported, err := resourceMinioS3BucketImportState(context.Background(), d, &S3MinioClient{
				S3Client:     s3Client,
				S3CompatMode: compatMode,
			})
			if err != nil {
				t.Fatalf("expected import on 501 object-lock backend to succeed, got: %v", err)
			}
			if len(imported) != 1 {
				t.Fatalf("expected 1 imported resource, got %d", len(imported))
			}
			if got, ok := imported[0].State().Attributes["object_locking"]; !ok || got != "false" {
				t.Fatalf("imported State().Attributes[\"object_locking\"] = (%q, %v), want (\"false\", true)", got, ok)
			}
		})
	}
}
