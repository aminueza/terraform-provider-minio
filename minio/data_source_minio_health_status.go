package minio

import (
	"context"
	"fmt"
	"log"
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
				Description: "Timestamp ID for this health check",
			},
			"live": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Liveness probe (/minio/health/live)",
			},
			"ready": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Readiness probe (/minio/health/ready)",
			},
			"write_quorum": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Write quorum status (/minio/health/cluster)",
			},
			"read_quorum": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Read quorum status (/minio/health/cluster/read)",
			},
			"safe_for_maintenance": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Safe to perform maintenance without losing quorum",
			},
			"healthy": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Overall health (true if all checks pass)",
			},
		},
	}
}

func dataSourceMinioHealthStatusRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	m := meta.(*S3MinioClient)
	endpointURL := m.S3Client.EndpointURL()
	baseURL := fmt.Sprintf("%s://%s", endpointURL.Scheme, endpointURL.Host)

	log.Printf("[DEBUG] Checking MinIO health at %s", baseURL)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	live, err := checkHealthEndpoint(ctx, client, baseURL, "/minio/health/live")
	if err != nil {
		return NewResourceError("checking liveness", baseURL, err)
	}

	ready, err := checkHealthEndpoint(ctx, client, baseURL, "/minio/health/ready")
	if err != nil {
		return NewResourceError("checking readiness", baseURL, err)
	}

	writeQuorum, err := checkHealthEndpoint(ctx, client, baseURL, "/minio/health/cluster")
	if err != nil {
		return NewResourceError("checking write quorum", baseURL, err)
	}

	readQuorum, err := checkHealthEndpoint(ctx, client, baseURL, "/minio/health/cluster/read")
	if err != nil {
		return NewResourceError("checking read quorum", baseURL, err)
	}

	safeForMaintenance, err := checkMaintenanceEndpoint(ctx, client, baseURL, "/minio/health/cluster?maintenance=true")
	if err != nil {
		return NewResourceError("checking maintenance safety", baseURL, err)
	}

	healthy := live && ready && writeQuorum && readQuorum

	log.Printf("[DEBUG] Health status - live: %v, ready: %v, write_quorum: %v, read_quorum: %v, safe_for_maintenance: %v, healthy: %v",
		live, ready, writeQuorum, readQuorum, safeForMaintenance, healthy)

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

	if err := d.Set("live", live); err != nil {
		return NewResourceError("setting live", baseURL, err)
	}
	if err := d.Set("ready", ready); err != nil {
		return NewResourceError("setting ready", baseURL, err)
	}
	if err := d.Set("write_quorum", writeQuorum); err != nil {
		return NewResourceError("setting write_quorum", baseURL, err)
	}
	if err := d.Set("read_quorum", readQuorum); err != nil {
		return NewResourceError("setting read_quorum", baseURL, err)
	}
	if err := d.Set("safe_for_maintenance", safeForMaintenance); err != nil {
		return NewResourceError("setting safe_for_maintenance", baseURL, err)
	}
	if err := d.Set("healthy", healthy); err != nil {
		return NewResourceError("setting healthy", baseURL, err)
	}

	return nil
}

// Checks health endpoint. Returns true for 200, false for 503.
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
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusServiceUnavailable:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, url)
	}
}

// Checks maintenance endpoint. Returns true for 200 (safe), false for 412 (would lose quorum).
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
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusPreconditionFailed:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, url)
	}
}
