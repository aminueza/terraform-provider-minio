package minio

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestObjectTagsMissingObjectOrBucket(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   string
		gone   bool
	}{
		{name: "object deleted", status: http.StatusNotFound, code: "NoSuchKey", gone: true},
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
			meta := &S3MinioClient{S3Client: client}

			ops := []struct {
				name string
				run  func(context.Context, *schema.ResourceData, interface{}) diag.Diagnostics
			}{
				{"read", minioReadObjectTags},
				{"delete", minioDeleteObjectTags},
			}

			for _, op := range ops {
				d := resourceMinioObjectTags().TestResourceData()
				d.SetId("test-bucket/path/to/object")
				if err := d.Set("bucket", "test-bucket"); err != nil {
					t.Fatal(err)
				}
				if err := d.Set("key", "path/to/object"); err != nil {
					t.Fatal(err)
				}

				diags := op.run(context.Background(), d, meta)

				if tc.gone {
					if diags.HasError() {
						t.Fatalf("%s: expected no error, got %v", op.name, diags)
					}
					if d.Id() != "" {
						t.Fatalf("%s: expected the resource to be removed from state, id is %q", op.name, d.Id())
					}
					continue
				}
				if !diags.HasError() {
					t.Fatalf("%s: expected an error for %s", op.name, tc.code)
				}
				if d.Id() == "" {
					t.Fatalf("%s: expected the resource to stay in state on %s", op.name, tc.code)
				}
			}
		})
	}
}
