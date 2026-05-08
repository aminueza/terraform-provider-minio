package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestDataSourceMinioIAMExport_schema(t *testing.T) {
	if err := dataSourceMinioIAMExport().InternalValidate(nil, false); err != nil {
		t.Fatalf("minio_iam_export schema invalid: %v", err)
	}
}

func TestAccDataSourceMinioIAMExport_basic(t *testing.T) {
	const name = "data.minio_iam_export.export"
	policyName := acctest.RandomWithPrefix("tfacc-iam-export")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMExportConfig(policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(name, "id"),
					resource.TestCheckResourceAttrSet(name, "iam_data"),
					resource.TestCheckResourceAttrSet(name, "sha256"),
					resource.TestCheckResourceAttrSet(name, "size_bytes"),
				),
			},
		},
	})
}

func testAccIAMExportConfig(policyName string) string {
	return fmt.Sprintf(`
resource "minio_iam_policy" "seed" {
  name   = %[1]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:ListBucket"]
      Resource = ["arn:aws:s3:::*"]
    }]
  })
}

data "minio_iam_export" "export" {
  depends_on = [minio_iam_policy.seed]
}
`, policyName)
}
