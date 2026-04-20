package minio

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestDataSourceMinioKMS_schemas(t *testing.T) {
	if err := dataSourceMinioKMSStatus().InternalValidate(nil, false); err != nil {
		t.Fatalf("minio_kms_status schema invalid: %v", err)
	}
	if err := dataSourceMinioKMSMetrics().InternalValidate(nil, false); err != nil {
		t.Fatalf("minio_kms_metrics schema invalid: %v", err)
	}
}

func testAccPreCheckKMS(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)
	if os.Getenv("MINIO_KMS_CONFIGURED") == "" {
		t.Skip("MINIO_KMS_CONFIGURED not set; skipping KMS tests")
	}
}

func TestAccDataSourceMinioKMSStatus_basic(t *testing.T) {
	const name = "data.minio_kms_status.kms"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckKMS(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `data "minio_kms_status" "kms" { provider = kmsminio }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(name, "id"),
					resource.TestCheckResourceAttrSet(name, "default_key_id"),
					resource.TestCheckResourceAttr(name, "state.#", "1"),
					resource.TestCheckResourceAttrSet(name, "state.0.key_store_reachable"),
					resource.TestCheckResourceAttrSet(name, "state.0.uptime_seconds"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioKMSMetrics_basic(t *testing.T) {
	const name = "data.minio_kms_metrics.kms"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckKMS(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `data "minio_kms_metrics" "kms" { provider = kmsminio }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(name, "id"),
					resource.TestCheckResourceAttrSet(name, "request_ok"),
					resource.TestCheckResourceAttrSet(name, "uptime_seconds"),
					resource.TestCheckResourceAttrSet(name, "cpus"),
				),
			},
		},
	})
}
