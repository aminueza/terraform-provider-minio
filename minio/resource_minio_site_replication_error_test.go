package minio

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestIsSiteReplicationNotConfiguredError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "madmin config missing error",
			err: madmin.ErrorResponse{
				Code:       "XMinioSiteReplicationConfigMissingError",
				Message:    "Site not found in site replication configuration",
				StatusCode: http.StatusBadRequest,
			},
			want: true,
		},
		{
			name: "madmin invalid request when not enabled",
			err: madmin.ErrorResponse{
				Code:       "XMinioSiteReplicationInvalidRequest",
				Message:    "site replication is not enabled",
				StatusCode: http.StatusBadRequest,
			},
			want: true,
		},
		{
			name: "wrapped madmin invalid request with not configured",
			err: fmt.Errorf("admin call failed: %w", madmin.ErrorResponse{
				Code:       "XMinioSiteReplicationInvalidRequest",
				Message:    "site replication not configured",
				StatusCode: http.StatusBadRequest,
			}),
			want: true,
		},
		{
			name: "madmin invalid request for different validation error stays fatal",
			err: madmin.ErrorResponse{
				Code:       "XMinioSiteReplicationInvalidRequest",
				Message:    "all existing replicated sites must be specified - missing site-b",
				StatusCode: http.StatusBadRequest,
			},
			want: false,
		},
		{
			name: "madmin peer response error stays fatal",
			err: madmin.ErrorResponse{
				Code:       "XMinioSiteReplicationPeerResp",
				Message:    "Unable to fetch server info for site-b: dial tcp 10.0.0.2:9000: connect: connection refused",
				StatusCode: http.StatusBadRequest,
			},
			want: false,
		},
		{
			name: "madmin backend unavailable stays fatal",
			err: madmin.ErrorResponse{
				Code:       "XMinioSiteReplicationBackendIssue",
				Message:    "Error when requesting object layer backend",
				StatusCode: http.StatusServiceUnavailable,
			},
			want: false,
		},
		{
			name: "plain site replication not configured",
			err:  errors.New("site replication not configured"),
			want: true,
		},
		{
			name: "plain unable to fetch server info does not drop state",
			err:  errors.New("Unable to fetch server info for site-b"),
			want: false,
		},
		{
			name: "plain global deployment ID mismatch does not drop state",
			err:  errors.New("Global deployment ID mismatch"),
			want: false,
		},
		{
			name: "plain transport timeout does not drop state",
			err:  errors.New("context deadline exceeded"),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSiteReplicationNotConfiguredError(tc.err); got != tc.want {
				t.Errorf("isSiteReplicationNotConfiguredError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestSiteReplicationReadPreservesStateOnTransientErrors(t *testing.T) {
	newMeta := func(t *testing.T, status int, code, msg string) *S3MinioClient {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, `{"Code":%q,"Message":%q}`, code, msg)
		}))
		t.Cleanup(srv.Close)

		admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
			Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
			Secure: false,
		})
		if err != nil {
			t.Fatalf("creating admin client: %v", err)
		}
		return &S3MinioClient{S3Admin: admin}
	}

	newSiteReplicationData := func(t *testing.T) *schema.ResourceData {
		t.Helper()
		d := schema.TestResourceDataRaw(t, resourceMinioSiteReplication().Schema, map[string]interface{}{
			"name": "repl-cluster",
			"site": []interface{}{
				map[string]interface{}{"name": "site-a", "endpoint": "https://site-a.local:9000"},
				map[string]interface{}{"name": "site-b", "endpoint": "https://site-b.local:9000"},
			},
		})
		d.SetId("repl-cluster")
		return d
	}

	t.Run("peer unreachable keeps state and returns error", func(t *testing.T) {
		meta := newMeta(t, http.StatusBadRequest, "XMinioSiteReplicationPeerResp", "Unable to fetch server info for site-b")
		d := newSiteReplicationData(t)

		diags := minioReadSiteReplication(context.Background(), d, meta)
		if !diags.HasError() {
			t.Fatal("expected error diagnostic when peer server info is unreachable")
		}
		if got := d.Id(); got != "repl-cluster" {
			t.Errorf("d.Id() = %q, want %q (state must be preserved)", got, "repl-cluster")
		}
	})

	t.Run("backend error keeps state and returns error", func(t *testing.T) {
		meta := newMeta(t, http.StatusInternalServerError, "XMinioSiteReplicationBackendIssue", "Error when requesting object layer backend")
		d := newSiteReplicationData(t)

		diags := minioReadSiteReplication(context.Background(), d, meta)
		if !diags.HasError() {
			t.Fatal("expected error diagnostic on backend issue")
		}
		if got := d.Id(); got != "repl-cluster" {
			t.Errorf("d.Id() = %q, want %q (state must be preserved)", got, "repl-cluster")
		}
	})

	t.Run("canceled context keeps state and returns error", func(t *testing.T) {
		meta := newMeta(t, http.StatusOK, "", "")
		d := newSiteReplicationData(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		diags := minioReadSiteReplication(ctx, d, meta)
		if !diags.HasError() {
			t.Fatal("expected error diagnostic on canceled context")
		}
		if got := d.Id(); got != "repl-cluster" {
			t.Errorf("d.Id() = %q, want %q (state must be preserved)", got, "repl-cluster")
		}
	})

	t.Run("not enabled error clears state without error", func(t *testing.T) {
		meta := newMeta(t, http.StatusBadRequest, "XMinioSiteReplicationInvalidRequest", "site replication is not enabled")
		d := newSiteReplicationData(t)

		diags := minioReadSiteReplication(context.Background(), d, meta)
		if diags.HasError() {
			t.Fatalf("unexpected error for unconfigured site replication: %v", diags)
		}
		if got := d.Id(); got != "" {
			t.Errorf("d.Id() = %q, want empty string", got)
		}
	})

	t.Run("config missing error clears state without error", func(t *testing.T) {
		meta := newMeta(t, http.StatusBadRequest, "XMinioSiteReplicationConfigMissingError", "Site not found in site replication configuration")
		d := newSiteReplicationData(t)

		diags := minioReadSiteReplication(context.Background(), d, meta)
		if diags.HasError() {
			t.Fatalf("unexpected error for missing site replication config: %v", diags)
		}
		if got := d.Id(); got != "" {
			t.Errorf("d.Id() = %q, want empty string", got)
		}
	})

	t.Run("delete treats not enabled as already absent", func(t *testing.T) {
		meta := newMeta(t, http.StatusBadRequest, "XMinioSiteReplicationInvalidRequest", "site replication is not enabled")
		d := newSiteReplicationData(t)

		if diags := minioDeleteSiteReplication(context.Background(), d, meta); diags.HasError() {
			t.Fatalf("expected delete to treat not-enabled as nil, got: %v", diags)
		}
	})

	t.Run("delete returns error when peer is unreachable", func(t *testing.T) {
		meta := newMeta(t, http.StatusBadRequest, "XMinioSiteReplicationPeerResp", "Unable to fetch server info for site-b")
		d := newSiteReplicationData(t)

		if diags := minioDeleteSiteReplication(context.Background(), d, meta); !diags.HasError() {
			t.Fatal("expected delete to surface peer reachability failure")
		}
	})
}
