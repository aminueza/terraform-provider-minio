package minio

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var testAccProviders map[string]func() (*schema.Provider, error)
var testAccProvider *schema.Provider

func init() {
	testAccProvider = Provider()
	testAccProviders = map[string]func() (*schema.Provider, error){
		"minio": func() (*schema.Provider, error) {
			return testAccProvider, nil
		},
	}
}

func TestProvider(t *testing.T) {
	if err := Provider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ *schema.Provider = Provider()
}

func testAccPreCheck(t *testing.T) {
	valid := true

	if v, _ := os.LookupEnv("TF_ACC"); v == "" {
		valid = false
	}

	if _, ok := os.LookupEnv("MINIO_ENDPOINT"); !ok {
		valid = false
	}
	if _, ok := os.LookupEnv("MINIO_USER"); !ok {
		valid = false
	}
	if _, ok := os.LookupEnv("MINIO_PASSWORD"); !ok {
		valid = false
	}
	if _, ok := os.LookupEnv("MINIO_ENABLE_HTTPS"); !ok {
		valid = false
	}

	if !valid {
		t.Fatal("you must to set env variables for integration tests!")
	}
}
