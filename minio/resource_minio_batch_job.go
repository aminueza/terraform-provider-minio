package minio

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
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
		UpdateContext: minioUpdateBatchJob,
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
				Description: "Current job status (`started`, `completed`, or `failed`).",
			},
			"wait_for_status": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validation.StringInSlice([]string{"completed"}, false),
				Description:  "Block during Create until the job reaches this status. Only `completed` is supported; `started` is trivially true and `failed` is treated as an error during the wait.",
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

// minioUpdateBatchJob is a no-op refresh. wait_for_status and
// wait_timeout_seconds are Create-time-only knobs; changing them on an existing
// resource has no operational effect, so Update simply re-reads state.
func minioUpdateBatchJob(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return minioReadBatchJob(ctx, d, meta)
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

// waitForBatchJobStatus polls until the job reaches targetStatus or the timeout
// elapses. A "failed" status is always treated as a terminal error.
func waitForBatchJobStatus(ctx context.Context, config *S3MinioBatchJob, jobID string, targetStatus string, timeout time.Duration) diag.Diagnostics {
	log.Printf("[DEBUG] Waiting for batch job %s to reach status: %s", jobID, targetStatus)

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		status, err := config.MinioAdmin.BatchJobStatus(ctx, jobID)
		if err != nil {
			log.Printf("[DEBUG] BatchJobStatus unavailable for %s during wait: %v", jobID, err)
			return retry.RetryableError(fmt.Errorf("batch job status unavailable: %w", err))
		}

		current := bareStatus(status)
		if current == "failed" {
			return retry.NonRetryableError(fmt.Errorf("batch job %s failed", jobID))
		}
		if current == targetStatus {
			return nil
		}

		return retry.RetryableError(fmt.Errorf("batch job %s is %s, waiting for %s", jobID, current, targetStatus))
	})

	if err != nil {
		return NewResourceError("waiting for batch job status", jobID, err)
	}

	log.Printf("[DEBUG] Batch job %s reached status: %s", jobID, targetStatus)
	return nil
}
