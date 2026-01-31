package minio

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioHealthStatus() *schema.Resource {
	return &schema.Resource{
		Description:        "Checks MinIO cluster health using unauthenticated health endpoints.",
		ReadWithoutTimeout: dataSourceMinioHealthStatusRead,
		Schema: map[string]*schema.Schema{
			"id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Timestamp-based identifier for this health check",
			},
			"live": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Server liveness status (/minio/health/live)",
			},
			"ready": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Server readiness status (/minio/health/ready)",
			},
			"write_quorum": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Write quorum health status (/minio/health/cluster)",
			},
			"read_quorum": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Read quorum health status (/minio/health/cluster/read)",
			},
			"safe_for_maintenance": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether it's safe to perform maintenance without losing quorum (/minio/health/cluster?maintenance=true)",
			},
			"healthy": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Overall health status (true if all checks pass)",
			},
		},
	}
}

func dataSourceMinioHealthStatusRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	// Get provider configuration
	m := meta.(*S3MinioClient)

	// Get endpoint URL from the S3 client
	endpointURL := m.S3Client.EndpointURL()
	baseURL := fmt.Sprintf("%s://%s", endpointURL.Scheme, endpointURL.Host)

	// Create HTTP client with timeout
	// Health endpoints are unauthenticated, so we use a simple client
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Check each health endpoint
	live, err := checkHealthEndpoint(ctx, client, baseURL, "/minio/health/live")
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to check liveness endpoint: %w", err))
	}

	ready, err := checkHealthEndpoint(ctx, client, baseURL, "/minio/health/ready")
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to check readiness endpoint: %w", err))
	}

	writeQuorum, err := checkHealthEndpoint(ctx, client, baseURL, "/minio/health/cluster")
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to check write quorum endpoint: %w", err))
	}

	readQuorum, err := checkHealthEndpoint(ctx, client, baseURL, "/minio/health/cluster/read")
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to check read quorum endpoint: %w", err))
	}

	// For safe_for_maintenance: 200 = safe to perform maintenance, 412 = not safe (would lose quorum)
	safeForMaintenance, err := checkMaintenanceEndpoint(ctx, client, baseURL, "/minio/health/cluster?maintenance=true")
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to check maintenance safety endpoint: %w", err))
	}

	// Overall health is true only if all checks pass
	// Note: safe_for_maintenance doesn't affect overall health - it's informational
	healthy := live && ready && writeQuorum && readQuorum

	// Set resource attributes
	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

	if err := d.Set("live", live); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("ready", ready); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("write_quorum", writeQuorum); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("read_quorum", readQuorum); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("safe_for_maintenance", safeForMaintenance); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("healthy", healthy); err != nil {
		return diag.FromErr(err)
	}

	return diags
}

// checkHealthEndpoint makes a GET request to a health endpoint and returns true if healthy (200), false if unhealthy (503)
func checkHealthEndpoint(ctx context.Context, client *http.Client, baseURL, path string) (bool, error) {
	url := baseURL + path

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to make request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusServiceUnavailable:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, url)
	}
}

// checkMaintenanceEndpoint checks if it's safe to perform maintenance on the cluster
// Returns true if safe to perform maintenance (200), false if not safe/would lose quorum (412)
func checkMaintenanceEndpoint(ctx context.Context, client *http.Client, baseURL, path string) (bool, error) {
	url := baseURL + path

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to make request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// 200 means it's safe to perform maintenance (sufficient nodes online)
		return true, nil
	case http.StatusPreconditionFailed:
		// 412 means not safe to perform maintenance (would lose quorum)
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, url)
	}
}
