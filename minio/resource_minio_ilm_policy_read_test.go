package minio

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestMinioReadILMPolicyErrors(t *testing.T) {
	expirationRule := map[string]interface{}{"id": "expire", "status": "Enabled", "expiration": "30d"}
	abortOnlyRule := map[string]interface{}{
		"id":                                "abort",
		"status":                            "Enabled",
		"abort_incomplete_multipart_upload": []interface{}{map[string]interface{}{"days_after_initiation": "7d"}},
	}

	cases := []struct {
		name        string
		rule        map[string]interface{}
		newResource bool
		status      int
		code        string
		wantErr     bool
		wantGone    bool
	}{
		{name: "lifecycle removed outside Terraform", rule: expirationRule, status: http.StatusNotFound, code: "NoSuchLifecycleConfiguration", wantGone: true},
		{name: "bucket deleted", rule: expirationRule, status: http.StatusNotFound, code: "NoSuchBucket", wantGone: true},
		{name: "access denied is reported", rule: expirationRule, status: http.StatusForbidden, code: "AccessDenied", wantErr: true},
		{name: "missing lifecycle right after create is reported", rule: expirationRule, newResource: true, status: http.StatusNotFound, code: "NoSuchLifecycleConfiguration", wantErr: true},
		{name: "abort-only rules the server does not store stay in state", rule: abortOnlyRule, status: http.StatusNotFound, code: "NoSuchLifecycleConfiguration"},
		{name: "abort-only rules stay in state right after create", rule: abortOnlyRule, newResource: true, status: http.StatusNotFound, code: "NoSuchLifecycleConfiguration"},
		{name: "abort-only rules on a deleted bucket are removed", rule: abortOnlyRule, status: http.StatusNotFound, code: "NoSuchBucket", wantGone: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>error</Message></Error>`, tc.code)
			})

			d := schema.TestResourceDataRaw(t, resourceMinioILMPolicy().Schema, map[string]interface{}{
				"bucket": "test-bucket",
				"rule":   []interface{}{tc.rule},
			})
			d.SetId("test-bucket")
			if tc.newResource {
				d.MarkNewResource()
			}

			diags := minioReadILMPolicy(context.Background(), d, &S3MinioClient{S3Client: client})

			if diags.HasError() != tc.wantErr {
				t.Fatalf("wantErr %v, got %v", tc.wantErr, diags)
			}
			if gone := d.Id() == ""; gone != tc.wantGone {
				t.Fatalf("wantGone %v, id is %q", tc.wantGone, d.Id())
			}
		})
	}
}
