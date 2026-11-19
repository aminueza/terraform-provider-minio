package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// Flipping tls and tls_skip_verify from true to false must write
// tls=off and tls_skip_verify=off to the server; the old code only wrote
// the two fields when they were true, and the server kept their old
// value. The target is disabled, so no broker is needed.
// See issue #1331.
func TestAccMinioAuditKafka_tlsOff(t *testing.T) {
	resourceName := "minio_audit_kafka.test"
	name := "tfacc-" + acctest.RandString(6)

	// Sequential on purpose: the server check restarts the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("audit_kafka", name),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioAuditKafkaTlsConfig(name, true),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					resource.TestCheckResourceAttr(resourceName, "tls", "true"),
					resource.TestCheckResourceAttr(resourceName, "tls_skip_verify", "true"),
					testAccCheckServerStoredTarget("audit_kafka", name, map[string]string{
						"tls":             "on",
						"tls_skip_verify": "on",
						"enable":          "off",
					}),
				),
			},
			{
				Config: testAccMinioAuditKafkaTlsConfig(name, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "tls", "false"),
					resource.TestCheckResourceAttr(resourceName, "tls_skip_verify", "false"),
					testAccCheckServerStoredTarget("audit_kafka", name, map[string]string{
						"tls":             "off",
						"tls_skip_verify": "off",
						"enable":          "off",
					}),
				),
			},
		},
	})
}

func testAccMinioAuditKafkaTlsConfig(name string, tls bool) string {
	return fmt.Sprintf(`
resource "minio_audit_kafka" "test" {
  name            = %[1]q
  brokers         = "localhost:9092"
  topic           = "minio-audit"
  enable          = false
  tls             = %[2]t
  tls_skip_verify = %[2]t
}
`, name, tls)
}
