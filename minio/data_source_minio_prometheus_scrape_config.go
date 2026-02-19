package minio

import (
	"context"
	"fmt"
	"log"
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func dataSourceMinioPrometheusScrapeConfig() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceMinioPrometheusScrapeConfigRead,
		Description: "Generates Prometheus scrape configuration for MinIO metrics endpoints.",

		Schema: map[string]*schema.Schema{
			"metric_type": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringInSlice([]string{"cluster", "node", "bucket", "resource"}, false),
				Description:  "Metric type for the scrape configuration. Valid values are: cluster, node, bucket, resource",
			},
			"alias": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "minio",
				ValidateFunc: validation.StringMatch(regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`), "must start with alphanumeric and contain only alphanumeric characters, hyphens, and underscores"),
				Description:  "Alias for the MinIO server in Prometheus configuration",
			},
			"metrics_version": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "v3",
				ValidateFunc: validation.StringInSlice([]string{"v2", "v3"}, false),
				Description:  "Metrics version. Valid values are: v2, v3",
			},
			"bearer_token": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				ValidateFunc: validation.StringMatch(regexp.MustCompile(`^[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+$`), "must be a valid JWT token format"),
				Description:  "Bearer token for authenticated access to Prometheus metrics (when using JWT auth)",
			},
			"scrape_config": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Generated Prometheus scrape configuration in YAML format",
			},
			"metrics_path": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Metrics endpoint path",
			},
		},
	}
}

func dataSourceMinioPrometheusScrapeConfigRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := PrometheusScrapeConfig(d, meta)

	metricType := config.MetricType
	alias := config.Alias
	metricsVersion := config.MetricsVersion
	bearerToken := getOptionalField(d, "bearer_token", "").(string)

	log.Printf("[DEBUG] Generating Prometheus scrape config for metric type: %s", metricType)

	scheme := "http"
	if config.UseSSL {
		scheme = "https"
	}

	var metricsPath string
	if metricsVersion == "v3" {
		metricsPath = fmt.Sprintf("/minio/metrics/v3?type=%s", metricType)
	} else {
		metricsPath = fmt.Sprintf("/minio/v2/metrics/%s", metricType)
	}

	scrapeConfig := fmt.Sprintf(`scrape_configs:
  - job_name: %s
    metrics_path: %s
    scheme: %s`, alias, metricsPath, scheme)

	if bearerToken != "" {
		scrapeConfig += fmt.Sprintf(`
    bearer_token: %s`, bearerToken)
	}

	scrapeConfig += fmt.Sprintf(`
    static_configs:
      - targets: [%q]`, config.MinioEndpoint)

	if err := d.Set("scrape_config", scrapeConfig); err != nil {
		return NewResourceError("setting scrape_config", metricType, err)
	}

	if err := d.Set("metrics_path", metricsPath); err != nil {
		return NewResourceError("setting metrics_path", metricType, err)
	}

	d.SetId(metricType)

	log.Printf("[DEBUG] Generated Prometheus scrape config for metric type: %s", metricType)

	return nil
}
