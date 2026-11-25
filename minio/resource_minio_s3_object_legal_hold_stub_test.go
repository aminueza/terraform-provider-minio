package minio

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TestObjectLegalHoldReadPinsUnlockedBucketTrigger pins which server error each
// legal hold read branch actually reacts to, measured against MinIO
// RELEASE.2025-09-07T16-13-09Z: a bucket created without object locking answers
// GetObjectLegalHold with InvalidRequest "Bucket is missing
// ObjectLockConfiguration", which objectLockReadGone maps to dropping the
// resource; an object in a locked bucket that never had a hold answers
// NoSuchObjectLockConfiguration, which records status OFF; any other
// InvalidRequest is a real read failure, not a reason to drop state. See issue #1413.
func TestObjectLegalHoldReadPinsUnlockedBucketTrigger(t *testing.T) {
	cases := []struct {
		name       string
		errCode    string
		errMessage string
		wantGone   bool
		wantErr    bool
	}{
		{"a bucket without object lock arrives as InvalidRequest and drops the resource", "InvalidRequest", "Bucket is missing ObjectLockConfiguration", true, false},
		{"an object without a hold arrives as NoSuchObjectLockConfiguration and records OFF", "NoSuchObjectLockConfiguration", "The specified object does not have a ObjectLock configuration", false, false},
		{"an unrelated InvalidRequest fails the read instead of dropping the resource", "InvalidRequest", "Requested range not satisfiable", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newObjectLockBucketTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Query().Has("legal-hold") {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = fmt.Fprintf(w, `<Error><Code>%s</Code><Message>%s</Message></Error>`, tc.errCode, tc.errMessage)
					return
				}
				t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
				w.WriteHeader(http.StatusInternalServerError)
			})

			d := schema.TestResourceDataRaw(t, resourceMinioObjectLegalHold().Schema, map[string]interface{}{
				"bucket": "probe-bucket",
				"key":    "probe-key.txt",
			})
			d.SetId("probe-bucket/probe-key.txt")
			diags := minioReadObjectLegalHold(context.Background(), d, &S3MinioClient{S3Client: client})

			if tc.wantErr {
				if !diags.HasError() {
					t.Fatalf("read succeeded, want an error for an unrelated %s", tc.errCode)
				}
				if d.Id() == "" {
					t.Error("resource dropped from state, want it kept with a failed read")
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("read failed: %v", diags)
			}

			if tc.wantGone {
				if d.Id() != "" {
					t.Errorf("resource id = %q, want the resource dropped from state", d.Id())
				}
				return
			}
			if d.Id() == "" {
				t.Fatal("resource dropped from state, want it kept with status OFF")
			}
			if got := d.Get("status").(string); got != "OFF" {
				t.Errorf("status = %q, want OFF", got)
			}
		})
	}
}
