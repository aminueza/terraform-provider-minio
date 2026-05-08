package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestResourceMinioIAMImport_schema(t *testing.T) {
	if err := resourceMinioIAMImport().InternalValidate(nil, true); err != nil {
		t.Fatalf("minio_iam_import schema invalid: %v", err)
	}
}

func TestAccResourceMinioIAMImport_roundTrip(t *testing.T) {
	const importName = "minio_iam_import.restore"
	policyName := acctest.RandomWithPrefix("tfacc-iam-import")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMImportRoundTripConfig(policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(importName, "id"),
					resource.TestCheckResourceAttrSet(importName, "sha256"),
				),
				// MinIO's zip export embeds non-deterministic metadata, so
				// chaining data.minio_iam_export -> minio_iam_import in the
				// same apply produces different bytes on each refresh. The
				// import is idempotent server-side; users wanting stable
				// plans should put export and import in separate states or
				// pin iam_data via lifecycle.ignore_changes.
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccIAMImportRoundTripConfig(policyName string) string {
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

data "minio_iam_export" "snapshot" {
  depends_on = [minio_iam_policy.seed]
}

resource "minio_iam_import" "restore" {
  iam_data = data.minio_iam_export.snapshot.iam_data
}
`, policyName)
}
