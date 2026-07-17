package minio

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var testAccProviders map[string]func() (*schema.Provider, error)
var testAccProvider *schema.Provider
var testAccSecondProvider *schema.Provider
var testAccThirdProvider *schema.Provider
var testAccFourthProvider *schema.Provider
var testAccKmsProvider *schema.Provider
var testAccLdapProvider *schema.Provider

func init() {
	testAccProvider = newProvider()
	testAccSecondProvider = newProvider("SECOND_")
	testAccThirdProvider = newProvider("THIRD_")
	testAccFourthProvider = newProvider("FOURTH_")
	testAccKmsProvider = newProvider("KMS_")
	testAccLdapProvider = newProvider("LDAP_")
	testAccProviders = map[string]func() (*schema.Provider, error){
		"minio": func() (*schema.Provider, error) {
			return testAccProvider, nil
		},
		"secondminio": func() (*schema.Provider, error) {
			return testAccSecondProvider, nil
		},
		"thirdminio": func() (*schema.Provider, error) {
			return testAccThirdProvider, nil
		},
		"fourthminio": func() (*schema.Provider, error) {
			return testAccFourthProvider, nil
		},
		"kmsminio": func() (*schema.Provider, error) {
			return testAccKmsProvider, nil
		},
		"ldapminio": func() (*schema.Provider, error) {
			return testAccLdapProvider, nil
		},
	}
}

func TestProvider(t *testing.T) {
	if err := newProvider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ = newProvider()
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
	if v, _ := os.LookupEnv("TF_ACC"); v == "" {
		t.Fatal("TF_ACC must be set for acceptance tests")
	}

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

// testAccEndpoint returns the endpoint (host:port) of the MinIO test instance
// configured by the given env var prefix ("", "SECOND_", "THIRD_", ...).
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
