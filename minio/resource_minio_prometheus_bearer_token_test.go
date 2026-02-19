package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioPrometheusBearerToken_basic(t *testing.T) {
	resourceName := "minio_prometheus_bearer_token.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPrometheusBearerTokenBasic("cluster"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusBearerTokenExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "metric_type", "cluster"),
					resource.TestCheckResourceAttr(resourceName, "expires_in", "87600h"),
					resource.TestCheckResourceAttrSet(resourceName, "token"),
					resource.TestCheckResourceAttrSet(resourceName, "token_expiry"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token", "expires_in", "limit", "token_expiry"},
			},
		},
	})
}

func TestAccMinioPrometheusBearerToken_allMetricTypes(t *testing.T) {
	metricTypes := []string{"cluster", "node", "bucket", "resource"}

	for _, metricType := range metricTypes {
		t.Run(metricType, func(t *testing.T) {
			resourceName := "minio_prometheus_bearer_token.test"

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:          func() { testAccPreCheck(t) },
				ProviderFactories: testAccProviders,
				Steps: []resource.TestStep{
					{
						Config: testAccMinioPrometheusBearerTokenBasic(metricType),
						Check: resource.ComposeTestCheckFunc(
							testAccCheckMinioPrometheusBearerTokenExists(resourceName),
							resource.TestCheckResourceAttr(resourceName, "metric_type", metricType),
							resource.TestCheckResourceAttrSet(resourceName, "token"),
						),
					},
				},
			})
		})
	}
}

func TestAccMinioPrometheusBearerToken_update(t *testing.T) {
	resourceName := "minio_prometheus_bearer_token.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPrometheusBearerTokenBasic("cluster"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusBearerTokenExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "expires_in", "87600h"),
				),
			},
			{
				Config: testAccMinioPrometheusBearerTokenExpiresIn("cluster", "24h"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusBearerTokenExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "expires_in", "24h"),
				),
			},
		},
	})
}

func testAccCheckMinioPrometheusBearerTokenExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no bearer token ID is set")
		}

		return nil
	}
}

func testAccMinioPrometheusBearerTokenBasic(metricType string) string {
	return fmt.Sprintf(`
resource "minio_prometheus_bearer_token" "test" {
  metric_type = %q
  expires_in  = "87600h"
}
`, metricType)
}

func testAccMinioPrometheusBearerTokenExpiresIn(metricType, expiresIn string) string {
	return fmt.Sprintf(`
resource "minio_prometheus_bearer_token" "test" {
  metric_type = %q
  expires_in  = %q
}
`, metricType, expiresIn)
}
