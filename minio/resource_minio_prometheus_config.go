package minio

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceMinioPrometheusConfig() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreatePrometheusConfig,
		ReadContext:   minioReadPrometheusConfig,
		UpdateContext: minioUpdatePrometheusConfig,
		DeleteContext: minioDeletePrometheusConfig,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Read:   schema.DefaultTimeout(2 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},
		Description: `Manages MinIO Prometheus configuration for metrics collection.

This resource configures how MinIO exposes Prometheus metrics, including
authentication type, metrics version, and endpoint settings. The configuration
is stored in MinIO's server configuration and affects all metrics endpoints.

Note: Changes to Prometheus configuration may require a MinIO server restart
to take effect, depending on the specific settings being modified.`,

		Schema: map[string]*schema.Schema{
			"auth_type": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "jwt",
				ValidateFunc: validation.StringInSlice([]string{"jwt", "public"}, false),
				Description:  "Authentication type for Prometheus metrics. Valid values: jwt (default), public",
			},
			"metrics_version": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "v3",
				ValidateFunc: validation.StringInSlice([]string{"v2", "v3"}, false),
				Description:  "Metrics version to expose. Valid values: v2, v3 (default)",
			},
			"prometheus_url": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Custom Prometheus metrics endpoint URL. If not specified, uses default endpoint",
			},
			"job_id": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Custom job ID for Prometheus metrics. If not specified, uses default",
			},
			"generate_tokens": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Whether to automatically generate bearer tokens for all metric types when auth_type is jwt",
			},
			"cluster_token": {
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
				Description: "Generated bearer token for cluster metrics (when generate_tokens is true)",
			},
			"node_token": {
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
				Description: "Generated bearer token for node metrics (when generate_tokens is true)",
			},
			"bucket_token": {
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
				Description: "Generated bearer token for bucket metrics (when generate_tokens is true)",
			},
			"resource_token": {
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
				Description: "Generated bearer token for resource metrics (when generate_tokens is true)",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether the configuration change requires a MinIO server restart",
			},
		},
	}
}

func minioCreatePrometheusConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := PrometheusConfig(d, meta)

	log.Printf("[DEBUG] Creating Prometheus config with auth_type: %s, metrics_version: %s", config.AuthType, config.MetricsVersion)

	// Build the Prometheus configuration string
	configStr := buildPrometheusConfigString(config)

	// Set the Prometheus configuration using the admin API
	timeout := d.Timeout(schema.TimeoutCreate)
	var restartRequired bool
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		restart, err := config.MinioAdmin.SetConfigKV(ctx, configStr)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error setting prometheus config: %w", err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to set prometheus config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		return NewResourceError("creating prometheus config", "prometheus", err)
	}

	// Set the resource ID
	d.SetId("prometheus")

	// Generate tokens if requested
	if config.GenerateTokens && config.AuthType == "jwt" {
		if err := generatePrometheusTokens(ctx, d, meta); err != nil {
			return diag.FromErr(err)
		}
	}

	_ = d.Set("restart_required", restartRequired)

	if restartRequired {
		log.Printf("[WARN] Prometheus config change requires MinIO server restart to take effect")
	}

	log.Printf("[DEBUG] Created Prometheus config successfully")

	return minioReadPrometheusConfig(ctx, d, meta)
}

func minioReadPrometheusConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)

	log.Printf("[DEBUG] Reading Prometheus config")

	timeout := d.Timeout(schema.TimeoutRead)
	var configData []byte
	var err error

	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		configData, err = client.S3Admin.GetConfigKV(ctx, "prometheus")
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
				log.Printf("[WARN] Prometheus config not found, resource may have been removed")
				d.SetId("")
				return nil
			}

			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("error reading prometheus config: %w", err))
			}

			return retry.NonRetryableError(fmt.Errorf("failed to get prometheus config: %w", err))
		}
		return nil
	})

	if err != nil {
		return NewResourceError("reading prometheus config", "prometheus", err)
	}

	// If the resource was removed
	if d.Id() == "" {
		return nil
	}

	// Parse the Prometheus configuration
	parsedConfig, err := parsePrometheusConfig(string(configData))
	if err != nil {
		return NewResourceError("parsing prometheus config", "prometheus", err)
	}

	// Set the parsed values
	if err := d.Set("auth_type", parsedConfig.AuthType); err != nil {
		return NewResourceError("setting auth_type", "prometheus", err)
	}
	if err := d.Set("metrics_version", parsedConfig.MetricsVersion); err != nil {
		return NewResourceError("setting metrics_version", "prometheus", err)
	}
	if err := d.Set("prometheus_url", parsedConfig.PrometheusURL); err != nil {
		return NewResourceError("setting prometheus_url", "prometheus", err)
	}
	if err := d.Set("job_id", parsedConfig.JobID); err != nil {
		return NewResourceError("setting job_id", "prometheus", err)
	}

	return nil
}

func minioUpdatePrometheusConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := PrometheusConfig(d, meta)

	log.Printf("[DEBUG] Updating Prometheus config")

	// Build the Prometheus configuration string
	configStr := buildPrometheusConfigString(config)

	// Update the Prometheus configuration
	timeout := d.Timeout(schema.TimeoutUpdate)
	var restartRequired bool
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		restart, err := config.MinioAdmin.SetConfigKV(ctx, configStr)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error updating prometheus config: %w", err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to update prometheus config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		return NewResourceError("updating prometheus config", "prometheus", err)
	}

	// Regenerate tokens if requested and auth type changed to jwt
	if config.GenerateTokens && config.AuthType == "jwt" {
		if err := generatePrometheusTokens(ctx, d, meta); err != nil {
			return diag.FromErr(err)
		}
	}

	_ = d.Set("restart_required", restartRequired)

	if restartRequired {
		log.Printf("[WARN] Prometheus config update requires MinIO server restart to take effect")
	}

	log.Printf("[DEBUG] Updated Prometheus config successfully")

	return minioReadPrometheusConfig(ctx, d, meta)
}

func minioDeletePrometheusConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)

	log.Printf("[DEBUG] Deleting Prometheus config")

	timeout := d.Timeout(schema.TimeoutDelete)
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		_, err := client.S3Admin.DelConfigKV(ctx, "prometheus")
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error deleting prometheus config: %w", err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to delete prometheus config: %w", err))
		}
		return nil
	})

	if err != nil {
		return NewResourceError("deleting prometheus config", "prometheus", err)
	}

	d.SetId("")
	log.Printf("[DEBUG] Deleted Prometheus config successfully")

	return nil
}

func buildPrometheusConfigString(config *S3MinioPrometheusConfig) string {
	var configParts []string

	// Add auth type
	configParts = append(configParts, fmt.Sprintf("auth_type=%s", config.AuthType))

	// Add metrics version
	configParts = append(configParts, fmt.Sprintf("metrics_version=%s", config.MetricsVersion))

	// Add custom URL if specified
	if config.PrometheusURL != "" {
		configParts = append(configParts, fmt.Sprintf("url=%s", config.PrometheusURL))
	}

	// Add job ID if specified
	if config.JobID != "" {
		configParts = append(configParts, fmt.Sprintf("job_id=%s", config.JobID))
	}

	return fmt.Sprintf("prometheus %s", strings.Join(configParts, " "))
}

func parsePrometheusConfig(configData string) (*S3MinioPrometheusConfig, error) {
	config := &S3MinioPrometheusConfig{
		AuthType:       "jwt", // default
		MetricsVersion: "v3",  // default
		PrometheusURL:  "",    // default (empty)
		JobID:          "",    // default (empty)
	}

	// Remove "prometheus" prefix if present
	configData = strings.TrimSpace(configData)
	if strings.HasPrefix(configData, "prometheus ") {
		configData = strings.TrimSpace(strings.TrimPrefix(configData, "prometheus "))
	}

	// Parse key=value pairs
	parts := strings.Fields(configData)
	for _, part := range parts {
		if strings.Contains(part, "=") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				key := strings.TrimSpace(kv[0])
				value := strings.TrimSpace(kv[1])

				switch key {
				case "auth_type":
					config.AuthType = value
				case "metrics_version":
					config.MetricsVersion = value
				case "url":
					config.PrometheusURL = value
				case "job_id":
					config.JobID = value
				}
			}
		}
	}

	return config, nil
}

func generatePrometheusTokens(ctx context.Context, d *schema.ResourceData, meta interface{}) error {
	config := PrometheusConfig(d, meta)
	metricTypes := []string{"cluster", "node", "bucket", "resource"}
	tokenFields := map[string]string{
		"cluster":  "cluster_token",
		"node":     "node_token",
		"bucket":   "bucket_token",
		"resource": "resource_token",
	}

	for _, metricType := range metricTypes {
		// Generate token with default expiry
		duration, err := time.ParseDuration("87600h") // 10 years default
		if err != nil {
			return fmt.Errorf("parsing default token duration: %w", err)
		}

		token, err := generatePrometheusToken(config.MinioAccessKey, config.MinioSecretKey, duration, 876000)
		if err != nil {
			return fmt.Errorf("generating %s token: %w", metricType, err)
		}

		if err := d.Set(tokenFields[metricType], token); err != nil {
			return fmt.Errorf("setting %s token: %w", metricType, err)
		}
	}

	return nil
}
