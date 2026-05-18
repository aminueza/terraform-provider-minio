package minio

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioBatchJob() *schema.Resource {
	return &schema.Resource{
		Description: "Manages a MinIO batch job (replicate, expire, or keyrotate).",

		CreateContext: minioCreateBatchJob,
		ReadContext:   minioReadBatchJob,
		DeleteContext: minioDeleteBatchJob,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"job_type": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Batch job type. One of `replicate`, `expire`, `keyrotate`.",
			},
			"job_yaml": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
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
	if err := d.Set("job_id", result.ID); err != nil {
		return NewResourceError("setting job_id", d.Id(), err)
	}

	log.Printf("[DEBUG] Created batch job: %s", result.ID)

	return minioReadBatchJob(ctx, d, meta)
}

func minioReadBatchJob(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	batchConfig := BatchJobConfig(d, meta)

	jobID := d.Id()
	log.Printf("[DEBUG] Reading batch job: %s", jobID)

	_, err := batchConfig.MinioAdmin.DescribeBatchJob(ctx, jobID)
	if err != nil {
		errResp := madmin.ToErrorResponse(err)
		if errResp.Code == "BadRequest" || errResp.Code == "XMinioBatchJobNotFound" {
			log.Printf("[DEBUG] Batch job %s not found, removing from state", jobID)
			d.SetId("")
			return nil
		}
		return NewResourceError("describing batch job", jobID, err)
	}

	status, err := batchConfig.MinioAdmin.BatchJobStatus(ctx, jobID)
	if err != nil {
		log.Printf("[DEBUG] BatchJobStatus unavailable for %s: %v", jobID, err)
	}

	// Build a descriptive status string
	statusStr := "started"
	if status.LastMetric.Complete {
		statusStr = "completed"
	}
	if status.LastMetric.Failed {
		statusStr = "failed"
	}

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
		// Ignore "already finished" or "not found" errors for idempotency
		if errResp.Code == "BadRequest" || errResp.Code == "XMinioBatchJobNotFound" {
			log.Printf("[DEBUG] Batch job %s already finished or not found, skipping cancel", jobID)
			d.SetId("")
			return nil
		}
		return NewResourceError("cancelling batch job", jobID, err)
	}

	log.Printf("[DEBUG] Deleted batch job: %s", jobID)
	d.SetId("")

	return nil
}
