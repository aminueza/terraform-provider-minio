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
	snapshotDeniedXML  = `<Error><Code>AccessDenied</Code><Message>Access Denied.</Message></Error>`
	snapshotNotImplXML = `<Error><Code>NotImplemented</Code><Message>The operation is not supported by this backend</Message></Error>`
	snapshotNoBucket   = `<Error><Code>NoSuchBucket</Code><Message>The specified bucket does not exist</Message></Error>`
)

type snapshotStubAnswer struct {
	status int
	body   string
}

var (
	snapshotDenied         = snapshotStubAnswer{http.StatusForbidden, snapshotDeniedXML}
	snapshotNotImplemented = snapshotStubAnswer{http.StatusNotImplemented, snapshotNotImplXML}
)

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

// The cross-protection workaround in minioPutBucketPolicy must capture the
// versioning configuration before SetBucketPolicy runs (issue #1389), so a loss
// caused by the write itself can be restored. A failed verification read counts
// as lost because re-applying the captured configuration is idempotent. A failed
// snapshot read aborts before the write, except when s3_compat_mode absorbs a
// backend that does not implement the API, and a failed restore is an error.
func TestBucketPolicyPutVersioningSnapshotRestore(t *testing.T) {
	cases := []struct {
		name             string
		dropOnWrite      bool
		snapshotFail     *snapshotStubAnswer
		compatMode       bool
		absorbed         bool
		restoreFail      bool
		noSuchBucketOnce bool
		secondRead       snapshotStubAnswer
		wantRestore      bool
	}{
		{"a write that drops versioning is restored", true, nil, false, false, false, false, snapshotStubAnswer{}, true},
		{"a transient NoSuchBucket on the snapshot read is retried", false, nil, false, false, false, true, snapshotStubAnswer{}, false},
		{"a failed snapshot read aborts before the write", false, &snapshotDenied, false, false, false, false, snapshotStubAnswer{}, false},
		{"a missing versioning API aborts with s3_compat_mode off", false, &snapshotNotImplemented, false, false, false, false, snapshotStubAnswer{}, false},
		{"a missing versioning API skips the check under s3_compat_mode", false, &snapshotNotImplemented, true, true, false, false, snapshotStubAnswer{}, false},
		{"a denied snapshot read aborts even under s3_compat_mode", false, &snapshotDenied, true, false, false, false, snapshotStubAnswer{}, false},
		{"a failed restore is reported", true, nil, false, false, true, false, snapshotStubAnswer{}, false},
		{"a failed verification read still restores the snapshot", false, nil, false, false, false, false, snapshotDenied, true},
		{"an empty verification read restores the snapshot", false, nil, false, false, false, false, snapshotStubAnswer{http.StatusOK, snapshotEmptyXML}, true},
		{"an intact verification read does not restore", false, nil, false, false, false, false, snapshotStubAnswer{http.StatusOK, snapshotEnabledXML}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			versioningReads := 0
			policyWrites := 0
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
					policyWrites++
					body, _ := io.ReadAll(r.Body)
					setPolicy = string(body)
				case r.Method == http.MethodGet && query.Has("policy"):
					_, _ = fmt.Fprint(w, setPolicy)
				case r.Method == http.MethodGet && query.Has("versioning"):
					versioningReads++
					switch {
					case tc.noSuchBucketOnce && versioningReads == 1:
						w.WriteHeader(http.StatusNotFound)
						_, _ = fmt.Fprint(w, snapshotNoBucket)
					case tc.noSuchBucketOnce:
						_, _ = fmt.Fprint(w, snapshotEnabledXML)
					case tc.snapshotFail != nil && versioningReads == 1:
						w.WriteHeader(tc.snapshotFail.status)
						_, _ = fmt.Fprint(w, tc.snapshotFail.body)
					case tc.dropOnWrite:
						if policyWrites > 0 {
							_, _ = fmt.Fprint(w, snapshotEmptyXML)
						} else {
							_, _ = fmt.Fprint(w, snapshotEnabledXML)
						}
					case versioningReads == 2:
						w.WriteHeader(tc.secondRead.status)
						_, _ = fmt.Fprint(w, tc.secondRead.body)
					default:
						_, _ = fmt.Fprint(w, snapshotEnabledXML)
					}
				case r.Method == http.MethodPut && query.Has("versioning"):
					body, _ := io.ReadAll(r.Body)
					restoreBodies = append(restoreBodies, string(body))
					if tc.restoreFail {
						w.WriteHeader(snapshotDenied.status)
						_, _ = fmt.Fprint(w, snapshotDenied.body)
					}
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
			diags := minioPutBucketPolicy(context.Background(), d, &S3MinioClient{S3Client: snapshotStubClient(t, srv.URL), S3CompatMode: tc.compatMode})

			if tc.snapshotFail != nil && !tc.absorbed {
				if !diags.HasError() {
					t.Fatalf("create succeeded, want an error aborting on the failed snapshot read")
				}
				if !strings.Contains(diags[0].Summary, "reading bucket versioning before setting policy") {
					t.Errorf("error summary = %q, want it to name the pre-write versioning read", diags[0].Summary)
				}
				if policyWrites != 0 {
					t.Errorf("policy writes = %d, want the write skipped when the snapshot read fails", policyWrites)
				}
				if d.Id() != "" {
					t.Errorf("a failed create left the resource %q in state", d.Id())
				}
				return
			}
			if tc.restoreFail {
				if !diags.HasError() {
					t.Fatalf("create succeeded, want an error reporting the failed restore")
				}
				if !strings.Contains(diags[0].Summary, "restoring bucket versioning after setting policy") {
					t.Errorf("error summary = %q, want it to name the failed restore", diags[0].Summary)
				}
				if d.Id() != "" {
					t.Errorf("a failed create left the resource %q in state", d.Id())
				}
				if len(restoreBodies) != 1 {
					t.Fatalf("restore attempts = %d, want exactly 1", len(restoreBodies))
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("create failed: %v", diags)
			}
			if policyWrites != 1 {
				t.Fatalf("policy writes = %d, want exactly 1", policyWrites)
			}
			if tc.snapshotFail != nil {
				if versioningReads != 1 {
					t.Fatalf("the stub served %d versioning reads, want only the skipped snapshot read", versioningReads)
				}
				if len(restoreBodies) != 0 {
					t.Fatalf("versioning restore writes = %d, want 0 when the check is skipped", len(restoreBodies))
				}
				return
			}
			wantVersioningReads := 2
			if tc.noSuchBucketOnce {
				wantVersioningReads = 3
			}
			if versioningReads != wantVersioningReads {
				t.Fatalf("the stub served %d versioning reads, want the snapshot and the verification read plus any retry", versioningReads)
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

// The cross-protection workaround in minioPutBucketVersioning must capture the
// bucket policy before SetBucketVersioning runs (issue #1389), for the same
// reasons as TestBucketPolicyPutVersioningSnapshotRestore.
func TestBucketVersioningPutPolicySnapshotRestore(t *testing.T) {
	cases := []struct {
		name             string
		dropOnWrite      bool
		snapshotFail     *snapshotStubAnswer
		compatMode       bool
		absorbed         bool
		restoreFail      bool
		noSuchBucketOnce bool
		secondRead       snapshotStubAnswer
		wantRestore      bool
	}{
		{"a write that drops the policy is restored", true, nil, false, false, false, false, snapshotStubAnswer{}, true},
		{"a transient NoSuchBucket on the snapshot read is retried", false, nil, false, false, false, true, snapshotStubAnswer{}, false},
		{"a failed snapshot read aborts before the write", false, &snapshotDenied, false, false, false, false, snapshotStubAnswer{}, false},
		{"a missing policy API aborts with s3_compat_mode off", false, &snapshotNotImplemented, false, false, false, false, snapshotStubAnswer{}, false},
		{"a missing policy API skips the check under s3_compat_mode", false, &snapshotNotImplemented, true, true, false, false, snapshotStubAnswer{}, false},
		{"a denied snapshot read aborts even under s3_compat_mode", false, &snapshotDenied, true, false, false, false, snapshotStubAnswer{}, false},
		{"a failed restore is reported", true, nil, false, false, true, false, snapshotStubAnswer{}, false},
		{"a failed verification read still restores the snapshot", false, nil, false, false, false, false, snapshotDenied, true},
		{"an empty verification read restores the snapshot", false, nil, false, false, false, false, snapshotStubAnswer{http.StatusOK, ""}, true},
		{"a cleared verification read restores the snapshot", false, nil, false, false, false, false, snapshotStubAnswer{http.StatusOK, "{}"}, true},
		{"an intact verification read does not restore", false, nil, false, false, false, false, snapshotStubAnswer{http.StatusOK, snapshotPolicyJSON}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			policyReads := 0
			versioningWrites := 0
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
					versioningWrites++
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodGet && query.Has("versioning"):
					_, _ = fmt.Fprint(w, snapshotEnabledXML)
				case r.Method == http.MethodGet && query.Has("policy"):
					policyReads++
					switch {
					case tc.noSuchBucketOnce && policyReads == 1:
						w.WriteHeader(http.StatusNotFound)
						_, _ = fmt.Fprint(w, snapshotNoBucket)
					case tc.noSuchBucketOnce:
						_, _ = fmt.Fprint(w, snapshotPolicyJSON)
					case tc.snapshotFail != nil && policyReads == 1:
						w.WriteHeader(tc.snapshotFail.status)
						_, _ = fmt.Fprint(w, tc.snapshotFail.body)
					case tc.dropOnWrite:
						if versioningWrites > 0 {
							w.WriteHeader(http.StatusOK)
						} else {
							_, _ = fmt.Fprint(w, snapshotPolicyJSON)
						}
					case policyReads == 2:
						w.WriteHeader(tc.secondRead.status)
						_, _ = fmt.Fprint(w, tc.secondRead.body)
					default:
						_, _ = fmt.Fprint(w, snapshotPolicyJSON)
					}
				case r.Method == http.MethodPut && query.Has("policy"):
					body, _ := io.ReadAll(r.Body)
					restoreBodies = append(restoreBodies, string(body))
					if tc.restoreFail {
						w.WriteHeader(snapshotDenied.status)
						_, _ = fmt.Fprint(w, snapshotDenied.body)
					}
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
			diags := minioPutBucketVersioning(context.Background(), d, &S3MinioClient{S3Client: snapshotStubClient(t, srv.URL), S3CompatMode: tc.compatMode})

			if tc.snapshotFail != nil && !tc.absorbed {
				if !diags.HasError() {
					t.Fatalf("create succeeded, want an error aborting on the failed snapshot read")
				}
				if !strings.Contains(diags[0].Summary, "reading bucket policy before setting versioning") {
					t.Errorf("error summary = %q, want it to name the pre-write policy read", diags[0].Summary)
				}
				if versioningWrites != 0 {
					t.Errorf("versioning writes = %d, want the write skipped when the snapshot read fails", versioningWrites)
				}
				if d.Id() != "" {
					t.Errorf("a failed create left the resource %q in state", d.Id())
				}
				return
			}
			if tc.restoreFail {
				if !diags.HasError() {
					t.Fatalf("create succeeded, want an error reporting the failed restore")
				}
				if !strings.Contains(diags[0].Summary, "restoring bucket policy after setting versioning") {
					t.Errorf("error summary = %q, want it to name the failed restore", diags[0].Summary)
				}
				if d.Id() != "" {
					t.Errorf("a failed create left the resource %q in state", d.Id())
				}
				if len(restoreBodies) != 1 {
					t.Fatalf("restore attempts = %d, want exactly 1", len(restoreBodies))
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("create failed: %v", diags)
			}
			if versioningWrites != 1 {
				t.Fatalf("versioning writes = %d, want exactly 1", versioningWrites)
			}
			if tc.snapshotFail != nil {
				if policyReads != 1 {
					t.Fatalf("the stub served %d policy reads, want only the skipped snapshot read", policyReads)
				}
				if len(restoreBodies) != 0 {
					t.Fatalf("policy restore writes = %d, want 0 when the check is skipped", len(restoreBodies))
				}
				return
			}
			wantPolicyReads := 2
			if tc.noSuchBucketOnce {
				wantPolicyReads = 3
			}
			if policyReads != wantPolicyReads {
				t.Fatalf("the stub served %d policy reads, want the snapshot and the verification read plus any retry", policyReads)
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
