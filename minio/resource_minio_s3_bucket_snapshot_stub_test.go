package minio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	snapshotPolicyJSON = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::test-bucket/*"]}]}`
	snapshotEnabledXML = `<VersioningConfiguration><Status>Enabled</Status></VersioningConfiguration>`
	snapshotEmptyXML   = `<VersioningConfiguration></VersioningConfiguration>`
)

type snapshotStubAnswer struct {
	status int
	body   string
}

func snapshotStubClient(t *testing.T, url string) *minio.Client {
	t.Helper()
	client, err := minio.New(strings.TrimPrefix(url, "http://"), &minio.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
		Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("creating S3 client: %v", err)
	}
	return client
}

// The cross-protection workaround in minioPutBucketPolicy must re-apply the captured
// versioning configuration also when the verification read itself fails: re-applying
// the same configuration is idempotent, and skipping the restore on a read error
// silently drops the protection the workaround exists for.
func TestBucketPolicyPutRestoresVersioningSnapshotWhenVerifyReadFails(t *testing.T) {
	denied := snapshotStubAnswer{http.StatusForbidden, `<Error><Code>AccessDenied</Code><Message>Access Denied.</Message></Error>`}

	cases := []struct {
		name        string
		secondRead  snapshotStubAnswer
		wantRestore bool
	}{
		{"a failed verification read still restores the snapshot", denied, true},
		{"an empty verification read restores the snapshot", snapshotStubAnswer{http.StatusOK, snapshotEmptyXML}, true},
		{"an intact verification read does not restore", snapshotStubAnswer{http.StatusOK, snapshotEnabledXML}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			versioningReads := 0
			var restoreBodies []string
			setPolicy := ""

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				query := r.URL.Query()
				switch {
				case r.Method == http.MethodGet && query.Has("location"):
					_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint>us-east-1</LocationConstraint>`)
				case r.Method == http.MethodPut && query.Has("policy"):
					body, _ := io.ReadAll(r.Body)
					setPolicy = string(body)
				case r.Method == http.MethodGet && query.Has("policy"):
					_, _ = fmt.Fprint(w, setPolicy)
				case r.Method == http.MethodGet && query.Has("versioning"):
					versioningReads++
					if versioningReads == 2 {
						w.WriteHeader(tc.secondRead.status)
						_, _ = fmt.Fprint(w, tc.secondRead.body)
					} else {
						_, _ = fmt.Fprint(w, snapshotEnabledXML)
					}
				case r.Method == http.MethodPut && query.Has("versioning"):
					body, _ := io.ReadAll(r.Body)
					restoreBodies = append(restoreBodies, string(body))
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer srv.Close()

			d := schema.TestResourceDataRaw(t, resourceMinioBucketPolicy().Schema, map[string]interface{}{
				"bucket": "test-bucket",
				"policy": snapshotPolicyJSON,
			})
			diags := minioPutBucketPolicy(context.Background(), d, &S3MinioClient{S3Client: snapshotStubClient(t, srv.URL)})
			if diags.HasError() {
				t.Fatalf("create failed: %v", diags)
			}

			if versioningReads < 2 {
				t.Fatalf("the stub served %d versioning reads, want both the snapshot and the verification read", versioningReads)
			}
			if tc.wantRestore {
				if len(restoreBodies) != 1 {
					t.Fatalf("versioning restore writes = %d, want exactly 1", len(restoreBodies))
				}
				if !strings.Contains(restoreBodies[0], "<Status>Enabled</Status>") {
					t.Errorf("restore body = %q, want it to re-apply the captured Enabled configuration", restoreBodies[0])
				}
			} else if len(restoreBodies) != 0 {
				t.Fatalf("versioning restore writes = %d, want 0 when versioning survived", len(restoreBodies))
			}
		})
	}
}

// The cross-protection workaround in minioPutBucketVersioning must re-apply the
// captured bucket policy also when the verification read itself fails, for the same
// reason as TestBucketPolicyPutRestoresVersioningSnapshotWhenVerifyReadFails.
func TestBucketVersioningPutRestoresPolicySnapshotWhenVerifyReadFails(t *testing.T) {
	denied := snapshotStubAnswer{http.StatusForbidden, `<Error><Code>AccessDenied</Code><Message>Access Denied.</Message></Error>`}

	cases := []struct {
		name        string
		secondRead  snapshotStubAnswer
		wantRestore bool
	}{
		{"a failed verification read still restores the snapshot", denied, true},
		{"an empty verification read restores the snapshot", snapshotStubAnswer{http.StatusOK, ""}, true},
		{"a cleared verification read restores the snapshot", snapshotStubAnswer{http.StatusOK, "{}"}, true},
		{"an intact verification read does not restore", snapshotStubAnswer{http.StatusOK, snapshotPolicyJSON}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			policyReads := 0
			var restoreBodies []string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				query := r.URL.Query()
				switch {
				case r.Method == http.MethodGet && query.Has("location"):
					_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint>us-east-1</LocationConstraint>`)
				case r.Method == http.MethodHead:
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodPut && query.Has("versioning"):
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodGet && query.Has("versioning"):
					_, _ = fmt.Fprint(w, snapshotEnabledXML)
				case r.Method == http.MethodGet && query.Has("policy"):
					policyReads++
					if policyReads == 2 {
						w.WriteHeader(tc.secondRead.status)
						_, _ = fmt.Fprint(w, tc.secondRead.body)
					} else {
						_, _ = fmt.Fprint(w, snapshotPolicyJSON)
					}
				case r.Method == http.MethodPut && query.Has("policy"):
					body, _ := io.ReadAll(r.Body)
					restoreBodies = append(restoreBodies, string(body))
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer srv.Close()

			d := schema.TestResourceDataRaw(t, resourceMinioBucketVersioning().Schema, map[string]interface{}{
				"bucket": "test-bucket",
				"versioning_configuration": []interface{}{
					map[string]interface{}{"status": "Enabled"},
				},
			})
			diags := minioPutBucketVersioning(context.Background(), d, &S3MinioClient{S3Client: snapshotStubClient(t, srv.URL)})
			if diags.HasError() {
				t.Fatalf("create failed: %v", diags)
			}

			if policyReads < 2 {
				t.Fatalf("the stub served %d policy reads, want both the snapshot and the verification read", policyReads)
			}
			if tc.wantRestore {
				if len(restoreBodies) != 1 {
					t.Fatalf("policy restore writes = %d, want exactly 1", len(restoreBodies))
				}
				if !strings.Contains(restoreBodies[0], `"Version":"2012-10-17"`) {
					t.Errorf("restore body = %q, want it to re-apply the captured policy", restoreBodies[0])
				}
			} else if len(restoreBodies) != 0 {
				t.Fatalf("policy restore writes = %d, want 0 when the policy survived", len(restoreBodies))
			}
		})
	}
}
