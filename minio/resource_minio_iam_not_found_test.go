package minio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// The bodies are what the servers send for a missing user, group or policy:
// RustFS answers every one with an S3 XML error coded NoSuchResource (see
// iam_error_to_s3_error in rustfs/src/admin/handlers/iam_error.rs), MinIO with
// a JSON error coded per kind. See issue #1484.
func TestIAMNotFoundClassification(t *testing.T) {
	const rustfsMissing = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchResource</Code><Message>user does not exist</Message></Error>`

	cases := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantMissing bool
	}{
		{
			name:        "RustFS NoSuchResource is a missing object",
			status:      http.StatusNotFound,
			contentType: "application/xml",
			body:        rustfsMissing,
			wantMissing: true,
		},
		{
			name:        "the MinIO code for the kind is a missing object",
			status:      http.StatusNotFound,
			contentType: "application/json",
			body:        `{"Code":"XMinioAdminNoSuch%s","Message":"The specified object does not exist."}`,
			wantMissing: true,
		},
		{
			name:        "a 404 with another code is not",
			status:      http.StatusNotFound,
			contentType: "application/xml",
			body:        `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NotImplemented</Code><Message>not here</Message></Error>`,
		},
		{
			name:        "a 404 page from something in front of the server is not",
			status:      http.StatusNotFound,
			contentType: "text/html",
			body:        `<html><body>404 Not Found</body></html>`,
		},
		{
			name:        "access denied is not",
			status:      http.StatusForbidden,
			contentType: "application/xml",
			body:        `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied.</Message></Error>`,
		},
	}

	lookups := []struct {
		kind     string
		call     func(*madmin.AdminClient) error
		isAbsent func(error) bool
	}{
		{"User", func(a *madmin.AdminClient) error {
			_, err := a.GetUserInfo(context.Background(), "alice")
			return err
		}, isNoSuchUserError},
		{"Group", func(a *madmin.AdminClient) error {
			_, err := a.GetGroupDescription(context.Background(), "devs")
			return err
		}, isMissingGroupError},
		{"Policy", func(a *madmin.AdminClient) error {
			_, err := a.InfoCannedPolicy(context.Background(), "readonly-x")
			return err
		}, isNoSuchPolicyError},
	}

	for _, tc := range cases {
		for _, lk := range lookups {
			t.Run(tc.name+"/"+lk.kind, func(t *testing.T) {
				body := strings.Replace(tc.body, "%s", lk.kind, 1)
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", tc.contentType)
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, body)
				}))
				defer srv.Close()
				admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
					Creds: credentials.NewStaticV4("accesskey", "secretkey", ""),
				})
				if err != nil {
					t.Fatalf("creating admin client: %v", err)
				}

				err = lk.call(admin)
				if err == nil {
					t.Fatal("lookup succeeded, want the server's error")
				}
				if got := lk.isAbsent(err); got != tc.wantMissing {
					t.Errorf("missing = %v for %v, want %v", got, err, tc.wantMissing)
				}
			})
		}
	}
}
