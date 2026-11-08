package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func groupPolicyAdminStub(t *testing.T, infoStatus int, infoBody string, removed *bool) *madmin.AdminClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info-canned-policy"):
			w.WriteHeader(infoStatus)
			_, _ = fmt.Fprint(w, infoBody)
		case strings.HasSuffix(r.URL.Path, "/remove-canned-policy"):
			*removed = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}
	return client
}

func TestIAMGroupPolicyReadAndDeleteErrors(t *testing.T) {
	const policyBody = `{"PolicyName":"test-policy","Policy":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bucket/*"]}]}}`

	cases := []struct {
		name          string
		status        int
		body          string
		readErr       bool
		readGone      bool
		deleteErr     bool
		deleteRemoves bool
	}{
		{name: "policy exists", status: http.StatusOK, body: policyBody, deleteRemoves: true},
		{name: "policy deleted", status: http.StatusNotFound, body: `{"Code":"XMinioAdminNoSuchPolicy","Message":"The canned policy does not exist"}`, readGone: true},
		{name: "access denied", status: http.StatusForbidden, body: `{"Code":"AccessDenied","Message":"Access Denied."}`, readErr: true, deleteErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ops := []struct {
				name string
				run  func(context.Context, *schema.ResourceData, interface{}) diag.Diagnostics
			}{
				{"read", minioReadGroupPolicy},
				{"delete", minioDeleteGroupPolicy},
			}
			for _, op := range ops {
				removed := false
				client := groupPolicyAdminStub(t, tc.status, tc.body, &removed)

				d := resourceMinioIAMGroupPolicy().TestResourceData()
				d.SetId("test-group:test-policy")

				diags := op.run(context.Background(), d, &S3MinioClient{S3Admin: client})

				if op.name == "read" {
					if diags.HasError() != tc.readErr {
						t.Fatalf("read: wantErr %v, got %v", tc.readErr, diags)
					}
					if gone := d.Id() == ""; gone != tc.readGone {
						t.Fatalf("read: wantGone %v, id is %q", tc.readGone, d.Id())
					}
					continue
				}
				if diags.HasError() != tc.deleteErr {
					t.Fatalf("delete: wantErr %v, got %v", tc.deleteErr, diags)
				}
				if removed != tc.deleteRemoves {
					t.Fatalf("delete: remove-canned-policy called = %v, want %v", removed, tc.deleteRemoves)
				}
			}
		})
	}
}

func TestIAMGroupPolicyReadAfterCreateReportsMissingPolicy(t *testing.T) {
	removed := false
	client := groupPolicyAdminStub(t, http.StatusNotFound, `{"Code":"XMinioAdminNoSuchPolicy","Message":"The canned policy does not exist"}`, &removed)

	d := resourceMinioIAMGroupPolicy().TestResourceData()
	d.MarkNewResource()
	d.SetId("test-group:test-policy")

	if diags := minioReadGroupPolicy(context.Background(), d, &S3MinioClient{S3Admin: client}); !diags.HasError() {
		t.Fatal("expected an error when the policy is missing right after create")
	}
}
