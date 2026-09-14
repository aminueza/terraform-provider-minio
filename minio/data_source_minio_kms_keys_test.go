package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func kmsKeysStub(t *testing.T, status int, body string, gotQuery *url.Values) *S3MinioClient {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotQuery != nil {
			*gotQuery = r.URL.Query()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating the admin client: %s", err)
	}

	return &S3MinioClient{S3Admin: admin}
}

func TestDataSourceKMSKeysReadsWhatTheServerReports(t *testing.T) {
	body := `[
	  {"name":"terraform-key","createdAt":"2026-09-11T10:00:00Z","createdBy":"minio"},
	  {"name":"legacy-key"}
	]`

	var query url.Values
	meta := kmsKeysStub(t, http.StatusOK, body, &query)

	d := schema.TestResourceDataRaw(t, dataSourceMinioKMSKeys().Schema, map[string]interface{}{
		"pattern": "terraform-*",
	})

	if diags := dataSourceMinioKMSKeysRead(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("reading the data source: %v", diags)
	}

	if got := query.Get("pattern"); got != "terraform-*" {
		t.Errorf("the server received pattern %q, want the configured %q", got, "terraform-*")
	}
	if d.Id() != "terraform-*" {
		t.Errorf("id = %q, want the pattern, so repeated plans stay stable", d.Id())
	}

	keys, ok := d.Get("keys").([]interface{})
	if !ok {
		t.Fatalf("keys is %T, want a list", d.Get("keys"))
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(keys))
	}

	first := keys[0].(map[string]interface{})
	if first["name"] != "terraform-key" {
		t.Errorf("keys[0].name = %v", first["name"])
	}
	if first["created_at"] != "2026-09-11T10:00:00Z" {
		t.Errorf("keys[0].created_at = %v, want RFC 3339", first["created_at"])
	}
	if first["created_by"] != "minio" {
		t.Errorf("keys[0].created_by = %v", first["created_by"])
	}

	second := keys[1].(map[string]interface{})
	if second["name"] != "legacy-key" {
		t.Errorf("keys[1].name = %v", second["name"])
	}
	if second["created_at"] != "" {
		t.Errorf("keys[1].created_at = %v, want empty when the backend reports no creation time", second["created_at"])
	}
}

func TestDataSourceKMSKeysDefaultsToEveryKey(t *testing.T) {
	var query url.Values
	meta := kmsKeysStub(t, http.StatusOK, `[]`, &query)

	d := schema.TestResourceDataRaw(t, dataSourceMinioKMSKeys().Schema, map[string]interface{}{})

	if diags := dataSourceMinioKMSKeysRead(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("reading the data source: %v", diags)
	}

	if got := query.Get("pattern"); got != "*" {
		t.Errorf("the server received pattern %q, want %q", got, "*")
	}

	keys := d.Get("keys").([]interface{})
	if len(keys) != 0 {
		t.Errorf("got %d keys, want an empty list rather than a failure", len(keys))
	}
}

func TestDataSourceKMSKeysReportsAServerError(t *testing.T) {
	meta := kmsKeysStub(t, http.StatusNotImplemented, `{"Code":"NotImplemented","Message":"key listing is not supported"}`, nil)

	d := schema.TestResourceDataRaw(t, dataSourceMinioKMSKeys().Schema, map[string]interface{}{})

	diags := dataSourceMinioKMSKeysRead(context.Background(), d, meta)
	if !diags.HasError() {
		t.Fatal("a server that refuses to list keys must be reported, not read as an empty list")
	}
	if !strings.Contains(diags[0].Summary, "listing KMS keys") {
		t.Errorf("summary = %q, want it to name the operation", diags[0].Summary)
	}
}

func TestAccDataSourceMinioKMSKeys_basic(t *testing.T) {
	keyID := fmt.Sprintf("tfacc-kms-keys-%d", acctest.RandInt())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckKMSKey(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioKMSKeyDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "minio_kms_key" "test" {
  provider = "kmsminio"
  key_id   = %q
}

data "minio_kms_keys" "match" {
  provider = "kmsminio"
  pattern  = minio_kms_key.test.key_id
}

data "minio_kms_keys" "all" {
  provider   = "kmsminio"
  depends_on = [minio_kms_key.test]
}
`, keyID),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_kms_keys.match", "keys.#", "1"),
					resource.TestCheckResourceAttr("data.minio_kms_keys.match", "keys.0.name", keyID),
					testAccCheckKMSKeysListContains("data.minio_kms_keys.all", keyID),
				),
			},
		},
	})
}

func testAccCheckKMSKeysListContains(dataSourceName, keyID string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[dataSourceName]
		if !ok {
			return fmt.Errorf("not found: %s", dataSourceName)
		}

		count := rs.Primary.Attributes["keys.#"]
		if count == "" || count == "0" {
			return fmt.Errorf("%s listed no keys at all", dataSourceName)
		}

		for attribute, value := range rs.Primary.Attributes {
			if strings.HasSuffix(attribute, ".name") && value == keyID {
				return nil
			}
		}

		return fmt.Errorf("%s listed %s keys, none of them %s", dataSourceName, count, keyID)
	}
}
