package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioPrometheusScrapeConfig_basic(t *testing.T) {
	resourceName := "data.minio_prometheus_scrape_config.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      nil,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPrometheusScrapeConfigBasic("cluster"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusScrapeConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "metric_type", "cluster"),
					resource.TestCheckResourceAttr(resourceName, "alias", "minio"),
					resource.TestCheckResourceAttr(resourceName, "metrics_version", "v3"),
					resource.TestCheckResourceAttrSet(resourceName, "scrape_config"),
					resource.TestCheckResourceAttrSet(resourceName, "metrics_path"),
				),
			},
		},
	})
}

func TestAccMinioPrometheusScrapeConfig_allMetricTypes(t *testing.T) {
	metricTypes := []string{"cluster", "node", "bucket", "resource"}

	for _, metricType := range metricTypes {
		t.Run(metricType, func(t *testing.T) {
			resourceName := "data.minio_prometheus_scrape_config.test"

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:          func() { testAccPreCheck(t) },
				ProviderFactories: testAccProviders,
				CheckDestroy:      nil,
				Steps: []resource.TestStep{
					{
						Config: testAccMinioPrometheusScrapeConfigBasic(metricType),
						Check: resource.ComposeTestCheckFunc(
							testAccCheckMinioPrometheusScrapeConfigExists(resourceName),
							resource.TestCheckResourceAttr(resourceName, "metric_type", metricType),
							resource.TestCheckResourceAttrSet(resourceName, "scrape_config"),
						),
					},
				},
			})
		})
	}
}

func TestAccMinioPrometheusScrapeConfig_v2(t *testing.T) {
	resourceName := "data.minio_prometheus_scrape_config.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      nil,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPrometheusScrapeConfigVersion("cluster", "v2"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusScrapeConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "metric_type", "cluster"),
					resource.TestCheckResourceAttr(resourceName, "metrics_version", "v2"),
					resource.TestCheckResourceAttr(resourceName, "metrics_path", "/minio/v2/metrics/cluster"),
				),
			},
		},
	})
}

func TestAccMinioPrometheusScrapeConfig_customAlias(t *testing.T) {
	resourceName := "data.minio_prometheus_scrape_config.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      nil,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPrometheusScrapeConfigAlias("cluster", "my-minio-cluster"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusScrapeConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "metric_type", "cluster"),
					resource.TestCheckResourceAttr(resourceName, "alias", "my-minio-cluster"),
				),
			},
		},
	})
}

func testAccCheckMinioPrometheusScrapeConfigExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no scrape config ID is set")
		}

		return nil
	}
}

func testAccMinioPrometheusScrapeConfigBasic(metricType string) string {
	return fmt.Sprintf(`
data "minio_prometheus_scrape_config" "test" {
  metric_type     = %q
  alias           = "minio"
  metrics_version = "v3"
}
`, metricType)
}

func testAccMinioPrometheusScrapeConfigVersion(metricType, version string) string {
	return fmt.Sprintf(`
data "minio_prometheus_scrape_config" "test" {
  metric_type     = %q
  alias           = "minio"
  metrics_version = %q
}
`, metricType, version)
}

func testAccMinioPrometheusScrapeConfigAlias(metricType, alias string) string {
	return fmt.Sprintf(`
data "minio_prometheus_scrape_config" "test" {
  metric_type     = %q
  alias           = %q
  metrics_version = "v3"
}
`, metricType, alias)
}
