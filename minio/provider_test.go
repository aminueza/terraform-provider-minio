package minio

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var testAccProviders map[string]*schema.Provider
var testAccProvider *schema.Provider

func init() {
	testAccProvider = Provider()
	testAccProviders = map[string]*schema.Provider{
		"minio": testAccProvider,
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
	ok := os.Getenv("TF_ACC") == ""

	if os.Getenv("MINIO_ENDPOINT") != "" {
		ok = true
	}
	if os.Getenv("MINIO_ACCESS_KEY") != "" {
		ok = true
	}
	if os.Getenv("MINIO_SECRET_KEY") != "" {
		ok = true
	}
	if os.Getenv("MINIO_ENABLE_HTTPS") != "" {
		ok = true
	}
	if !ok {
		panic("you must to set env variables for integration tests!")
	}
}
