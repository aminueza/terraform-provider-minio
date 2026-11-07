package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// fakeReplicationServer keeps the remote targets and the replication
// configuration of the bucket "source" and enforces the MinIO rules that
// matter here: one remote target per endpoint, no removal of a remote target
// the configuration still references, and deleting the configuration deletes
// every remote target of the bucket.
type fakeReplicationServer struct {
	t *testing.T

	mu       sync.Mutex
	targets  []madmin.BucketTarget
	config   string
	newARN   string
	requests []string
	// failCreate makes the creation of a new remote target fail.
	failCreate bool
}

func (f *fakeReplicationServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case strings.HasSuffix(r.URL.Path, "/list-remote-targets"):
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.targets)
	case strings.HasSuffix(r.URL.Path, "/set-remote-target"):
		target, err := f.decodeTarget(r)
		if err != nil {
			f.t.Errorf("decoding the remote target: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("update") == "true" {
			f.requests = append(f.requests, "update-remote-target")
			for i := range f.targets {
				if f.targets[i].Arn == target.Arn && r.URL.Query().Get("sync") == "true" {
					f.targets[i].ReplicationSync = target.ReplicationSync
				}
			}
			_, _ = fmt.Fprintf(w, "%q", target.Arn)
			return
		}
		f.requests = append(f.requests, "set-remote-target")
		for _, existing := range f.targets {
			// Like getRemoteARN in MinIO: the same endpoint, target bucket and
			// access key return the existing ARN and change nothing.
			if existing.Endpoint == target.Endpoint && existing.TargetBucket == target.TargetBucket && existing.Credentials.AccessKey == target.Credentials.AccessKey {
				_, _ = fmt.Fprintf(w, "%q", existing.Arn)
				return
			}
			if existing.Endpoint == target.Endpoint {
				w.WriteHeader(http.StatusConflict)
				_, _ = fmt.Fprint(w, `{"Code":"XMinioAdminBucketRemoteAlreadyExists","Message":"The remote target already exists"}`)
				return
			}
		}
		if f.failCreate {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"Code":"XMinioAdminRemoteTargetConnectionErr","Message":"Remote service connection error"}`)
			return
		}
		target.Arn = f.newARN
		f.targets = append(f.targets, target)
		_, _ = fmt.Fprintf(w, "%q", f.newARN)
	case strings.HasSuffix(r.URL.Path, "/remove-remote-target"):
		arn := r.URL.Query().Get("arn")
		f.requests = append(f.requests, "remove-remote-target "+arn)
		if strings.Contains(f.config, arn) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"Code":"XMinioAdminRemoteTargetDeleteNotAllowed","Message":"Remote target still referenced by the replication configuration"}`)
			return
		}
		for i := range f.targets {
			if f.targets[i].Arn == arn {
				f.targets = append(f.targets[:i], f.targets[i+1:]...)
				break
			}
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.TrimSuffix(r.URL.Path, "/") == "/source" && r.URL.Query().Has("replication"):
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, f.config)
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			f.config = string(body)
			f.requests = append(f.requests, "put-replication "+f.ruleARNs())
		case http.MethodDelete:
			f.config = `<ReplicationConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></ReplicationConfiguration>`
			f.targets = nil
			f.requests = append(f.requests, "delete-replication")
		}
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func (f *fakeReplicationServer) decodeTarget(r *http.Request) (madmin.BucketTarget, error) {
	var target madmin.BucketTarget
	data, err := madmin.DecryptData("secretkey", r.Body)
	if err != nil {
		return target, err
	}
	err = json.Unmarshal(data, &target)
	return target, err
}

func (f *fakeReplicationServer) ruleARNs() string {
	var arns []string
	for _, target := range append(f.targets, madmin.BucketTarget{Arn: f.newARN}) {
		if target.Arn != "" && strings.Contains(f.config, target.Arn) && !slices.Contains(arns, target.Arn) {
			arns = append(arns, target.Arn)
		}
	}
	return fmt.Sprint(arns)
}

func newFakeReplicationClient(t *testing.T, f *fakeReplicationServer) *S3MinioClient {
	t.Helper()
	srv := httptest.NewServer(f)
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

func replicationRuleData(t *testing.T, targetBucket string, synchronous bool) *schema.ResourceData {
	t.Helper()
	return schema.TestResourceDataRaw(t, resourceMinioBucketReplication().Schema, map[string]interface{}{
		"bucket": "source",
		"rule": []interface{}{
			map[string]interface{}{
				"priority": 1,
				"target": []interface{}{
					map[string]interface{}{
						"bucket":      targetBucket,
						"host":        "minio-b:9000",
						"access_key":  "replication",
						"secret_key":  "replication-secret",
						"synchronous": synchronous,
					},
				},
			},
		},
	})
}

func replicationConfigFor(arn string) string {
	return fmt.Sprintf(`<ReplicationConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ID>rule-1</ID><Status>Enabled</Status><Priority>1</Priority><DeleteMarkerReplication><Status>Disabled</Status></DeleteMarkerReplication><DeleteReplication><Status>Disabled</Status></DeleteReplication><Destination><Bucket>%s</Bucket></Destination><Filter><Prefix></Prefix></Filter></Rule></ReplicationConfiguration>`, arn)
}

func TestPutBucketReplicationChangesTargetBucketOnSameEndpoint(t *testing.T) {
	const oldARN = "arn:minio:replication::old:bucket-1"
	const newARN = "arn:minio:replication::new:bucket-2"

	f := &fakeReplicationServer{
		t: t,
		targets: []madmin.BucketTarget{{
			SourceBucket: "source",
			Endpoint:     "minio-b:9000",
			TargetBucket: "bucket-1",
			Arn:          oldARN,
			Type:         madmin.ReplicationService,
			Credentials:  &madmin.Credentials{AccessKey: "replication", SecretKey: "replication-secret"},
		}},
		config: replicationConfigFor(oldARN),
		newARN: newARN,
	}

	d := replicationRuleData(t, "bucket-2", false)
	if diags := minioPutBucketReplication(context.Background(), d, newFakeReplicationClient(t, f)); diags.HasError() {
		t.Fatalf("expected no error, got %v (requests: %q)", diags, f.requests)
	}

	want := []string{
		"delete-replication",
		"set-remote-target",
		"put-replication [" + newARN + "]",
	}
	if fmt.Sprint(f.requests) != fmt.Sprint(want) {
		t.Fatalf("requests = %q, want %q", f.requests, want)
	}
	if len(f.targets) != 1 || f.targets[0].Arn != newARN {
		t.Fatalf("remote targets = %+v, want only %q", f.targets, newARN)
	}
}

func TestPutBucketReplicationUpdatesSynchronousModeInPlace(t *testing.T) {
	const arn = "arn:minio:replication::current:bucket-1"

	f := &fakeReplicationServer{
		t: t,
		targets: []madmin.BucketTarget{{
			SourceBucket: "source",
			Endpoint:     "minio-b:9000",
			TargetBucket: "bucket-1",
			Arn:          arn,
			Type:         madmin.ReplicationService,
			Credentials:  &madmin.Credentials{AccessKey: "replication", SecretKey: "replication-secret"},
		}},
		config: replicationConfigFor(arn),
	}

	d := replicationRuleData(t, "bucket-1", true)
	if diags := minioPutBucketReplication(context.Background(), d, newFakeReplicationClient(t, f)); diags.HasError() {
		t.Fatalf("expected no error, got %v (requests: %q)", diags, f.requests)
	}

	want := []string{"update-remote-target", "put-replication [" + arn + "]"}
	if fmt.Sprint(f.requests) != fmt.Sprint(want) {
		t.Fatalf("requests = %q, want %q", f.requests, want)
	}
	if len(f.targets) != 1 || !f.targets[0].ReplicationSync {
		t.Fatalf("remote targets = %+v, want %q switched to synchronous replication", f.targets, arn)
	}
}

func TestPutBucketReplicationRetargetFailureSaysReplicationStopped(t *testing.T) {
	const oldARN = "arn:minio:replication::old:bucket-1"

	f := &fakeReplicationServer{
		t: t,
		targets: []madmin.BucketTarget{{
			SourceBucket: "source",
			Endpoint:     "minio-b:9000",
			TargetBucket: "bucket-1",
			Arn:          oldARN,
			Type:         madmin.ReplicationService,
			Credentials:  &madmin.Credentials{AccessKey: "replication", SecretKey: "replication-secret"},
		}},
		config:     replicationConfigFor(oldARN),
		failCreate: true,
	}

	d := replicationRuleData(t, "bucket-2", false)
	diags := minioPutBucketReplication(context.Background(), d, newFakeReplicationClient(t, f))
	if !diags.HasError() {
		t.Fatalf("expected an error, got none (requests: %q)", f.requests)
	}
	if msg := fmt.Sprint(diags); !strings.Contains(msg, "replication to minio-b:9000 is stopped until an apply succeeds") {
		t.Fatalf("error does not say replication stopped: %s", msg)
	}
}
