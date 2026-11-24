package minio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestSiteReplicationReadChecksStatePopulation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/site-replication/info") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(madmin.SiteReplicationInfo{
			Enabled: true,
			Name:    "repl-cluster",
			Sites: []madmin.PeerInfo{
				{Name: "site-a", Endpoint: "https://site-a.local:9000"},
				{Name: "site-b", Endpoint: "https://site-b.local:9000"},
			},
		})
	}))
	t.Cleanup(srv.Close)

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}
	meta := &S3MinioClient{S3Admin: admin}

	t.Run("read_sets_name_enabled_and_preserves_site_credentials", func(t *testing.T) {
		d := schema.TestResourceDataRaw(t, resourceMinioSiteReplication().Schema, map[string]interface{}{
			"name": "repl-cluster",
			"site": []interface{}{
				map[string]interface{}{
					"name":       "site-a",
					"endpoint":   "https://site-a.local:9000",
					"access_key": "ak-a",
					"secret_key": "sk-a",
				},
				map[string]interface{}{
					"name":       "site-b",
					"endpoint":   "https://site-b.local:9000",
					"access_key": "ak-b",
					"secret_key": "sk-b",
				},
			},
		})
		d.SetId("repl-cluster")

		if diags := minioReadSiteReplication(context.Background(), d, meta); diags.HasError() {
			t.Fatalf("minioReadSiteReplication failed: %v", diags)
		}
		if got := d.Get("name").(string); got != "repl-cluster" {
			t.Errorf("name = %q, want %q", got, "repl-cluster")
		}
		if !d.Get("enabled").(bool) {
			t.Error("expected enabled=true")
		}
		sites := d.Get("site").([]interface{})
		if len(sites) != 2 {
			t.Fatalf("len(site) = %d, want 2", len(sites))
		}
		first := sites[0].(map[string]interface{})
		if first["access_key"] != "ak-a" || first["secret_key"] != "sk-a" {
			t.Errorf("site[0] credentials = (%v, %v), want (ak-a, sk-a)", first["access_key"], first["secret_key"])
		}
	})

	t.Run("import_style_read_falls_back_to_id", func(t *testing.T) {
		d := resourceMinioSiteReplication().TestResourceData()
		d.SetId("imported-repl")

		if diags := minioReadSiteReplication(context.Background(), d, meta); diags.HasError() {
			t.Fatalf("minioReadSiteReplication import failed: %v", diags)
		}
		if got := d.Get("name").(string); got != "imported-repl" {
			t.Errorf("name = %q, want %q", got, "imported-repl")
		}
		if !d.Get("enabled").(bool) {
			t.Error("expected enabled=true")
		}
		if sites := d.Get("site").([]interface{}); len(sites) != 2 {
			t.Errorf("len(site) = %d, want 2", len(sites))
		}
	})
}
