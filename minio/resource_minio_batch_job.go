package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/madmin-go/v4"
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
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Batch job type. The provider queries the server's supported types via `GetSupportedBatchJobTypes` at Create time and rejects values the server does not advertise. Typically one of `replicate`, `expire`, `keyrotate`.",
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

	tflog.Debug(ctx, fmt.Sprintf("Creating batch job of type: %s", batchConfig.JobType))

	if diags := validateBatchJobType(ctx, batchConfig.MinioAdmin, batchConfig.JobType); diags != nil {
		return diags
	}

	result, err := batchConfig.MinioAdmin.StartBatchJob(ctx, batchConfig.JobYAML)
	if err != nil {
		return NewResourceError("starting batch job", batchConfig.JobType, err)
	}

	d.SetId(result.ID)

	tflog.Debug(ctx, fmt.Sprintf("Created batch job: %s", result.ID))

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
	tflog.Debug(ctx, fmt.Sprintf("Reading batch job: %s", jobID))

	_, err := batchConfig.MinioAdmin.DescribeBatchJob(ctx, jobID)
	if err != nil {
		errResp := madmin.ToErrorResponse(err)
		if errResp.Code == "XMinioBatchJobNotFound" {
			tflog.Debug(ctx, fmt.Sprintf("Batch job %s not found, removing from state", jobID))
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

	tflog.Debug(ctx, fmt.Sprintf("Read batch job: %s (status: %s)", jobID, statusStr))

	return nil
}

func minioUpdateBatchJob(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return minioReadBatchJob(ctx, d, meta)
}

func minioDeleteBatchJob(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	batchConfig := BatchJobConfig(d, meta)

	jobID := d.Id()
	tflog.Debug(ctx, fmt.Sprintf("Deleting (cancelling) batch job: %s", jobID))

	if err := batchConfig.MinioAdmin.CancelBatchJob(ctx, jobID); err != nil {
		errResp := madmin.ToErrorResponse(err)
		if errResp.Code == "XMinioBatchJobNotFound" {
			tflog.Debug(ctx, fmt.Sprintf("Batch job %s already not found, skipping cancel", jobID))
			d.SetId("")
			return nil
		}
		return NewResourceError("cancelling batch job", jobID, err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Deleted batch job: %s", jobID))
	d.SetId("")

	return nil
}

func validateBatchJobType(ctx context.Context, admin *madmin.AdminClient, jobType string) diag.Diagnostics {
	if jobType == "" {
		return NewResourceError("validating job_type", jobType, fmt.Errorf("job_type must not be empty"))
	}

	supported, apiUnavailable, err := admin.GetSupportedBatchJobTypes(ctx)
	if err != nil {
		return NewResourceError("listing supported batch job types", jobType, err)
	}

	if apiUnavailable {
		tflog.Debug(ctx, fmt.Sprintf("GetSupportedBatchJobTypes unavailable on this MinIO version, accepting job_type %q without server-side validation", jobType))
		return nil
	}

	for _, t := range supported {
		if string(t) == jobType {
			return nil
		}
	}

	names := make([]string, len(supported))
	for i, t := range supported {
		names[i] = string(t)
	}
	return NewResourceError("validating job_type", jobType, fmt.Errorf("job_type %q is not supported by this MinIO server; supported types: %s", jobType, strings.Join(names, ", ")))
}

func waitForBatchJobStatus(ctx context.Context, config *S3MinioBatchJob, jobID string, targetStatus string, timeout time.Duration) diag.Diagnostics {
	tflog.Debug(ctx, fmt.Sprintf("Waiting for batch job %s to reach status: %s", jobID, targetStatus))

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		status, err := config.MinioAdmin.BatchJobStatus(ctx, jobID)
		if err != nil {
			tflog.Debug(ctx, fmt.Sprintf("BatchJobStatus unavailable for %s during wait: %v", jobID, err))
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

	tflog.Debug(ctx, fmt.Sprintf("Batch job %s reached status: %s", jobID, targetStatus))
	return nil
}
