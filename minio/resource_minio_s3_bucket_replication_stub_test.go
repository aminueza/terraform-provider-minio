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

	if !diags.HasError() {
		t.Fatal("expected the failed removal of the stale remote target to be reported")
	}
	if removals == 0 {
		t.Fatal("expected the stale remote target to be removed")
	}
	if d.Id() != "source" {
		t.Fatalf("resource ID = %q, want %q so the written configuration stays in state", d.Id(), "source")
	}
}
