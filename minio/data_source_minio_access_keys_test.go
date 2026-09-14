package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
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

var accessKeyGroupAttributes = map[string][]string{
	"builtin": {"user", "access_key", "parent_user", "name", "description", "status", "expiration"},
	"ldap":    {"user", "access_key", "parent_user", "name", "description", "status", "expiration"},
	"openid":  {"kind", "config_name", "user_id", "readable_name", "access_key", "parent_user", "name", "description", "status", "expiration"},
}

func TestDataSourceAccessKeysExposesExactlyTheseAttributes(t *testing.T) {
	for group, expected := range accessKeyGroupAttributes {
		elem, ok := dataSourceMinioAccessKeys().Schema[group].Elem.(*schema.Resource)
		if !ok {
			t.Fatalf("%s is not a nested resource", group)
		}

		want := map[string]bool{}
		for _, name := range expected {
			want[name] = true
			if _, declared := elem.Schema[name]; !declared {
				t.Errorf("%s.%s is missing from the schema", group, name)
			}
		}

		for name := range elem.Schema {
			if !want[name] {
				t.Errorf("%s.%s is not in the expected set. This data source must carry identifiers and metadata only, so adding an attribute here has to be a deliberate act: add it to accessKeyGroupAttributes and say why in the pull request.", group, name)
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
	if len(openid) != 2 {
		t.Fatalf("got %d OpenID keys, want 2: the credential the login minted and the service account under it", len(openid))
	}

	byKind := map[string]map[string]interface{}{}
	for _, entry := range openid {
		row := entry.(map[string]interface{})
		byKind[row["kind"].(string)] = row
	}

	sts, ok := byKind["sts"]
	if !ok {
		t.Fatalf("the OpenID group has no sts entry: minioAccessKey is the key the login itself minted, and auditing it is the point")
	}
	if sts["access_key"] != "STSKEY" {
		t.Errorf("sts access_key = %v, want the key from minioAccessKey", sts["access_key"])
	}
	if sts["status"] != "" || sts["expiration"] != "" {
		t.Errorf("sts entry = %v: the listing reports no status or expiry for it, so both must stay empty", sts)
	}

	account, ok := byKind["service_account"]
	if !ok {
		t.Fatal("the OpenID group has no service_account entry")
	}
	if account["access_key"] != "OIDCKEY" {
		t.Errorf("service account access_key = %v", account["access_key"])
	}

	for kind, row := range byKind {
		if row["user_id"] != "sub-123" || row["readable_name"] != "dana" || row["config_name"] != "_" {
			t.Errorf("%s entry = %v, want the claims carried alongside the key", kind, row)
		}
	}
}

func TestDataSourceAccessKeysAsksForEveryOpenIDConfig(t *testing.T) {
	requests := map[string]url.Values{}
	meta := accessKeysStub(t, map[string]string{
		"/idp/openid/list-access-keys-bulk": `[]`,
	}, requests)

	d := schema.TestResourceDataRaw(t, dataSourceMinioAccessKeys().Schema, map[string]interface{}{
		"identity_providers": []interface{}{"openid"},
	})

	if diags := dataSourceMinioAccessKeysRead(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("reading the data source: %v", diags)
	}

	if got := requests["/idp/openid/list-access-keys-bulk"].Get("allConfigs"); got != "true" {
		t.Errorf("allConfigs = %q, want %q: without it the server falls back to the default config and silently omits the keys of every named one", got, "true")
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
	for _, tc := range []struct {
		provider string
		summary  string
	}{
		{accessKeyProviderBuiltin, "listing builtin access keys"},
		{accessKeyProviderLDAP, "listing LDAP access keys"},
		{accessKeyProviderOpenID, "listing OpenID access keys"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			meta := accessKeysStub(t, map[string]string{}, nil)

			d := schema.TestResourceDataRaw(t, dataSourceMinioAccessKeys().Schema, map[string]interface{}{
				"identity_providers": []interface{}{tc.provider},
			})

			diags := dataSourceMinioAccessKeysRead(context.Background(), d, meta)
			if !diags.HasError() {
				t.Fatal("asking for an identity provider the server does not run must be reported, not read as an empty group")
			}
			if !strings.Contains(diags[0].Summary, tc.summary) {
				t.Errorf("summary = %q, want it to name the provider that failed", diags[0].Summary)
			}
		})
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
					testAccCheckAccessKeysCarryOnlyExpectedAttributes("data.minio_access_keys.all"),
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

func testAccCheckAccessKeysCarryOnlyExpectedAttributes(dataSourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		listing, ok := s.RootModule().Resources[dataSourceName]
		if !ok {
			return fmt.Errorf("not found: %s", dataSourceName)
		}

		checked := 0
		for attribute := range listing.Primary.Attributes {
			group, _, found := strings.Cut(attribute, ".")
			expected, isGroup := accessKeyGroupAttributes[group]
			if !found || !isGroup {
				continue
			}

			field := attribute[strings.LastIndex(attribute, ".")+1:]
			if field == "#" || field == "%" {
				continue
			}

			checked++
			if !slices.Contains(expected, field) {
				return fmt.Errorf("%s put %s in the state, which is outside the attributes this data source is meant to carry", dataSourceName, attribute)
			}
		}

		if checked == 0 {
			return fmt.Errorf("%s listed no key attributes at all, so this check proved nothing", dataSourceName)
		}

		return nil
	}
}
