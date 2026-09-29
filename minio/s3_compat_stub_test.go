package minio

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// s3CompatStub answers the S3 and MinIO admin APIs the way a backend that does
// not implement the feature under test does. It exists so the s3_compat_mode
// paths can be exercised end to end without a MinIO server: the provider code
// runs unchanged and only the transport is replaced.
//
// The error body matters. minio-go and madmin-go both build a typed
// minio.ErrorResponse / madmin.ErrorResponse out of the S3 XML document, which
// is what the provider matches on; a body-less 501 would leave Code set to the
// HTTP status line and would not reproduce a real backend.
type s3CompatStub struct {
	server *httptest.Server
	client *minio.Client
	admin  *madmin.AdminClient
	bucket string

	mu       sync.Mutex
	requests []string

	// objectLockEnabled answers the versioning and object-lock reads that
	// minio_s3_bucket_object_lock_configuration and minio_s3_bucket_retention
	// run before they write. Both check that the bucket has versioning and an
	// object lock before writing either, so with those reads failing the write
	// is never reached and the test only covers the pre-flight. Only the write
	// test sets it; the reads of those two resources are the feature itself, and
	// they need those endpoints to fail.
	objectLockEnabled bool
}

// s3CompatStubCode is the error code the stub reports for a given HTTP status.
func s3CompatStubCode(status int) string {
	switch status {
	case http.StatusMethodNotAllowed:
		return "MethodNotAllowed"
	default:
		return "NotImplemented"
	}
}

func newS3CompatStub(t *testing.T, bucket string, status int) *s3CompatStub {
	t.Helper()

	stub := &s3CompatStub{bucket: bucket}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		stub.requests = append(stub.requests, r.Method+" "+r.URL.RequestURI())
		stub.mu.Unlock()

		// A stub that answered 501 to everything would make every read fail
		// on its existence check instead of on the feature call. HEAD on the
		// bucket is the existence check, and it is also how StatObject reaches
		// the object, so the path decides which one gets a healthy answer.
		if r.Method == http.MethodHead && strings.TrimSuffix(r.URL.Path, "/") == "/"+stub.bucket {
			w.WriteHeader(http.StatusOK)
			return
		}
		if _, ok := r.URL.Query()["location"]; ok {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
			return
		}
		// The write that follows these two reads must still fail, so only the GET
		// is answered. A PUT to either endpoint is the feature under test.
		if stub.objectLockEnabled && r.Method == http.MethodGet {
			if _, ok := r.URL.Query()["versioning"]; ok {
				w.Header().Set("Content-Type", "application/xml")
				fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`)
				return
			}
			if _, ok := r.URL.Query()["object-lock"]; ok {
				w.Header().Set("Content-Type", "application/xml")
				fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ObjectLockEnabled>Enabled</ObjectLockEnabled></ObjectLockConfiguration>`)
				return
			}
		}

		w.Header().Set("Content-Type", "application/xml")
		// A HEAD response carries no body, so minio-go can only recover the
		// error code from these headers. MinIO and R2 both set them.
		w.Header().Set("x-minio-error-code", s3CompatStubCode(status))
		w.Header().Set("x-minio-error-desc", "The "+s3CompatStubCode(status)+" operation is not supported by this backend")
		w.WriteHeader(status)
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>The %s operation is not supported by this backend</Message><Resource>%s</Resource><RequestId>stub</RequestId><HostId>stub</HostId></Error>`,
			s3CompatStubCode(status), s3CompatStubCode(status), r.URL.Path)
	}))
	t.Cleanup(stub.server.Close)

	host := strings.TrimPrefix(stub.server.URL, "http://")

	client, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
		Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("building S3 stub client: %v", err)
	}
	stub.client = client

	admin, err := madmin.NewWithOptions(host, &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("building admin stub client: %v", err)
	}
	stub.admin = admin

	return stub
}

// provider returns the meta value the resource CRUD functions expect.
func (s *s3CompatStub) provider(compat bool) *S3MinioClient {
	return &S3MinioClient{
		S3Client:     s.client,
		S3Admin:      s.admin,
		S3CompatMode: compat,
	}
}

// sawRequest reports whether the stub was asked for the given request, matched
// on the path and query the SDK produces. Without it a read that never reached
// the feature call would satisfy the state assertions vacuously.
func (s *s3CompatStub) sawRequest(want string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, got := range s.requests {
		if strings.Contains(got, want) {
			return true
		}
	}
	return false
}

func (s *s3CompatStub) requestLog() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}
