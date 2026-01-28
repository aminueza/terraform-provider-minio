package minio

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var testAccProviders map[string]func() (*schema.Provider, error)
var testAccProvider *schema.Provider
var testAccSecondProvider *schema.Provider
var testAccThirdProvider *schema.Provider
var testAccFourthProvider *schema.Provider

func init() {
	testAccProvider = newProvider()
	testAccSecondProvider = newProvider("SECOND_")
	testAccThirdProvider = newProvider("THIRD_")
	testAccFourthProvider = newProvider("FOURTH_")
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
	valid := true

	if v, _ := os.LookupEnv("TF_ACC"); v == "" {
		valid = false
	}

	for _, envvar := range kEnvVarNeeded {
		if _, ok := os.LookupEnv(envvar); !ok {
			valid = false
			break
		}
	}

	if !valid {
		t.Fatal("you must to set env variables for integration tests!")
	}
}
