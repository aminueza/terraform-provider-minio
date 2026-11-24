package minio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func newConfigKVStubClient(t *testing.T, applyWithoutRestart bool, initial map[string]string) (*S3MinioClient, map[string]string) {
	t.Helper()

	const stubSecret = "secretkey"
	var mu sync.Mutex
	store := make(map[string]string, len(initial))
	for k, v := range initial {
		store[k] = v
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint := adminPathWithoutVersion(r.URL.Path)
		switch endpoint {
		case "/set-config-kv":
			encryptedBody, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("reading set-config-kv body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			plain, err := madmin.DecryptData(stubSecret, io.NopCloser(strings.NewReader(string(encryptedBody))))
			if err != nil {
				t.Errorf("decrypting set-config-kv payload: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			kv := strings.TrimSpace(string(plain))
			key := kv
			if idx := strings.IndexByte(kv, ' '); idx > 0 {
				key = kv[:idx]
			}
			mu.Lock()
			store[key] = kv
			mu.Unlock()
			if applyWithoutRestart {
				w.Header().Set(madmin.ConfigAppliedHeader, madmin.ConfigAppliedTrue)
			}
			w.WriteHeader(http.StatusOK)
		case "/get-config-kv":
			key := r.URL.Query().Get("key")
			mu.Lock()
			val, ok := store[key]
			mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				if _, err := w.Write([]byte(`{"Code":"NotFound","Message":"key not found"}`)); err != nil {
					t.Errorf("writing not-found response: %v", err)
				}
				return
			}
			encrypted, err := madmin.EncryptData(stubSecret, []byte(val))
			if err != nil {
				t.Errorf("encrypting get-config-kv response: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if _, err := w.Write(encrypted); err != nil {
				t.Errorf("writing get-config-kv response: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", stubSecret, ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating madmin stub client: %v", err)
	}

	return &S3MinioClient{S3Admin: admin}, store
}

func TestConfigResourcesSetResourceFieldReportsSchemaErrors(t *testing.T) {
	t.Setenv("TF_ACC", "")

	resources := []struct {
		name string
		res  *schema.Resource
		raw  map[string]interface{}
		key  string
		val  interface{}
	}{
		{
			name: "minio_config",
			res:  resourceMinioConfig(),
			raw:  map[string]interface{}{"key": "api", "value": "requests_max=100"},
			key:  "restart_required",
			val:  true,
		},
		{
			name: "minio_server_config_api",
			res:  resourceMinioServerConfigApi(),
			raw:  map[string]interface{}{"requests_max": "100"},
			key:  "restart_required",
			val:  false,
		},
		{
			name: "minio_server_config_etcd",
			res:  resourceMinioServerConfigEtcd(),
			raw:  map[string]interface{}{"endpoints": "http://127.0.0.1:2379"},
			key:  "path_prefix",
			val:  "/tenant",
		},
		{
			name: "minio_server_config_heal",
			res:  resourceMinioServerConfigHeal(),
			raw:  map[string]interface{}{"bitrotscan": "on"},
			key:  "max_io",
			val:  "16",
		},
		{
			name: "minio_server_config_region",
			res:  resourceMinioServerConfigRegion(),
			raw:  map[string]interface{}{"name": "us-east-1"},
			key:  "comment",
			val:  "primary",
		},
		{
			name: "minio_server_config_scanner",
			res:  resourceMinioServerConfigScanner(),
			raw:  map[string]interface{}{"speed": "fast"},
			key:  "restart_required",
			val:  true,
		},
		{
			name: "minio_server_config_storage_class",
			res:  resourceMinioServerConfigStorageClass(),
			raw:  map[string]interface{}{"standard": "EC:2"},
			key:  "rrs",
			val:  "EC:1",
		},
	}

	for _, tc := range resources {
		t.Run(tc.name, func(t *testing.T) {
			d := schema.TestResourceDataRaw(t, tc.res.Schema, tc.raw)
			if diags := setResourceField(d, tc.key, tc.val, tc.name); diags != nil {
				t.Fatalf("unexpected error setting %q: %v", tc.key, diags)
			}

			errDiags := setResourceField(d, "nonexistent_schema_attribute", "value", tc.name)
			if errDiags == nil || !errDiags.HasError() {
				t.Fatalf("expected error setting nonexistent field, got %+v", errDiags)
			}
			if !strings.Contains(errDiags[0].Summary, "setting nonexistent_schema_attribute") {
				t.Fatalf("expected summary to mention setting nonexistent_schema_attribute, got %q", errDiags[0].Summary)
			}
		})
	}
}

func TestServerConfigAndConfigDSetAgainstStubAdmin(t *testing.T) {
	ctx := context.Background()

	t.Run("minio_config_create_read_update", func(t *testing.T) {
		client, _ := newConfigKVStubClient(t, true, nil)
		d := schema.TestResourceDataRaw(t, resourceMinioConfig().Schema, map[string]interface{}{
			"key":   "api",
			"value": "requests_max=500",
		})
		if diags := minioCreateConfig(ctx, d, client); diags.HasError() {
			t.Fatalf("minioCreateConfig failed: %v", diags)
		}
		if d.Get("restart_required").(bool) {
			t.Fatal("expected restart_required=false when ConfigApplied=true")
		}
		if got := d.Get("value").(string); got != "requests_max=500" {
			t.Fatalf("value = %q, want %q", got, "requests_max=500")
		}
		if got := d.Get("key").(string); got != "api" {
			t.Fatalf("key = %q, want %q", got, "api")
		}

		clientRestart, _ := newConfigKVStubClient(t, false, nil)
		if diags := minioUpdateConfig(ctx, d, clientRestart); diags.HasError() {
			t.Fatalf("minioUpdateConfig failed: %v", diags)
		}
		if !d.Get("restart_required").(bool) {
			t.Fatal("expected restart_required=true when ConfigApplied header absent")
		}

		rawClient, _ := newConfigKVStubClient(t, true, map[string]string{
			"region": "us-west-2",
		})
		dRaw := schema.TestResourceDataRaw(t, resourceMinioConfig().Schema, map[string]interface{}{
			"key":   "region",
			"value": "us-east-1",
		})
		dRaw.SetId("region")
		if diags := minioReadConfig(ctx, dRaw, rawClient); diags.HasError() {
			t.Fatalf("minioReadConfig raw failed: %v", diags)
		}
		if got := dRaw.Get("value").(string); got != "us-west-2" {
			t.Fatalf("raw value = %q, want %q", got, "us-west-2")
		}
	})

	t.Run("server_config_etcd", func(t *testing.T) {
		client, _ := newConfigKVStubClient(t, true, nil)
		d := schema.TestResourceDataRaw(t, resourceMinioServerConfigEtcd().Schema, map[string]interface{}{
			"endpoints":       "http://127.0.0.1:2379",
			"path_prefix":     "/tenant",
			"coredns_path":    "/skydns",
			"client_cert":     "/tmp/cert.pem",
			"client_cert_key": "/tmp/key.pem",
			"comment":         "fed",
		})
		if diags := minioServerConfigEtcdSet(ctx, d, client); diags.HasError() {
			t.Fatalf("minioServerConfigEtcdSet failed: %v", diags)
		}
		if got := d.Get("path_prefix").(string); got != "/tenant" {
			t.Fatalf("path_prefix = %q, want %q", got, "/tenant")
		}
		if got := d.Get("comment").(string); got != "fed" {
			t.Fatalf("comment = %q, want %q", got, "fed")
		}
		if d.Get("restart_required").(bool) {
			t.Fatal("expected restart_required=false")
		}
	})

	t.Run("server_config_heal", func(t *testing.T) {
		client, _ := newConfigKVStubClient(t, false, nil)
		d := schema.TestResourceDataRaw(t, resourceMinioServerConfigHeal().Schema, map[string]interface{}{
			"bitrotscan":    "on",
			"max_sleep":     "100ms",
			"max_io":        "8",
			"drive_workers": "2",
		})
		if diags := minioServerConfigHealSet(ctx, d, client); diags.HasError() {
			t.Fatalf("minioServerConfigHealSet failed: %v", diags)
		}
		if !d.Get("restart_required").(bool) {
			t.Fatal("expected restart_required=true")
		}
		if got := d.Get("max_io").(string); got != "8" {
			t.Fatalf("max_io = %q, want %q", got, "8")
		}
	})

	t.Run("server_config_region", func(t *testing.T) {
		client, _ := newConfigKVStubClient(t, true, nil)
		d := schema.TestResourceDataRaw(t, resourceMinioServerConfigRegion().Schema, map[string]interface{}{
			"name":    "eu-central-1",
			"comment": "fra",
		})
		if diags := minioServerConfigRegionSet(ctx, d, client); diags.HasError() {
			t.Fatalf("minioServerConfigRegionSet failed: %v", diags)
		}
		if got := d.Get("name").(string); got != "eu-central-1" {
			t.Fatalf("name = %q, want %q", got, "eu-central-1")
		}
		if got := d.Get("comment").(string); got != "fra" {
			t.Fatalf("comment = %q, want %q", got, "fra")
		}
	})

	t.Run("server_config_api_scanner_storage_class", func(t *testing.T) {
		client, _ := newConfigKVStubClient(t, true, nil)

		dAPI := schema.TestResourceDataRaw(t, resourceMinioServerConfigApi().Schema, map[string]interface{}{
			"requests_max": "250",
		})
		if diags := minioServerConfigApiSet(ctx, dAPI, client); diags.HasError() {
			t.Fatalf("minioServerConfigApiSet failed: %v", diags)
		}
		if got := dAPI.Get("requests_max").(string); got != "250" {
			t.Fatalf("requests_max = %q, want %q", got, "250")
		}

		dScanner := schema.TestResourceDataRaw(t, resourceMinioServerConfigScanner().Schema, map[string]interface{}{
			"speed": "fast",
		})
		if diags := minioServerConfigScannerSet(ctx, dScanner, client); diags.HasError() {
			t.Fatalf("minioServerConfigScannerSet failed: %v", diags)
		}
		if got := dScanner.Get("speed").(string); got != "fast" {
			t.Fatalf("speed = %q, want %q", got, "fast")
		}

		dSC := schema.TestResourceDataRaw(t, resourceMinioServerConfigStorageClass().Schema, map[string]interface{}{
			"standard": "EC:2",
			"rrs":      "EC:1",
			"comment":  "tiered",
		})
		if diags := minioServerConfigStorageClassSet(ctx, dSC, client); diags.HasError() {
			t.Fatalf("minioServerConfigStorageClassSet failed: %v", diags)
		}
		if got := dSC.Get("standard").(string); got != "EC:2" {
			t.Fatalf("standard = %q, want %q", got, "EC:2")
		}
		if got := dSC.Get("rrs").(string); got != "EC:1" {
			t.Fatalf("rrs = %q, want %q", got, "EC:1")
		}
	})
}
