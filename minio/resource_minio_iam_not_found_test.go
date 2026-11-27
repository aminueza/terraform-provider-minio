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

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

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

// The report in #1484 was about create: the existence check failed, so the
// create aborted although the object was absent. These drive Create against a
// stub that answers like RustFS and check that the write goes through.
func TestIAMCreateProceedsOnRustfsNotFound(t *testing.T) {
	const policyJSON = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::original/*"]}]}`

	rustfsMissing := func(message string) string {
		return `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchResource</Code><Message>` +
			message + `</Message></Error>`
	}
	writeRustfsMissingBody := func(w http.ResponseWriter, message string) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, rustfsMissing(message))
	}

	t.Run("policy", func(t *testing.T) {
		name := "tfacc-pol-" + acctest.RandString(8)
		var mu sync.Mutex
		stored := ""
		var createCalls int

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			switch {
			case strings.HasSuffix(r.URL.Path, "/info-canned-policy"):
				if stored == "" {
					writeRustfsMissingBody(w, "policy does not exist")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"PolicyName":%q,"Policy":%s}`, name, stored)
			case strings.HasSuffix(r.URL.Path, "/add-canned-policy"):
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("reading add-canned-policy body: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				stored = string(body)
				createCalls++
				w.WriteHeader(http.StatusOK)
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}
		})

		state, diags := applyCreateWithStub(t, resourceMinioIAMPolicy(), map[string]interface{}{
			"name":   name,
			"policy": policyJSON,
		}, &S3MinioClient{S3Admin: admin})

		if diags.HasError() {
			t.Fatalf("create failed: %v", diags)
		}
		if createCalls != 1 {
			t.Errorf("AddCannedPolicy calls = %d, want 1", createCalls)
		}
		if state == nil || state.ID != name {
			t.Errorf("state = %#v, want the created policy", state)
		}
	})

	t.Run("group", func(t *testing.T) {
		name := "tfacc-grp-" + acctest.RandString(8)
		var mu sync.Mutex
		exists := false
		var createCalls int

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			switch {
			case strings.HasSuffix(r.URL.Path, "/group"):
				if !exists {
					writeRustfsMissingBody(w, fmt.Sprintf("group '%s' does not exist", name))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"name":%q,"status":"enabled","members":[]}`, name)
			case strings.HasSuffix(r.URL.Path, "/update-group-members"):
				exists = true
				createCalls++
				w.WriteHeader(http.StatusOK)
			case strings.HasSuffix(r.URL.Path, "/set-group-status"):
				w.WriteHeader(http.StatusOK)
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}
		})

		state, diags := applyCreateWithStub(t, resourceMinioIAMGroup(), map[string]interface{}{
			"name": name,
		}, &S3MinioClient{S3Admin: admin})

		if diags.HasError() {
			t.Fatalf("create failed: %v", diags)
		}
		if createCalls != 1 {
			t.Errorf("UpdateGroupMembers calls = %d, want 1", createCalls)
		}
		if state == nil || state.ID != name {
			t.Errorf("state = %#v, want the created group", state)
		}
	})

	t.Run("user", func(t *testing.T) {
		name := "tfacc-" + acctest.RandString(8)
		var mu sync.Mutex
		exists := false
		var createCalls int

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			switch {
			case strings.HasSuffix(r.URL.Path, "/user-info"):
				if !exists {
					writeRustfsMissingBody(w, "user does not exist")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"status":"enabled","policyName":"","memberOf":[]}`)
			case strings.HasSuffix(r.URL.Path, "/add-user"):
				exists = true
				createCalls++
				w.WriteHeader(http.StatusOK)
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}
		})

		ctx := context.Background()
		meta := &S3MinioClient{S3Admin: admin}
		res := resourceMinioIAMUser()

		diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(map[string]interface{}{
			"name":              name,
			"secret_wo":         "wo-secret-value",
			"secret_wo_version": 1,
		}), meta)
		if err != nil {
			t.Fatalf("planning create: %v", err)
		}
		// Resource.Diff does not carry the config through, and write-only values
		// are only readable from it.
		diff.RawConfig = cty.ObjectVal(map[string]cty.Value{
			"id":                cty.NullVal(cty.String),
			"name":              cty.StringVal(name),
			"force_destroy":     cty.False,
			"disable_user":      cty.False,
			"update_secret":     cty.False,
			"status":            cty.NullVal(cty.String),
			"secret":            cty.NullVal(cty.String),
			"secret_wo":         cty.StringVal("wo-secret-value"),
			"secret_wo_version": cty.NumberIntVal(1),
			"tags":              cty.NullVal(cty.Map(cty.String)),
		})

		state, diags := res.Apply(ctx, nil, diff, meta)

		if diags.HasError() {
			t.Fatalf("create failed: %v", diags)
		}
		if createCalls != 1 {
			t.Errorf("AddUser calls = %d, want 1", createCalls)
		}
		if state == nil || state.ID != name {
			t.Errorf("state = %#v, want the created user", state)
		}
	})
}
