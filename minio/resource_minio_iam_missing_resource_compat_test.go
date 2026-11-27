package minio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
)

func diagsString(diags diag.Diagnostics) string {
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.Summary+" "+d.Detail)
	}
	return strings.Join(msgs, "\n")
}

// A server that only mimics the MinIO admin API answers a missing IAM object
// with its own code. RustFS answers 404 NoSuchResource for users, groups and
// policies (rustfs/src/admin/handlers/iam_error.rs), where MinIO answers
// XMinioAdminNoSuchUser, XMinioAdminNoSuchGroup and XMinioAdminNoSuchPolicy.
func TestMadminMissingChecksAcceptCompatibleNotFound(t *testing.T) {
	checks := []struct {
		name  string
		fn    func(error) bool
		code  string
		other string
	}{
		{"policy", isNoSuchPolicyError, "XMinioAdminNoSuchPolicy", "XMinioAdminNoSuchUser"},
		{"group", isNoSuchGroupError, "XMinioAdminNoSuchGroup", "XMinioAdminNoSuchPolicy"},
		{"user", isNoSuchUserError, "XMinioAdminNoSuchUser", "XMinioAdminNoSuchGroup"},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			missing := []madmin.ErrorResponse{
				{Code: check.code, StatusCode: http.StatusNotFound},
				{Code: "NoSuchResource", StatusCode: http.StatusNotFound, Message: "the specified resource doesn't exist"},
			}
			for _, errResp := range missing {
				if !check.fn(errResp) {
					t.Errorf("error code %q must count as missing", errResp.Code)
				}
				wrapped := fmt.Errorf("looking up the %s: %w", check.name, errResp)
				if !check.fn(wrapped) {
					t.Errorf("wrapped error code %q must count as missing", errResp.Code)
				}
			}

			// A 404 names which object is missing only through its code, so the
			// code of another object type must not read as "this object is
			// missing", and neither must a code this provider does not know.
			notMissing := []madmin.ErrorResponse{
				{Code: check.other, StatusCode: http.StatusNotFound},
				{Code: "NotFound", StatusCode: http.StatusNotFound},
				{Code: "NoSuchResource", StatusCode: http.StatusInternalServerError},
				{Code: "AccessDenied", StatusCode: http.StatusForbidden},
				{Code: "InvalidArgument", StatusCode: http.StatusBadRequest},
				{Code: "InternalError", StatusCode: http.StatusInternalServerError},
			}
			for _, errResp := range notMissing {
				if check.fn(errResp) {
					t.Errorf("error code %q status %d must not count as missing", errResp.Code, errResp.StatusCode)
				}
			}

			if check.fn(nil) {
				t.Error("a nil error must not count as missing")
			}
		})
	}
}

func TestResourceMinioIAMPolicyCreateProceedsOnCompatibleNotFound(t *testing.T) {
	const policyJSON = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::original/*"]}]}`
	name := "tfacc-pol-" + acctest.RandString(8)

	var mu sync.Mutex
	stored := ""
	var createCalls int

	admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info-canned-policy"):
			if stored == "" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"Code":"NoSuchResource","Message":"policy does not exist"}`)
				return
			}
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
		t.Fatalf("expected Create to proceed when the server answers NoSuchResource, got: %s", diagsString(diags))
	}
	if createCalls != 1 {
		t.Fatalf("expected exactly 1 AddCannedPolicy call, got %d", createCalls)
	}
	if state == nil || state.ID != name {
		t.Fatalf("expected the created policy in state, got %#v", state)
	}
}

func TestResourceMinioIAMGroupCreateProceedsOnCompatibleNotFound(t *testing.T) {
	name := "tfacc-grp-" + acctest.RandString(8)

	var mu sync.Mutex
	exists := false
	var createCalls int

	admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/group"):
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"Code":"NoSuchResource","Message":"group '%s' does not exist"}`, name)
				return
			}
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
		t.Fatalf("expected Create to proceed when the server answers NoSuchResource, got: %s", diagsString(diags))
	}
	if createCalls != 1 {
		t.Fatalf("expected exactly 1 UpdateGroupMembers call, got %d", createCalls)
	}
	if state == nil || state.ID != name {
		t.Fatalf("expected the created group in state, got %#v", state)
	}
}

func TestResourceMinioIAMUserCreateProceedsOnCompatibleNotFound(t *testing.T) {
	name := "tfacc-" + acctest.RandString(8)

	var mu sync.Mutex
	exists := false
	var createCalls int

	admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/user-info"):
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"Code":"NoSuchResource","Message":"user does not exist"}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"status":"enabled","policyName":"","memberOf":[]}`)
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
		t.Fatalf("expected Create to proceed when the server answers NoSuchResource, got: %s", diagsString(diags))
	}
	if createCalls != 1 {
		t.Fatalf("expected exactly 1 AddUser call, got %d", createCalls)
	}
	if state == nil || state.ID != name {
		t.Fatalf("expected the created user in state, got %#v", state)
	}
}

// A read has to drop the resource from state when the object was deleted outside
// Terraform, on a server that answers NoSuchResource as well as on MinIO. The
// group message is the one RustFS sends, which the string match this provider
// used before does not recognize.
func TestIAMReadsDropResourceOnCompatibleNotFound(t *testing.T) {
	user := "tfacc-" + acctest.RandString(8)
	group := "tfacc-grp-" + acctest.RandString(8)

	tests := []struct {
		name string
		res  *schema.Resource
		id   string
		path string
		body string
	}{
		{
			name: "user",
			res:  resourceMinioIAMUser(),
			id:   user,
			path: "/user-info",
			body: `{"Code":"NoSuchResource","Message":"user does not exist"}`,
		},
		{
			name: "group",
			res:  resourceMinioIAMGroup(),
			id:   group,
			path: "/group",
			body: fmt.Sprintf(`{"Code":"NoSuchResource","Message":"group '%s' does not exist"}`, group),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := iamMissingTargetAdminStub(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, tc.path) {
					t.Errorf("unexpected path %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, tc.body)
			})

			d := tc.res.TestResourceData()
			d.SetId(tc.id)

			diags := tc.res.ReadContext(context.Background(), d, &S3MinioClient{S3Admin: client})
			if diags.HasError() {
				t.Fatalf("expected the read to drop the resource, got %v", diags)
			}
			if d.Id() != "" {
				t.Fatalf("expected empty id, got %q", d.Id())
			}
		})
	}
}
