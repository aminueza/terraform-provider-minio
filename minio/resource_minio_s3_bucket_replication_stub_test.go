package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestPutBucketReplicationKeepsIDWhenStaleTargetRemovalFails(t *testing.T) {
	const staleARN = "arn:minio:replication::stale:old-bucket"
	const newARN = "arn:minio:replication::new:target-bucket"

	var removals int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/list-remote-targets"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `[{"sourcebucket":"source","endpoint":"minio-old:9000","targetbucket":"old-bucket","arn":%q,"type":"replication"}]`, staleARN)
		case strings.HasSuffix(r.URL.Path, "/set-remote-target"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, "%q", newARN)
		case strings.HasSuffix(r.URL.Path, "/remove-remote-target"):
			removals++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"Code":"InternalError","Message":"We encountered an internal error, please try again.","Resource":"/minio/admin/v3/remove-remote-target"}`)
		case strings.TrimSuffix(r.URL.Path, "/") == "/source" && r.URL.Query().Has("replication") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><ReplicationConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></ReplicationConfiguration>`)
		case strings.TrimSuffix(r.URL.Path, "/") == "/source" && r.URL.Query().Has("replication") && r.Method == http.MethodPut:
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	creds := credentials.NewStaticV4("accesskey", "secretkey", "")
	s3Client, err := minio.New(host, &minio.Options{Creds: creds, Secure: false, Region: "us-east-1"})
	if err != nil {
		t.Fatalf("creating the S3 client: %v", err)
	}
	admin, err := madmin.NewWithOptions(host, &madmin.Options{Creds: creds, Secure: false})
	if err != nil {
		t.Fatalf("creating the admin client: %v", err)
	}

	d := schema.TestResourceDataRaw(t, resourceMinioBucketReplication().Schema, map[string]interface{}{
		"bucket": "source",
		"rule": []interface{}{
			map[string]interface{}{
				"priority": 1,
				"target": []interface{}{
					map[string]interface{}{
						"bucket":     "target-bucket",
						"host":       "minio-new:9000",
						"access_key": "replication",
						"secret_key": "replication-secret",
					},
				},
			},
		},
	})

	diags := minioPutBucketReplication(context.Background(), d, &S3MinioClient{S3Client: s3Client, S3Admin: admin})

	if diags.HasError() {
		t.Fatalf("expected no error, got %v", diags)
	}
	if len(diags) != 1 || diags[0].Severity != diag.Warning {
		t.Fatalf("expected a single warning for the failed removal of the stale remote target, got %v", diags)
	}
	if removals == 0 {
		t.Fatal("expected the stale remote target to be removed")
	}
	if d.Id() != "source" {
		t.Fatalf("resource ID = %q, want %q so the written configuration stays in state", d.Id(), "source")
	}
}

func newReplicationStubClient(t *testing.T, handler http.HandlerFunc) *S3MinioClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	creds := credentials.NewStaticV4("accesskey", "secretkey", "")
	s3Client, err := minio.New(host, &minio.Options{Creds: creds, Secure: false, Region: "us-east-1"})
	if err != nil {
		t.Fatalf("creating the S3 client: %v", err)
	}
	admin, err := madmin.NewWithOptions(host, &madmin.Options{Creds: creds, Secure: false})
	if err != nil {
		t.Fatalf("creating the admin client: %v", err)
	}
	return &S3MinioClient{S3Client: s3Client, S3Admin: admin}
}

func TestBucketReplicationRuleSchemaRequired(t *testing.T) {
	res := resourceMinioBucketReplication()
	ruleSchema := res.Schema["rule"]
	if !ruleSchema.Required {
		t.Fatal(`Schema["rule"].Required = false, want true`)
	}
	if ruleSchema.MinItems != 1 {
		t.Fatalf(`Schema["rule"].MinItems = %d, want 1`, ruleSchema.MinItems)
	}
	if diags := res.Validate(terraform.NewResourceConfigRaw(map[string]interface{}{"bucket": "source"})); !diags.HasError() {
		t.Fatal("expected config validation error when rule block is omitted, got nil")
	}
}

func TestPutBucketReplicationErrorsWithoutRules(t *testing.T) {
	meta := newReplicationStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request %s %s when no rules are configured", r.Method, r.URL.String())
	})

	d := schema.TestResourceDataRaw(t, resourceMinioBucketReplication().Schema, map[string]interface{}{
		"bucket": "source",
	})
	diags := minioPutBucketReplication(context.Background(), d, meta)
	if !diags.HasError() {
		t.Fatal("expected minioPutBucketReplication to return an error when no rules are provided")
	}
	if d.Id() != "" {
		t.Fatalf("resource ID = %q, want empty on error", d.Id())
	}
}

func TestReadAndDeleteBucketReplicationDropMissingBucket(t *testing.T) {
	meta := newReplicationStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchBucket</Code><Message>The specified bucket does not exist</Message><Resource>/source</Resource></Error>`)
	})

	dRead := schema.TestResourceDataRaw(t, resourceMinioBucketReplication().Schema, map[string]interface{}{
		"bucket": "source",
	})
	dRead.SetId("source")
	if diags := minioReadBucketReplication(context.Background(), dRead, meta); diags.HasError() {
		t.Fatalf("minioReadBucketReplication returned unexpected error on NoSuchBucket: %v", diags)
	}
	if dRead.Id() != "" {
		t.Fatalf("minioReadBucketReplication kept ID %q, want empty after NoSuchBucket", dRead.Id())
	}

	dDel := schema.TestResourceDataRaw(t, resourceMinioBucketReplication().Schema, map[string]interface{}{
		"bucket": "source",
	})
	dDel.SetId("source")
	if diags := minioDeleteBucketReplication(context.Background(), dDel, meta); diags.HasError() {
		t.Fatalf("minioDeleteBucketReplication returned unexpected error on NoSuchBucket: %v", diags)
	}
}

func TestReadAndDeleteBucketReplicationDropMissingBucketAdminAPI(t *testing.T) {
	// The S3 replication read succeeds while the admin API reports the bucket
	// gone with a non-standard message: only the typed madmin classification
	// in isNoSuchBucketError can catch this.
	meta := newReplicationStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/list-remote-targets"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"Code":"BucketNotFound","Message":"bucket missing","Resource":"/minio/admin/v3/list-remote-targets"}`)
		case strings.TrimSuffix(r.URL.Path, "/") == "/source" && r.URL.Query().Has("replication") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><ReplicationConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></ReplicationConfiguration>`)
		case strings.TrimSuffix(r.URL.Path, "/") == "/source" && r.URL.Query().Has("replication") && r.Method == http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case strings.TrimSuffix(r.URL.Path, "/") == "/source" && r.URL.Query().Has("replication") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotImplemented)
		}
	})

	dRead := schema.TestResourceDataRaw(t, resourceMinioBucketReplication().Schema, map[string]interface{}{
		"bucket": "source",
	})
	dRead.SetId("source")
	if diags := minioReadBucketReplication(context.Background(), dRead, meta); diags.HasError() {
		t.Fatalf("minioReadBucketReplication returned unexpected error on admin BucketNotFound: %v", diags)
	}
	if dRead.Id() != "" {
		t.Fatalf("minioReadBucketReplication kept ID %q, want empty after admin BucketNotFound", dRead.Id())
	}

	dDel := schema.TestResourceDataRaw(t, resourceMinioBucketReplication().Schema, map[string]interface{}{
		"bucket": "source",
	})
	dDel.SetId("source")
	if diags := minioDeleteBucketReplication(context.Background(), dDel, meta); diags.HasError() {
		t.Fatalf("minioDeleteBucketReplication returned unexpected error on admin BucketNotFound: %v", diags)
	}
}

func TestReadBucketReplicationAdminAccessDeniedKeepsResource(t *testing.T) {
	meta := newReplicationStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/list-remote-targets"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"Code":"AccessDenied","Message":"Access Denied.","Resource":"/minio/admin/v3/list-remote-targets"}`)
		case strings.TrimSuffix(r.URL.Path, "/") == "/source" && r.URL.Query().Has("replication") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><ReplicationConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></ReplicationConfiguration>`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotImplemented)
		}
	})

	d := schema.TestResourceDataRaw(t, resourceMinioBucketReplication().Schema, map[string]interface{}{
		"bucket": "source",
	})
	d.SetId("source")
	if diags := minioReadBucketReplication(context.Background(), d, meta); !diags.HasError() {
		t.Fatal("expected an error when the admin API denies access, got none")
	}
	if d.Id() != "source" {
		t.Fatalf("resource ID = %q, want %q kept after a non-NoSuchBucket admin error", d.Id(), "source")
	}
}

func TestReadBucketReplicationDropAfterCreateStillErrors(t *testing.T) {
	// The read that runs in the same apply as create must report the error
	// instead of dropping the resource that was just created.
	meta := newReplicationStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchBucket</Code><Message>The specified bucket does not exist</Message><Resource>/source</Resource></Error>`)
	})

	d := resourceMinioBucketReplication().TestResourceData()
	d.MarkNewResource()
	d.SetId("source")
	if err := d.Set("bucket", "source"); err != nil {
		t.Fatal(err)
	}
	if diags := minioReadBucketReplication(context.Background(), d, meta); !diags.HasError() {
		t.Fatal("expected an error when the bucket is missing right after create, got none")
	}
	if d.Id() != "source" {
		t.Fatalf("resource ID = %q, want %q kept when the read runs right after create", d.Id(), "source")
	}
}
