package minio

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioBatchJob() *schema.Resource {
	return &schema.Resource{
		Description: "Manages a MinIO batch job (replicate, expire, or keyrotate). " +
			"Batch jobs are asynchronous; this resource submits the job and tracks its status. " +
			"Use `wait_for_status` to optionally block until the job reaches a desired state. " +
			"Import is not supported because the job YAML definition cannot be retrieved from the MinIO API.",

		CreateContext: minioCreateBatchJob,
		ReadContext:   minioReadBatchJob,
		DeleteContext: minioDeleteBatchJob,

		Schema: map[string]*schema.Schema{
			"job_type": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice([]string{"replicate", "expire", "keyrotate"}, false),
				Description:  "Batch job type. One of `replicate`, `expire`, `keyrotate`.",
			},
			"job_yaml": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "YAML job definition for the batch operation.",
			},
			"job_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Assigned job ID returned by MinIO.",
			},
			"status": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Current job status string.",
			},
			"wait_for_status": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Block until the job reaches this status (`started`, `completed`, `failed`). If unset, returns immediately after submission.",
			},
			"wait_timeout_seconds": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     300,
				Description: "Maximum time in seconds to wait for `wait_for_status`. Defaults to 300.",
			},
		},
	}
}

func minioCreateBatchJob(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	batchConfig := BatchJobConfig(d, meta)

	log.Printf("[DEBUG] Creating batch job of type: %s", batchConfig.JobType)

	result, err := batchConfig.MinioAdmin.StartBatchJob(ctx, batchConfig.JobYAML)
	if err != nil {
		return NewResourceError("starting batch job", batchConfig.JobType, err)
	}

	d.SetId(result.ID)

	log.Printf("[DEBUG] Created batch job: %s", result.ID)

	// Optionally wait for the job to reach a desired status
	if waitFor, ok := d.GetOk("wait_for_status"); ok {
		timeout := time.Duration(d.Get("wait_timeout_seconds").(int)) * time.Second
		if diags := waitForBatchJobStatus(ctx, batchConfig, result.ID, waitFor.(string), timeout); diags != nil {
			return diags
		}
	}

	return minioReadBatchJob(ctx, d, meta)
}

func minioReadBatchJob(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	batchConfig := BatchJobConfig(d, meta)

	jobID := d.Id()
	log.Printf("[DEBUG] Reading batch job: %s", jobID)

	_, err := batchConfig.MinioAdmin.DescribeBatchJob(ctx, jobID)
	if err != nil {
		errResp := madmin.ToErrorResponse(err)
		if errResp.Code == "XMinioBatchJobNotFound" {
			log.Printf("[DEBUG] Batch job %s not found, removing from state", jobID)
			d.SetId("")
			return nil
		}
		return NewResourceError("describing batch job", jobID, err)
	}

	status, err := batchConfig.MinioAdmin.BatchJobStatus(ctx, jobID)
	if err != nil {
		return NewResourceError("getting batch job status", jobID, err)
	}

	// Build a descriptive status string
	statusStr := bareStatus(status)

	if err := d.Set("job_id", jobID); err != nil {
		return NewResourceError("setting job_id", jobID, err)
	}
	if err := d.Set("status", statusStr); err != nil {
		return NewResourceError("setting status", jobID, err)
	}

	log.Printf("[DEBUG] Read batch job: %s (status: %s)", jobID, statusStr)

	return nil
}

func minioDeleteBatchJob(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	batchConfig := BatchJobConfig(d, meta)

	jobID := d.Id()
	log.Printf("[DEBUG] Deleting (cancelling) batch job: %s", jobID)

	if err := batchConfig.MinioAdmin.CancelBatchJob(ctx, jobID); err != nil {
		errResp := madmin.ToErrorResponse(err)
		if errResp.Code == "XMinioBatchJobNotFound" {
			log.Printf("[DEBUG] Batch job %s already not found, skipping cancel", jobID)
			d.SetId("")
			return nil
		}
		return NewResourceError("cancelling batch job", jobID, err)
	}

	log.Printf("[DEBUG] Deleted batch job: %s", jobID)
	d.SetId("")

	return nil
}

// waitForBatchJobStatus polls the job status until it reaches the desired state or times out.
func waitForBatchJobStatus(ctx context.Context, config *S3MinioBatchJob, jobID string, targetStatus string, timeout time.Duration) diag.Diagnostics {
	log.Printf("[DEBUG] Waiting for batch job %s to reach status: %s", jobID, targetStatus)

	deadline := time.Now().Add(timeout)
	pollInterval := 5 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return NewResourceError("waiting for batch job status", jobID, ctx.Err())
		default:
		}

		status, err := config.MinioAdmin.BatchJobStatus(ctx, jobID)
		if err != nil {
			log.Printf("[DEBUG] BatchJobStatus unavailable for %s during wait: %v", jobID, err)
			time.Sleep(pollInterval)
			continue
		}

		currentStatus := bareStatus(status)
		if currentStatus == targetStatus {
			log.Printf("[DEBUG] Batch job %s reached status: %s", jobID, targetStatus)
			return nil
		}

		if currentStatus == "failed" && targetStatus != "failed" {
			return NewResourceError("waiting for batch job status", jobID, fmt.Errorf("job failed unexpectedly"))
		}

		time.Sleep(pollInterval)
	}

	return NewResourceError("waiting for batch job status", jobID, fmt.Errorf("timed out after %s waiting for status %q", timeout, targetStatus))
}
