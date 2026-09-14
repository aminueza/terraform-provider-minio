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

const accessKeysStubSecret = "secretkey"

func adminPathWithoutVersion(path string) string {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segments) < 4 || segments[0] != "minio" || segments[1] != "admin" {
		return path
	}
	return "/" + strings.Join(segments[3:], "/")
}

func accessKeysStub(t *testing.T, bodies map[string]string, requests map[string]url.Values) *S3MinioClient {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint := adminPathWithoutVersion(r.URL.Path)
		body, ok := bodies[endpoint]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, `{"Code":"NotFound","Message":"no stub for %s"}`, endpoint)
			return
		}
		if requests != nil {
			requests[endpoint] = r.URL.Query()
		}

		encrypted, err := madmin.EncryptData(accessKeysStubSecret, []byte(body))
		if err != nil {
			t.Errorf("encrypting the stub response: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(encrypted)
	}))
	t.Cleanup(srv.Close)

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", accessKeysStubSecret, ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating the admin client: %s", err)
	}

	return &S3MinioClient{S3Admin: admin}
}

func TestDataSourceAccessKeysDefaultsToBuiltinOnly(t *testing.T) {
	requests := map[string]url.Values{}
	meta := accessKeysStub(t, map[string]string{
		"/list-access-keys-bulk": `{
		  "carol": {"serviceAccounts": [{"accessKey": "CAROLKEY", "parentUser": "carol", "accountStatus": "on"}]},
		  "alice": {"serviceAccounts": [
		    {"accessKey": "ALICEKEY2", "parentUser": "alice", "accountStatus": "off", "name": "ci", "description": "pipeline"},
		    {"accessKey": "ALICEKEY1", "parentUser": "alice", "accountStatus": "on", "expiration": "2026-12-01T00:00:00Z"}
		  ]}
		}`,
	}, requests)

	d := schema.TestResourceDataRaw(t, dataSourceMinioAccessKeys().Schema, map[string]interface{}{})

	if diags := dataSourceMinioAccessKeysRead(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("reading the data source: %v", diags)
	}

	if got := requests["/list-access-keys-bulk"].Get("all"); got != "true" {
		t.Errorf("all = %q, want %q when no users are given", got, "true")
	}

	builtin := d.Get("builtin").([]interface{})
	if len(builtin) != 3 {
		t.Fatalf("got %d builtin keys, want 3", len(builtin))
	}

	var order []string
	for _, entry := range builtin {
		order = append(order, entry.(map[string]interface{})["access_key"].(string))
	}
	want := []string{"ALICEKEY1", "ALICEKEY2", "CAROLKEY"}
	for i, key := range want {
		if order[i] != key {
			t.Fatalf("order = %v, want %v: the server returns a map, so the listing must be sorted or every plan shows a diff", order, want)
		}
	}

	first := builtin[0].(map[string]interface{})
	if first["user"] != "alice" || first["parent_user"] != "alice" {
		t.Errorf("first entry = %v, want it attributed to alice", first)
	}
	if first["expiration"] != "2026-12-01T00:00:00Z" {
		t.Errorf("expiration = %v, want RFC 3339", first["expiration"])
	}
	if builtin[2].(map[string]interface{})["expiration"] != "" {
		t.Errorf("a key with no expiry must report an empty expiration, got %v", builtin[2])
	}
	if second := builtin[1].(map[string]interface{}); second["name"] != "ci" || second["description"] != "pipeline" {
		t.Errorf("name and description are not carried through: %v", second)
	}

	if len(d.Get("ldap").([]interface{})) != 0 || len(d.Get("openid").([]interface{})) != 0 {
		t.Error("ldap and openid must stay empty when they are not asked for")
	}
}

func TestDataSourceAccessKeysNeverExposesSecretMaterial(t *testing.T) {
	for name := range dataSourceMinioAccessKeys().Schema {
		if strings.Contains(name, "secret") {
			t.Errorf("top-level attribute %q looks like secret material", name)
		}
	}

	for _, group := range []string{"builtin", "ldap", "openid"} {
		elem := dataSourceMinioAccessKeys().Schema[group].Elem.(*schema.Resource)
		for name := range elem.Schema {
			if strings.Contains(name, "secret") {
				t.Errorf("%s.%s looks like secret material, which this data source must never carry", group, name)
			}
		}
	}
}

func TestDataSourceAccessKeysQueriesEachRequestedProvider(t *testing.T) {
	requests := map[string]url.Values{}
	meta := accessKeysStub(t, map[string]string{
		"/list-access-keys-bulk": `{"alice": {"serviceAccounts": [{"accessKey": "BUILTIN", "parentUser": "alice", "accountStatus": "on"}]}}`,
		"/idp/ldap/list-access-keys-bulk": `{"uid=bob,ou=users,dc=example,dc=com": {"serviceAccounts": [
		  {"accessKey": "LDAPKEY", "parentUser": "uid=bob,ou=users,dc=example,dc=com", "accountStatus": "on"}
		]}}`,
		"/idp/openid/list-access-keys-bulk": `[{"configName": "_", "users": [
		  {"minioAccessKey": "STSKEY", "ID": "sub-123", "readableName": "dana", "serviceAccounts": [
		    {"accessKey": "OIDCKEY", "parentUser": "sub-123", "accountStatus": "on"}
		  ]}
		]}]`,
	}, requests)

	d := schema.TestResourceDataRaw(t, dataSourceMinioAccessKeys().Schema, map[string]interface{}{
		"identity_providers": []interface{}{"builtin", "ldap", "openid"},
	})

	if diags := dataSourceMinioAccessKeysRead(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("reading the data source: %v", diags)
	}

	if len(d.Get("builtin").([]interface{})) != 1 {
		t.Error("the builtin group is missing its key")
	}

	ldap := d.Get("ldap").([]interface{})
	if len(ldap) != 1 {
		t.Fatalf("got %d LDAP keys, want 1", len(ldap))
	}
	if user := ldap[0].(map[string]interface{})["user"]; user != "uid=bob,ou=users,dc=example,dc=com" {
		t.Errorf("ldap user = %v, want the DN the server grouped the key under", user)
	}

	openid := d.Get("openid").([]interface{})
	if len(openid) != 1 {
		t.Fatalf("got %d OpenID keys, want 1", len(openid))
	}
	entry := openid[0].(map[string]interface{})
	if entry["access_key"] != "OIDCKEY" || entry["user_id"] != "sub-123" || entry["readable_name"] != "dana" || entry["config_name"] != "_" {
		t.Errorf("openid entry = %v, want the claims carried alongside the key", entry)
	}
}

func TestDataSourceAccessKeysPassesTheRequestedUsers(t *testing.T) {
	requests := map[string]url.Values{}
	meta := accessKeysStub(t, map[string]string{
		"/list-access-keys-bulk": `{"alice": {"serviceAccounts": []}}`,
	}, requests)

	d := schema.TestResourceDataRaw(t, dataSourceMinioAccessKeys().Schema, map[string]interface{}{
		"users": []interface{}{"alice", "bob"},
	})

	if diags := dataSourceMinioAccessKeysRead(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("reading the data source: %v", diags)
	}

	query := requests["/list-access-keys-bulk"]
	if got := query["users"]; len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Errorf("users = %v, want both names", got)
	}
	if query.Get("all") == "true" {
		t.Error("all must not be set alongside an explicit user list: the client rejects that combination")
	}
	if !strings.Contains(d.Id(), "alice,bob") {
		t.Errorf("id = %q, want it to carry the query so two differently filtered data sources do not collide", d.Id())
	}
}

func TestDataSourceAccessKeysReportsAProviderTheServerDoesNotRun(t *testing.T) {
	meta := accessKeysStub(t, map[string]string{
		"/list-access-keys-bulk": `{}`,
	}, nil)

	d := schema.TestResourceDataRaw(t, dataSourceMinioAccessKeys().Schema, map[string]interface{}{
		"identity_providers": []interface{}{"ldap"},
	})

	diags := dataSourceMinioAccessKeysRead(context.Background(), d, meta)
	if !diags.HasError() {
		t.Fatal("asking for an identity provider the server does not run must be reported, not read as an empty group")
	}
	if !strings.Contains(diags[0].Summary, "listing LDAP access keys") {
		t.Errorf("summary = %q, want it to name the provider that failed", diags[0].Summary)
	}
}

func TestAccDataSourceMinioAccessKeys_builtin(t *testing.T) {
	user := fmt.Sprintf("tfacc-ak-user-%d", acctest.RandInt())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "minio_iam_user" "test" {
  name          = %q
  force_destroy = true
}

resource "minio_iam_service_account" "test" {
  target_user = minio_iam_user.test.name
}

data "minio_access_keys" "all" {
  depends_on = [minio_iam_service_account.test]
}

data "minio_access_keys" "scoped" {
  users      = [minio_iam_user.test.name]
  depends_on = [minio_iam_service_account.test]
}
`, user),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAccessKeysContain("data.minio_access_keys.all", "minio_iam_service_account.test"),
					testAccCheckAccessKeysContain("data.minio_access_keys.scoped", "minio_iam_service_account.test"),
					resource.TestCheckResourceAttr("data.minio_access_keys.scoped", "ldap.#", "0"),
					resource.TestCheckResourceAttr("data.minio_access_keys.scoped", "openid.#", "0"),
					testAccCheckAccessKeysHoldNoSecret("data.minio_access_keys.all"),
				),
			},
		},
	})
}

func testAccCheckAccessKeysContain(dataSourceName, serviceAccountName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		account, ok := s.RootModule().Resources[serviceAccountName]
		if !ok {
			return fmt.Errorf("not found: %s", serviceAccountName)
		}
		wanted := account.Primary.Attributes["access_key"]
		if wanted == "" {
			return fmt.Errorf("%s has no access_key in state", serviceAccountName)
		}

		listing, ok := s.RootModule().Resources[dataSourceName]
		if !ok {
			return fmt.Errorf("not found: %s", dataSourceName)
		}

		for attribute, value := range listing.Primary.Attributes {
			if strings.HasPrefix(attribute, "builtin.") && strings.HasSuffix(attribute, ".access_key") && value == wanted {
				return nil
			}
		}

		return fmt.Errorf("%s does not list the access key %s created in this run", dataSourceName, wanted)
	}
}

func testAccCheckAccessKeysHoldNoSecret(dataSourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		listing, ok := s.RootModule().Resources[dataSourceName]
		if !ok {
			return fmt.Errorf("not found: %s", dataSourceName)
		}

		for attribute, value := range listing.Primary.Attributes {
			if strings.Contains(attribute, "secret") && value != "" {
				return fmt.Errorf("%s holds secret material in %s", dataSourceName, attribute)
			}
		}

		return nil
	}
}
