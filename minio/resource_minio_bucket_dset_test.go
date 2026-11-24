package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7/pkg/replication"
)

func TestPreserveBucketTagsStateErrorPropagation(t *testing.T) {
	if err := preserveBucketTagsState(nil); err != nil {
		t.Fatalf("preserveBucketTagsState(nil) error = %v, want nil", err)
	}

	dTags := schema.TestResourceDataRaw(t, resourceMinioBucketTags().Schema, map[string]interface{}{
		"bucket": "tag-bucket",
		"tags":   map[string]interface{}{"env": "prod"},
	})
	if err := preserveBucketTagsState(dTags); err != nil {
		t.Fatalf("preserveBucketTagsState(valid) error = %v", err)
	}
	got := dTags.Get("tags").(map[string]interface{})
	if got["env"] != "prod" {
		t.Fatalf("tags[env] = %v, want %q", got["env"], "prod")
	}

	dEmpty := schema.TestResourceDataRaw(t, resourceMinioBucketTags().Schema, map[string]interface{}{
		"bucket": "tag-bucket",
	})
	if err := preserveBucketTagsState(dEmpty); err != nil {
		t.Fatalf("preserveBucketTagsState(empty) error = %v", err)
	}
	if len(dEmpty.Get("tags").(map[string]interface{})) != 0 {
		t.Fatalf("expected empty tags map, got %v", dEmpty.Get("tags"))
	}

	// terraform-plugin-sdk panics on d.Set schema errors when TF_ACC is set.
	t.Setenv("TF_ACC", "")
	dNoTags := resourceMinioKMSKey().TestResourceData()
	if err := preserveBucketTagsState(dNoTags); err == nil {
		t.Fatal("expected error when schema lacks tags attribute, got nil")
	}
}

func TestBucketQuotaReadChecksStatePopulation(t *testing.T) {
	meta, _ := newDataSourceS3AndAdminStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/get-bucket-quota") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"size":2048,"quotatype":"hard"}`)
			return
		}
		http.NotFound(w, r)
	})

	d := schema.TestResourceDataRaw(t, resourceMinioBucketQuota().Schema, map[string]interface{}{
		"bucket": "quota-bucket",
		"quota":  1024,
		"type":   "hard",
	})
	d.SetId("quota-bucket")

	if diags := minioReadBucketQuota(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("minioReadBucketQuota failed: %v", diags)
	}
	if got := d.Get("bucket").(string); got != "quota-bucket" {
		t.Errorf("bucket = %q, want %q", got, "quota-bucket")
	}
	if got := d.Get("quota").(int); got != 2048 {
		t.Errorf("quota = %d, want %d", got, 2048)
	}
	if got := d.Get("type").(string); got != "hard" {
		t.Errorf("type = %q, want %q", got, "hard")
	}
}

func TestBucketUpdateQuotaAndReadDeleteChecksStatePopulation(t *testing.T) {
	var quotaSize int64 = 1024
	meta, _ := newDataSourceS3AndAdminStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/set-bucket-quota"):
			quotaSize = 4096
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/get-bucket-quota"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"size":%d,"quotatype":"hard"}`, quotaSize)
		case r.URL.Query().Has("object-lock"):
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>ObjectLockConfigurationNotFoundError</Code><Message>Object Lock configuration does not exist</Message></Error>`)
		case r.URL.Query().Has("tagging") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `<Tagging><TagSet><Tag><Key>team</Key><Value>core</Value></Tag></TagSet></Tagging>`)
		case r.Method == http.MethodGet && (r.URL.Query().Has("list-type") || r.URL.Query().Has("max-keys")):
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><Name>app-bucket</Name><IsTruncated>false</IsTruncated></ListBucketResult>`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
	meta.MaxRetries = 1
	meta.RetryDelayMs = 1

	ctx := context.Background()
	res := resourceMinioBucket()

	initialState := &terraform.InstanceState{
		ID: "app-bucket",
		Attributes: map[string]string{
			"id":     "app-bucket",
			"bucket": "app-bucket",
			"acl":    "private",
			"quota":  "1024",
		},
	}
	diff, err := res.Diff(ctx, initialState, terraform.NewResourceConfigRaw(map[string]interface{}{
		"bucket": "app-bucket",
		"acl":    "private",
		"quota":  4096,
	}), meta)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	updatedState, diags := res.Apply(ctx, initialState, diff, meta)
	if diags.HasError() {
		t.Fatalf("Apply update failed: %v", diags)
	}
	if got := updatedState.Attributes["quota"]; got != "4096" {
		t.Errorf("updated quota = %q, want %q", got, "4096")
	}
	if got := updatedState.Attributes["arn"]; got != "arn:aws:s3:::app-bucket" {
		t.Errorf("arn = %q, want %q", got, "arn:aws:s3:::app-bucket")
	}
	if got := updatedState.Attributes["tags.team"]; got != "core" {
		t.Errorf("tags.team = %q, want %q", got, "core")
	}
	if got := updatedState.Attributes["bucket_domain_name"]; got == "" {
		t.Error("expected non-empty bucket_domain_name after update/read")
	}

	dDelete := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"bucket":             "app-bucket",
		"bucket_domain_name": updatedState.Attributes["bucket_domain_name"],
	})
	dDelete.SetId("app-bucket")
	if diags := minioDeleteBucket(ctx, dDelete, meta); diags.HasError() {
		t.Fatalf("minioDeleteBucket failed: %v", diags)
	}
	if got := dDelete.Get("bucket_domain_name").(string); got != "" {
		t.Errorf("bucket_domain_name after delete = %q, want empty", got)
	}
}

func TestBucketTagsReadChecksStatePopulation(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		initial  map[string]interface{}
		wantTags map[string]string
	}{
		{
			name:     "NoSuchTagSet clears tags",
			status:   http.StatusNotFound,
			body:     `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchTagSet</Code><Message>The TagSet does not exist</Message></Error>`,
			initial:  map[string]interface{}{"old": "val"},
			wantTags: map[string]string{},
		},
		{
			name:     "NotImplemented preserves existing tags",
			status:   http.StatusNotImplemented,
			body:     `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NotImplemented</Code><Message>Not implemented</Message></Error>`,
			initial:  map[string]interface{}{"keep": "yes"},
			wantTags: map[string]string{"keep": "yes"},
		},
		{
			name:     "200 returns remote tags",
			status:   http.StatusOK,
			body:     `<Tagging><TagSet><Tag><Key>env</Key><Value>staging</Value></Tag></TagSet></Tagging>`,
			initial:  map[string]interface{}{},
			wantTags: map[string]string{"env": "staging"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta, _ := newDataSourceS3AndAdminStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})
			meta.MaxRetries = 1
			meta.RetryDelayMs = 1

			d := schema.TestResourceDataRaw(t, resourceMinioBucketTags().Schema, map[string]interface{}{
				"bucket": "tags-bucket",
				"tags":   tc.initial,
			})
			d.SetId("tags-bucket")

			if diags := minioReadBucketTags(context.Background(), d, meta); diags.HasError() {
				t.Fatalf("minioReadBucketTags failed: %v", diags)
			}
			if got := d.Get("bucket").(string); got != "tags-bucket" {
				t.Errorf("bucket = %q, want %q", got, "tags-bucket")
			}
			gotTags := d.Get("tags").(map[string]interface{})
			if len(gotTags) != len(tc.wantTags) {
				t.Fatalf("tags = %v, want %v", gotTags, tc.wantTags)
			}
			for k, wantV := range tc.wantTags {
				if gotTags[k] != wantV {
					t.Errorf("tags[%q] = %v, want %q", k, gotTags[k], wantV)
				}
			}
		})
	}
}

func TestBucketReplicationResyncSetsLastResyncID(t *testing.T) {
	meta := newReplicationStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Has("replication"):
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `<ReplicationConfiguration><Role>arn:aws:iam::123:role/rep</Role><Rule><ID>r1</ID><Status>Enabled</Status><Destination><Bucket>arn:minio:replication::id:dest-bucket</Bucket></Destination></Rule></ReplicationConfiguration>`)
		case r.Method == http.MethodPut && r.URL.Query().Has("replication-reset"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			payload := replication.ResyncTargetsInfo{
				Targets: []replication.ResyncTarget{
					{
						Arn:     "arn:minio:replication::id:dest-bucket",
						ResetID: "resync-42",
					},
				},
			}
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				t.Fatalf("encoding resync response: %v", err)
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	d := schema.TestResourceDataRaw(t, resourceMinioBucketReplication().Schema, map[string]interface{}{
		"bucket": "src-bucket",
	})
	d.SetId("src-bucket")

	cfg := &S3MinioBucketReplication{
		MinioBucket: "src-bucket",
		MinioClient: meta.S3Client,
		MinioAdmin:  meta.S3Admin,
	}
	if diags := triggerBucketReplicationResync(context.Background(), cfg, d); diags.HasError() {
		t.Fatalf("triggerBucketReplicationResync failed: %v", diags)
	}
	if got := d.Get("last_resync_id").(string); got != "resync-42" {
		t.Errorf("last_resync_id = %q, want %q", got, "resync-42")
	}
}
