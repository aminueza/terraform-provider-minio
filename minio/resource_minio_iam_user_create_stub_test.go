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
