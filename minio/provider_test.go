package minio

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"
)

var testAccProviders map[string]terraform.ResourceProvider
var testAccProvider *schema.Provider

func init() {
	testAccProvider = Provider().(*schema.Provider)
	testAccProviders = map[string]terraform.ResourceProvider{
		"minio": testAccProvider,
	}
}

func TestProvider(t *testing.T) {
	if err := Provider().(*schema.Provider).InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ terraform.ResourceProvider = Provider()
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
