package minio

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Read stores these timestamps as the server returns them, in UTC, so a
// configuration written with another offset must not plan a change.
func TestRFC3339AttributesIgnoreOffsetOnlyDiffs(t *testing.T) {
	resources := []struct {
		name      string
		resource  func() *schema.Resource
		attribute string
		state     map[string]string
		config    map[string]interface{}
	}{
		{
			name:      "minio_s3_object_retention",
			resource:  resourceMinioObjectRetention,
			attribute: "retain_until_date",
			state:     map[string]string{"id": "test-bucket/object", "bucket": "test-bucket", "key": "object", "mode": "GOVERNANCE", "governance_bypass": "false"},
			config:    map[string]interface{}{"bucket": "test-bucket", "key": "object", "mode": "GOVERNANCE"},
		},
		{
			name:      "minio_s3_object",
			resource:  resourceMinioObject,
			attribute: "expires",
			state:     map[string]string{"id": "object", "bucket_name": "test-bucket", "object_name": "object", "content": "hello"},
			config:    map[string]interface{}{"bucket_name": "test-bucket", "object_name": "object", "content": "hello"},
		},
	}

	cases := []struct {
		name     string
		state    string
		config   string
		wantDiff bool
	}{
		{name: "same instant with another offset", state: "2030-01-01T03:00:00Z", config: "2030-01-01T00:00:00-03:00", wantDiff: false},
		{name: "same value", state: "2030-01-01T03:00:00Z", config: "2030-01-01T03:00:00Z", wantDiff: false},
		{name: "different instant", state: "2030-01-01T03:00:00Z", config: "2030-01-01T03:00:00-03:00", wantDiff: true},
	}

	for _, res := range resources {
		for _, tc := range cases {
			t.Run(res.name+"/"+tc.name, func(t *testing.T) {
				attributes := map[string]string{res.attribute: tc.state}
				for k, v := range res.state {
					attributes[k] = v
				}
				raw := map[string]interface{}{res.attribute: tc.config}
				for k, v := range res.config {
					raw[k] = v
				}

				diff, err := res.resource().Diff(context.Background(),
					&terraform.InstanceState{ID: res.state["id"], Attributes: attributes},
					terraform.NewResourceConfigRaw(raw), nil)
				if err != nil {
					t.Fatalf("diff: %v", err)
				}

				gotDiff := false
				if diff != nil {
					_, gotDiff = diff.GetAttribute(res.attribute)
				}
				if gotDiff != tc.wantDiff {
					t.Fatalf("%s in diff = %v, want %v (diff: %#v)", res.attribute, gotDiff, tc.wantDiff, diff)
				}
			})
		}
	}
}
