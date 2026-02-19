package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioPrometheusConfig_basic(t *testing.T) {
	resourceName := "minio_prometheus_config.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioPrometheusConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPrometheusConfigConfigBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "auth_type", "jwt"),
					resource.TestCheckResourceAttr(resourceName, "metrics_version", "v3"),
					resource.TestCheckResourceAttr(resourceName, "generate_tokens", "false"),
					resource.TestCheckResourceAttrSet(resourceName, "restart_required"),
				),
			},
			{
				Config: testAccMinioPrometheusConfigConfigUpdated(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "auth_type", "public"),
					resource.TestCheckResourceAttr(resourceName, "metrics_version", "v2"),
					resource.TestCheckResourceAttr(resourceName, "prometheus_url", "https://prometheus.example.com"),
					resource.TestCheckResourceAttr(resourceName, "job_id", "minio-metrics"),
					resource.TestCheckResourceAttr(resourceName, "generate_tokens", "false"),
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

func TestAccMinioPrometheusConfig_generateTokens(t *testing.T) {
	resourceName := "minio_prometheus_config.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioPrometheusConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPrometheusConfigConfigWithTokens(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "auth_type", "jwt"),
					resource.TestCheckResourceAttr(resourceName, "metrics_version", "v3"),
					resource.TestCheckResourceAttr(resourceName, "generate_tokens", "true"),
					resource.TestCheckResourceAttrSet(resourceName, "cluster_token"),
					resource.TestCheckResourceAttrSet(resourceName, "node_token"),
					resource.TestCheckResourceAttrSet(resourceName, "bucket_token"),
					resource.TestCheckResourceAttrSet(resourceName, "resource_token"),
				),
			},
		},
	})
}

func TestAccMinioPrometheusConfig_publicAuth(t *testing.T) {
	resourceName := "minio_prometheus_config.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioPrometheusConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPrometheusConfigConfigPublic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "auth_type", "public"),
					resource.TestCheckResourceAttr(resourceName, "metrics_version", "v3"),
					resource.TestCheckResourceAttr(resourceName, "generate_tokens", "false"),
				),
			},
		},
	})
}

func testAccCheckMinioPrometheusConfigDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_prometheus_config" {
			continue
		}

		client := testAccProvider.Meta().(*S3MinioClient)
		configData, err := client.S3Admin.GetConfigKV(context.Background(), "prometheus")
		if err == nil && len(configData) > 0 {
			return fmt.Errorf("prometheus config still exists")
		}
	}

	return nil
}

func testAccCheckMinioPrometheusConfigExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No prometheus config ID is set")
		}

		client := testAccProvider.Meta().(*S3MinioClient)
		configData, err := client.S3Admin.GetConfigKV(context.Background(), "prometheus")
		if err != nil {
			return fmt.Errorf("Error getting prometheus config: %v", err)
		}

		if len(configData) == 0 {
			return fmt.Errorf("Prometheus config not found")
		}

		return nil
	}
}

func testAccMinioPrometheusConfigConfigBasic() string {
	return `
resource "minio_prometheus_config" "test" {
  auth_type       = "jwt"
  metrics_version = "v3"
  generate_tokens = false
}
`
}

func testAccMinioPrometheusConfigConfigUpdated() string {
	return `
resource "minio_prometheus_config" "test" {
  auth_type        = "public"
  metrics_version  = "v2"
  prometheus_url   = "https://prometheus.example.com"
  job_id           = "minio-metrics"
  generate_tokens  = false
}
`
}

func testAccMinioPrometheusConfigConfigWithTokens() string {
	return `
resource "minio_prometheus_config" "test" {
  auth_type       = "jwt"
  metrics_version = "v3"
  generate_tokens = true
}
`
}

func testAccMinioPrometheusConfigConfigPublic() string {
	return `
resource "minio_prometheus_config" "test" {
  auth_type       = "public"
  metrics_version = "v3"
  generate_tokens = false
}
`
}
