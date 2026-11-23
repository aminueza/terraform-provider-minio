package minio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestS3ObjectImportID(t *testing.T) {
	t.Run("valid ids set full bucket/object id", func(t *testing.T) {
		for _, tc := range []struct {
			id         string
			wantBucket string
			wantObject string
		}{
			{id: "my-bucket/plain.txt", wantBucket: "my-bucket", wantObject: "plain.txt"},
			{id: "my-bucket/dir/file.txt", wantBucket: "my-bucket", wantObject: "dir/file.txt"},
		} {
			t.Run(tc.id, func(t *testing.T) {
				d := resourceMinioObject().TestResourceData()
				d.SetId(tc.id)

				states, err := minioImportObject(context.Background(), d, nil)
				if err != nil {
					t.Fatalf("minioImportObject(%q) failed: %v", tc.id, err)
				}
				if len(states) != 1 {
					t.Fatalf("expected 1 state, got %d", len(states))
				}
				if states[0].Id() != tc.id {
					t.Fatalf("Id() = %q, want %q", states[0].Id(), tc.id)
				}
				if got := states[0].Get("bucket_name").(string); got != tc.wantBucket {
					t.Fatalf("bucket_name = %q, want %q", got, tc.wantBucket)
				}
				if got := states[0].Get("object_name").(string); got != tc.wantObject {
					t.Fatalf("object_name = %q, want %q", got, tc.wantObject)
				}
			})
		}
	})

	t.Run("malformed ids fail cleanly", func(t *testing.T) {
		for _, badID := range []string{"plain.txt", "/key", "bucket/", ""} {
			t.Run(fmt.Sprintf("%q", badID), func(t *testing.T) {
				d := resourceMinioObject().TestResourceData()
				d.SetId(badID)

				if _, err := minioImportObject(context.Background(), d, nil); err == nil {
					t.Fatalf("minioImportObject(%q) succeeded, want format error", badID)
				} else if !strings.Contains(err.Error(), "bucket_name/object_name") {
					t.Fatalf("error = %q, want bucket_name/object_name format hint", err.Error())
				}
			})
		}
	})
}

func TestS3ObjectCreateAndReadIDFormat(t *testing.T) {
	t.Run("create stores bucket/object id for plain and nested keys", func(t *testing.T) {
		for _, key := range []string{"plain.txt", "dir/file.txt"} {
			t.Run(key, func(t *testing.T) {
				client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
					q := r.URL.Query()
					switch {
					case r.Method == http.MethodPost && q.Has("uploads"):
						w.Header().Set("Content-Type", "application/xml")
						w.WriteHeader(http.StatusOK)
						_, _ = fmt.Fprintf(w, `<InitiateMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>test-bucket</Bucket><Key>%s</Key><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`, key)
					case r.Method == http.MethodPut && q.Has("partNumber"):
						_, _ = io.Copy(io.Discard, r.Body)
						w.Header().Set("ETag", `"5eb63bbbe01eeed093cb22bb8f5acdc3"`)
						w.WriteHeader(http.StatusOK)
					case r.Method == http.MethodPut:
						_, _ = io.Copy(io.Discard, r.Body)
						w.Header().Set("ETag", `"5eb63bbbe01eeed093cb22bb8f5acdc3"`)
						w.WriteHeader(http.StatusOK)
					case r.Method == http.MethodPost && q.Has("uploadId"):
						_, _ = io.Copy(io.Discard, r.Body)
						w.Header().Set("Content-Type", "application/xml")
						w.WriteHeader(http.StatusOK)
						_, _ = fmt.Fprintf(w, `<CompleteMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>test-bucket</Bucket><Key>%s</Key><ETag>"5eb63bbbe01eeed093cb22bb8f5acdc3"</ETag></CompleteMultipartUploadResult>`, key)
					case r.Method == http.MethodHead:
						w.Header().Set("ETag", `"5eb63bbbe01eeed093cb22bb8f5acdc3"`)
						w.Header().Set("Content-Type", "text/plain")
						w.Header().Set("Content-Length", "11")
						w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
						w.WriteHeader(http.StatusOK)
					case r.Method == http.MethodDelete && q.Has("uploadId"):
						w.WriteHeader(http.StatusNoContent)
					default:
						t.Errorf("unexpected HTTP %s %s", r.Method, r.URL.String())
						w.WriteHeader(http.StatusBadRequest)
					}
				})

				d := schema.TestResourceDataRaw(t, resourceMinioObject().Schema, map[string]interface{}{
					"bucket_name": "test-bucket",
					"object_name": key,
					"content":     "hello world",
				})
				if diags := minioCreateObject(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
					t.Fatalf("minioCreateObject(%q) failed: %v", key, diags)
				}

				wantID := "test-bucket/" + key
				if d.Id() != wantID {
					t.Fatalf("created Id() = %q, want %q", d.Id(), wantID)
				}
			})
		}
	})

	t.Run("read migrates legacy id from object_name to bucket/object", func(t *testing.T) {
		client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodHead {
				t.Errorf("unexpected HTTP %s %s", r.Method, r.URL.String())
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("ETag", `"5eb63bbbe01eeed093cb22bb8f5acdc3"`)
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", "11")
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.WriteHeader(http.StatusOK)
		})

		for _, key := range []string{"plain.txt", "dir/file.txt"} {
			t.Run(key, func(t *testing.T) {
				d := schema.TestResourceDataRaw(t, resourceMinioObject().Schema, map[string]interface{}{
					"bucket_name": "test-bucket",
					"object_name": key,
					"content":     "hello world",
				})
				d.SetId(key)

				if diags := minioReadObject(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
					t.Fatalf("minioReadObject(%q) failed: %v", key, diags)
				}
				wantID := "test-bucket/" + key
				if d.Id() != wantID {
					t.Fatalf("migrated Id() = %q, want %q", d.Id(), wantID)
				}
			})
		}
	})

	t.Run("read migrates legacy id even when s3_compat_mode absorbs 501", func(t *testing.T) {
		for _, key := range []string{"plain.txt", "dir/file.txt"} {
			t.Run(key, func(t *testing.T) {
				stub := newS3CompatStub(t, "test-bucket", http.StatusNotImplemented)
				d := schema.TestResourceDataRaw(t, resourceMinioObject().Schema, map[string]interface{}{
					"bucket_name": "test-bucket",
					"object_name": key,
					"content":     "compat-stub-body",
				})
				d.SetId(key)

				if diags := minioReadObject(context.Background(), d, stub.provider(true)); diags.HasError() {
					t.Fatalf("minioReadObject s3_compat_mode(%q) failed: %v", key, diags)
				}
				wantID := "test-bucket/" + key
				if d.Id() != wantID {
					t.Fatalf("s3_compat_mode Id() = %q, want %q", d.Id(), wantID)
				}
			})
		}
	})

	t.Run("read recovers both attributes from a full id", func(t *testing.T) {
		client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodHead {
				t.Errorf("unexpected HTTP %s %s", r.Method, r.URL.String())
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if r.URL.Path != "/test-bucket/dir/file.txt" {
				t.Errorf("stat on %q, want %q", r.URL.Path, "/test-bucket/dir/file.txt")
			}
			w.Header().Set("ETag", `"5eb63bbbe01eeed093cb22bb8f5acdc3"`)
			w.Header().Set("Content-Length", "11")
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.WriteHeader(http.StatusOK)
		})

		d := schema.TestResourceDataRaw(t, resourceMinioObject().Schema, map[string]interface{}{})
		d.SetId("test-bucket/dir/file.txt")

		if diags := minioReadObject(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
			t.Fatalf("minioReadObject failed: %v", diags)
		}
		if got := d.Get("bucket_name").(string); got != "test-bucket" {
			t.Errorf("bucket_name = %q, want %q", got, "test-bucket")
		}
		if got := d.Get("object_name").(string); got != "dir/file.txt" {
			t.Errorf("object_name = %q, want %q", got, "dir/file.txt")
		}
		if d.Id() != "test-bucket/dir/file.txt" {
			t.Errorf("Id() = %q, want it kept", d.Id())
		}
	})

	t.Run("read rejects a legacy id when no attributes are set", func(t *testing.T) {
		client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected request %s %s: the id is not parseable, so no call should be made", r.Method, r.URL.String())
			w.WriteHeader(http.StatusBadRequest)
		})

		d := schema.TestResourceDataRaw(t, resourceMinioObject().Schema, map[string]interface{}{})
		d.SetId("plain.txt")

		diags := minioReadObject(context.Background(), d, &S3MinioClient{S3Client: client})
		if !diags.HasError() {
			t.Fatal("expected an error for an id without the bucket prefix, got none")
		}
		if d.Id() != "plain.txt" {
			t.Errorf("Id() = %q, want it kept so the user can fix the state", d.Id())
		}
	})

	t.Run("read refuses to guess when only one attribute is set", func(t *testing.T) {
		client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected request %s %s: a legacy id must not be parsed against one attribute", r.Method, r.URL.String())
			w.WriteHeader(http.StatusBadRequest)
		})

		d := schema.TestResourceDataRaw(t, resourceMinioObject().Schema, map[string]interface{}{
			"bucket_name": "test-bucket",
		})
		d.SetId("dir/file.txt")

		diags := minioReadObject(context.Background(), d, &S3MinioClient{S3Client: client})
		if !diags.HasError() {
			t.Fatal("expected an error when only bucket_name is set, got none")
		}
		if d.Id() != "dir/file.txt" {
			t.Errorf("Id() = %q, want it kept so the user can fix the state", d.Id())
		}
	})
}
