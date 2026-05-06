package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioKMSKey_basic(t *testing.T) {
	keyID := fmt.Sprintf("tfacc-kms-key-%d", acctest.RandInt())
	resourceName := "minio_kms_key.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioKMSKeyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioKMSKeyConfig(keyID),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioKMSKeyExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key_id", keyID),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckMinioKMSKeyDestroy(s *terraform.State) error {
	conn := testAccProvider.Meta().(*S3MinioClient).S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_kms_key" {
			continue
		}

		_, err := conn.GetKeyStatus(context.Background(), rs.Primary.ID)
		if err != nil {
			return nil
		}

		return fmt.Errorf("KMS key %s still exists", rs.Primary.ID)
	}

	return nil
}

func testAccCheckMinioKMSKeyExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no KMS key ID is set")
		}

		conn := testAccProvider.Meta().(*S3MinioClient).S3Admin

		status, err := conn.GetKeyStatus(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error reading KMS key: %w", err)
		}

		if status.KeyID != rs.Primary.ID {
			return fmt.Errorf("KMS key not found")
		}

		return nil
	}
}

func testAccMinioKMSKeyConfig(keyID string) string {
	return fmt.Sprintf(`
resource "minio_kms_key" "test" {
  key_id = "%s"
}
`, keyID)
}
