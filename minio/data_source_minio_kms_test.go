package minio

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestDataSourceMinioKMS_schemas(t *testing.T) {
	if dataSourceMinioKMSStatus() == nil {
		t.Fatal("minio_kms_status: nil schema")
	}
	if dataSourceMinioKMSMetrics() == nil {
		t.Fatal("minio_kms_metrics: nil schema")
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
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckKMS(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `data "minio_kms_status" "kms" { provider = kmsminio }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_kms_status.kms", "id"),
					resource.TestCheckResourceAttrSet("data.minio_kms_status.kms", "default_key_id"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioKMSMetrics_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckKMS(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `data "minio_kms_metrics" "kms" { provider = kmsminio }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_kms_metrics.kms", "id"),
				),
			},
		},
	})
}
