package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestResourceMinioIAMUserCreateFailsWhenUserExists(t *testing.T) {
	name := "tfacc-" + acctest.RandString(8)

	var mu sync.Mutex
	var writeCalls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/user-info") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"status":"enabled","policyName":"readwrite","memberOf":[]}`)
			return
		}
		mu.Lock()
		writeCalls = append(writeCalls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}

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
	// Resource.Diff does not carry the config through, so set the raw config the
	// way the plugin protocol does; write-only values are only readable from it.
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

	if !diags.HasError() {
		t.Fatalf("expected Create to fail for existing user %q, but it succeeded", name)
	}
	wantMsg := fmt.Sprintf("minio_iam_user %q already exists", name)
	if !strings.Contains(diags[0].Summary, wantMsg) {
		t.Errorf("expected error containing %q, got %q", wantMsg, diags[0].Summary)
	}
	if !strings.Contains(diags[0].Summary, "terraform import minio_iam_user.<name> "+name) {
		t.Errorf("expected error to contain the import hint, got %q", diags[0].Summary)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(writeCalls) != 0 {
		t.Errorf("expected no write call for existing user, got %v", writeCalls)
	}
	if state != nil && state.ID != "" {
		t.Errorf("expected empty id after failed create, got %q", state.ID)
	}
}

// iamUserCreateStubApply runs Create against a stub admin server whose
// /user-info endpoint is answered by userInfo, and returns the diagnostics
// plus every non-read call the provider made.
func iamUserCreateStubApply(t *testing.T, name string, userInfo http.HandlerFunc) (diag.Diagnostics, []string) {
	t.Helper()

	var mu sync.Mutex
	var writeCalls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/user-info") {
			userInfo(w, r)
			return
		}
		mu.Lock()
		writeCalls = append(writeCalls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}

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

	_, diags := res.Apply(ctx, nil, diff, meta)

	mu.Lock()
	defer mu.Unlock()
	return diags, append([]string(nil), writeCalls...)
}

func TestResourceMinioIAMUserCreateFailsWhenUserExistsDisabled(t *testing.T) {
	name := "tfacc-" + acctest.RandString(8)

	diags, writeCalls := iamUserCreateStubApply(t, name, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"disabled","policyName":"","memberOf":[]}`)
	})

	if !diags.HasError() {
		t.Fatalf("expected Create to fail for disabled existing user %q, but it succeeded", name)
	}
	wantMsg := fmt.Sprintf("minio_iam_user %q already exists", name)
	if !strings.Contains(diags[0].Summary, wantMsg) {
		t.Errorf("expected error containing %q, got %q", wantMsg, diags[0].Summary)
	}
	if len(writeCalls) != 0 {
		t.Errorf("expected no write call for disabled existing user, got %v", writeCalls)
	}
}

func TestResourceMinioIAMUserCreateFailsWhenLookupErrors(t *testing.T) {
	name := "tfacc-" + acctest.RandString(8)

	diags, writeCalls := iamUserCreateStubApply(t, name, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"Code":"InternalError","Message":"simulated lookup failure"}`)
	})

	if !diags.HasError() {
		t.Fatalf("expected Create to fail when the existence lookup errors for %q, but it succeeded", name)
	}
	if !strings.Contains(diags[0].Summary, "checking for existing IAM user") {
		t.Errorf("expected error containing %q, got %q", "checking for existing IAM user", diags[0].Summary)
	}
	if strings.Contains(diags[0].Summary, "wo-secret-value") {
		t.Errorf("the write-only secret must not appear in the error, got %q", diags[0].Summary)
	}
	if len(writeCalls) != 0 {
		t.Errorf("expected no write call when the lookup errors, got %v", writeCalls)
	}
}
