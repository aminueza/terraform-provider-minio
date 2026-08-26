package minio

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// testAccProviders hands every test its own provider instances. Tests
// configure the providers they are given, and a test that configures one with
// invalid credentials (TestAccMinioS3Bucket_CredentialErrorDoesNotRemoveFromState)
// must not be able to reach a provider another test is running against, so no
// entry may return a shared instance.
var testAccProviders = map[string]func() (*schema.Provider, error){
	"minio": func() (*schema.Provider, error) {
		return newProvider(), nil
	},
	"secondminio": func() (*schema.Provider, error) {
		return newProvider("SECOND_"), nil
	},
	"thirdminio": func() (*schema.Provider, error) {
		return newProvider("THIRD_"), nil
	},
	"fourthminio": func() (*schema.Provider, error) {
		return newProvider("FOURTH_"), nil
	},
	"kmsminio": func() (*schema.Provider, error) {
		return newProvider("KMS_"), nil
	},
	"ldapminio": func() (*schema.Provider, error) {
		return newProvider("LDAP_"), nil
	},
}

type testAccClientResult struct {
	client *S3MinioClient
	err    error
}

var (
	testAccClientsMu sync.Mutex
	testAccClients   = map[string]testAccClientResult{}
)

// testAccClientForPrefix builds the client for the MinIO instance behind the
// given env var prefix ("", "SECOND_", "THIRD_", ...), once per prefix. It
// goes through the provider's own configuration path, so endpoint, TLS and
// credential handling match a real provider run. The clients belong to the
// check helpers alone: no test is handed one, so no test can reconfigure them.
func testAccClientForPrefix(prefix string) (*S3MinioClient, error) {
	testAccClientsMu.Lock()
	defer testAccClientsMu.Unlock()

	if result, ok := testAccClients[prefix]; ok {
		return result.client, result.err
	}

	var result testAccClientResult
	p := newProvider(prefix)
	if diags := p.Configure(context.Background(), terraform.NewResourceConfigRaw(nil)); diags.HasError() {
		result.err = fmt.Errorf("configuring the %sMINIO_* test client: %s", prefix, firstDiagnosticError(diags))
	} else if client, ok := p.Meta().(*S3MinioClient); ok {
		result.client = client
	} else {
		result.err = fmt.Errorf("configuring the %sMINIO_* test client: provider meta is %T, want *S3MinioClient", prefix, p.Meta())
	}

	testAccClients[prefix] = result
	return result.client, result.err
}

func firstDiagnosticError(diags diag.Diagnostics) string {
	for _, d := range diags {
		if d.Severity == diag.Error {
			if d.Detail == "" {
				return d.Summary
			}
			return d.Summary + ": " + d.Detail
		}
	}
	return "unknown error"
}

// testAccClient returns the client for the primary test instance, and
// testAccSecondClient, testAccThirdClient and testAccFourthClient the ones for
// the SECOND_, THIRD_ and FOURTH_ instances. testAccPreCheck already requires
// the environment for all four, so a failure here means the suite cannot run
// at all.
func testAccClient() *S3MinioClient       { return testAccMustClient("") }
func testAccSecondClient() *S3MinioClient { return testAccMustClient("SECOND_") }
func testAccThirdClient() *S3MinioClient  { return testAccMustClient("THIRD_") }
func testAccFourthClient() *S3MinioClient { return testAccMustClient("FOURTH_") }

// testAccKmsClient and testAccLdapClient return an error instead of stopping
// the run: their instances are optional, and the suites that use them skip
// themselves when the environment is missing.
func testAccKmsClient() (*S3MinioClient, error)  { return testAccClientForPrefix("KMS_") }
func testAccLdapClient() (*S3MinioClient, error) { return testAccClientForPrefix("LDAP_") }

func testAccMustClient(prefix string) *S3MinioClient {
	client, err := testAccClientForPrefix(prefix)
	if err != nil {
		panic(err.Error())
	}
	return client
}

// testAccStateUsesProvider reports whether the state holds a resource managed
// by the given provider type, e.g. "secondminio". Check helpers use it to stay
// off the instances a test never touched.
func testAccStateUsesProvider(s *terraform.State, providerType string) bool {
	for _, rs := range s.RootModule().Resources {
		if rs.Provider == providerType || strings.HasSuffix(rs.Provider, "/"+providerType) {
			return true
		}
	}
	return false
}

func TestProvider(t *testing.T) {
	if err := newProvider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ = newProvider()
}

func TestProviderFactories_freshInstancePerCall(t *testing.T) {
	for name, factory := range testAccProviders {
		first, err := factory()
		if err != nil {
			t.Fatalf("calling the %q factory: %s", name, err)
		}

		second, err := factory()
		if err != nil {
			t.Fatalf("calling the %q factory: %s", name, err)
		}

		if first == second {
			t.Fatalf("the %q factory returned the same provider twice; configuring it in one test would reach every test running alongside it", name)
		}
	}
}

func TestProviderFactories_configureStaysWithinOneInstance(t *testing.T) {
	factory := testAccProviders["minio"]

	valid, err := factory()
	if err != nil {
		t.Fatalf("calling the %q factory: %s", "minio", err)
	}
	if diags := valid.Configure(context.Background(), terraform.NewResourceConfigRaw(testAccProviderConfigRaw("right-password"))); diags.HasError() {
		t.Fatalf("configuring with valid credentials: %s", firstDiagnosticError(diags))
	}

	invalid, err := factory()
	if err != nil {
		t.Fatalf("calling the %q factory: %s", "minio", err)
	}
	if diags := invalid.Configure(context.Background(), terraform.NewResourceConfigRaw(testAccProviderConfigRaw("wrong-password"))); diags.HasError() {
		t.Fatalf("configuring with invalid credentials: %s", firstDiagnosticError(diags))
	}

	if client, ok := invalid.Meta().(*S3MinioClient); !ok || client.S3UserSecret != "wrong-password" {
		t.Fatal("the second provider did not take the invalid password, so this test would pass for the wrong reason")
	}

	client, ok := valid.Meta().(*S3MinioClient)
	if !ok {
		t.Fatalf("provider meta is %T, want *S3MinioClient", valid.Meta())
	}
	if client.S3UserSecret != "right-password" {
		t.Fatalf("configuring the second provider reached the first one's client: its secret is now %q", client.S3UserSecret)
	}
}

// s3_compat_mode keeps client creation from probing the server for its
// edition, so the two tests above need no MinIO instance.
func testAccProviderConfigRaw(password string) map[string]interface{} {
	return map[string]interface{}{
		"minio_server":   "127.0.0.1:9000",
		"minio_user":     "minio",
		"minio_password": password,
		"s3_compat_mode": true,
	}
}

var kEnvVarNeeded = []string{
	"MINIO_ENDPOINT",
	"MINIO_USER",
	"MINIO_PASSWORD",
	"MINIO_ENABLE_HTTPS",
	"SECOND_MINIO_ENDPOINT",
	"SECOND_MINIO_USER",
	"SECOND_MINIO_PASSWORD",
	"SECOND_MINIO_ENABLE_HTTPS",
	"THIRD_MINIO_ENDPOINT",
	"THIRD_MINIO_USER",
	"THIRD_MINIO_PASSWORD",
	"THIRD_MINIO_ENABLE_HTTPS",
	"FOURTH_MINIO_ENDPOINT",
	"FOURTH_MINIO_USER",
	"FOURTH_MINIO_PASSWORD",
	"FOURTH_MINIO_ENABLE_HTTPS",
}

func testAccPreCheck(t *testing.T) {
	var missing []string
	for _, envvar := range kEnvVarNeeded {
		if _, ok := os.LookupEnv(envvar); !ok {
			missing = append(missing, envvar)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("missing environment variables for acceptance tests: %s (see the \"test\" service in docker-compose.yml for the full set)", strings.Join(missing, ", "))
	}
}

func testAccEndpoint(prefix string) string {
	return os.Getenv(prefix + "MINIO_ENDPOINT")
}

// testAccEndpointURL returns the endpoint of the MinIO test instance as a URL,
// with the scheme derived from <prefix>MINIO_ENABLE_HTTPS.
func testAccEndpointURL(prefix string) string {
	scheme := "http"
	if enabled, _ := strconv.ParseBool(os.Getenv(prefix + "MINIO_ENABLE_HTTPS")); enabled {
		scheme = "https"
	}
	return scheme + "://" + testAccEndpoint(prefix)
}
