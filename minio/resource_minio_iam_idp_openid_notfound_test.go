package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMinioReadIdpOpenIdNotFound(t *testing.T) {
	cases := []struct {
		name            string
		afterWrite      bool
		restartRequired bool
		statusCode      int
		body            string
		wantErr         bool
		wantID          string
	}{
		{
			name:       "not found after write keeps resource in state",
			afterWrite: true,
			statusCode: http.StatusNotFound,
			body:       `{"Code":"XMinioAdminIDPCfgNotFound","Message":"IDP configuration 'tfacc-oidc' not found"}`,
			wantErr:    false,
			wantID:     "tfacc-oidc",
		},
		{
			name:       "not found on refresh clears resource",
			afterWrite: false,
			statusCode: http.StatusNotFound,
			body:       `{"Code":"XMinioAdminIDPCfgNotFound","Message":"IDP configuration 'tfacc-oidc' not found"}`,
			wantErr:    false,
			wantID:     "",
		},
		{
			name:            "not found on refresh keeps resource while restart pending",
			afterWrite:      false,
			restartRequired: true,
			statusCode:      http.StatusNotFound,
			body:            `{"Code":"XMinioAdminIDPCfgNotFound","Message":"IDP configuration 'tfacc-oidc' not found"}`,
			wantErr:         false,
			wantID:          "tfacc-oidc",
		},
		{
			name:       "unrelated error propagates after write",
			afterWrite: true,
			statusCode: http.StatusForbidden,
			body:       `{"Code":"AccessDenied","Message":"access denied"}`,
			wantErr:    true,
			wantID:     "tfacc-oidc",
		},
		{
			name:       "unrelated error propagates on refresh",
			afterWrite: false,
			statusCode: http.StatusForbidden,
			body:       `{"Code":"AccessDenied","Message":"access denied"}`,
			wantErr:    true,
			wantID:     "tfacc-oidc",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()

			host := strings.TrimPrefix(srv.URL, "http://")
			adminClient, err := madmin.NewWithOptions(host, &madmin.Options{
				Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
				Secure: false,
			})
			if err != nil {
				t.Fatalf("creating admin client: %v", err)
			}

			d := schema.TestResourceDataRaw(t, resourceMinioIAMIdpOpenId().Schema, map[string]interface{}{
				"name":             "tfacc-oidc",
				"config_url":       "http://idp.example.com/.well-known/openid-configuration",
				"client_id":        "client",
				"client_secret":    "secret",
				"restart_required": tc.restartRequired,
			})
			d.SetId("tfacc-oidc")

			meta := &S3MinioClient{S3Admin: adminClient}
			diags := minioReadIdpOpenIdConfig(context.Background(), d, meta, tc.afterWrite)

			if tc.wantErr && len(diags) == 0 {
				t.Error("expected error diagnostics but got none")
			}
			if !tc.wantErr && len(diags) > 0 {
				t.Errorf("expected no error but got: %v", diags)
			}
			if d.Id() != tc.wantID {
				t.Errorf("expected resource ID %q, got %q", tc.wantID, d.Id())
			}
		})
	}
}
