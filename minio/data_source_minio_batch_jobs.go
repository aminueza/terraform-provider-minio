package minio

import (
	"context"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

func dataSourceMinioBatchJobs() *schema.Resource {
	return &schema.Resource{
		Description: "Lists active MinIO batch jobs, optionally filtered by job type and status.",

		ReadContext: dataSourceMinioBatchJobsRead,

		Schema: map[string]*schema.Schema{
			"job_type": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Filter by batch job type (replicate, expire, keyrotate).",
			},
			"status": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Filter by job status (started, completed, failed). In-process filtering when the API does not support it.",
			},
			"jobs": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of matching batch jobs.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"job_id": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Job ID.",
						},
						"job_type": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Job type.",
						},
						"status": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Current job status.",
						},
						"user": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "User who submitted the job.",
						},
						"started": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Job start time in RFC3339 format.",
						},
					},
				},
			},
		},
	}
}

func dataSourceMinioBatchJobsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	log.Printf("[DEBUG] Reading batch jobs list")

	jobType := d.Get("job_type").(string)

	filter := &madmin.ListBatchJobsFilter{
		ByJobType: jobType,
	}

	result, err := admin.ListBatchJobs(ctx, filter)
	if err != nil {
		return NewResourceError("listing batch jobs", jobType, err)
	}

	// Build status map from BatchJobStatus for each job
	statusMap := make(map[string]string)
	for _, job := range result.Jobs {
		status, err := admin.BatchJobStatus(ctx, job.ID)
		if err != nil {
			log.Printf("[DEBUG] BatchJobStatus unavailable for %s: %v", job.ID, err)
			continue
		}
		statusMap[job.ID] = buildStatusString(status, job.Started, job.Elapsed)
	}

	// Filter jobs in-process by status if requested
	statusFilter := d.Get("status").(string)
	var filteredJobs []map[string]interface{}
	for _, job := range result.Jobs {
		if statusFilter != "" {
			jobStatus := statusMap[job.ID]
			if jobStatus == "" {
				jobStatus = "started"
			}
			if jobStatus != statusFilter {
				continue
			}
		}

		startedStr := ""
		if !job.Started.IsZero() {
			startedStr = job.Started.Format(time.RFC3339)
		}

		jobMap := map[string]interface{}{
			"job_id":   job.ID,
			"job_type": string(job.Type),
			"status":   statusMap[job.ID],
			"user":     job.User,
			"started":  startedStr,
		}
		filteredJobs = append(filteredJobs, jobMap)
	}

	if err := d.Set("jobs", filteredJobs); err != nil {
		return NewResourceError("setting jobs", "batch_jobs", err)
	}

	d.SetId("batch_jobs")

	log.Printf("[DEBUG] Read %d batch jobs", len(filteredJobs))

	return nil
}

func buildStatusString(status madmin.BatchJobStatus, started time.Time, elapsed time.Duration) string {
	if started.IsZero() {
		return ""
	}
	statusStr := "started"
	if status.LastMetric.Complete {
		statusStr = "completed"
	}
	if status.LastMetric.Failed {
		statusStr = "failed"
	}
	if elapsed > 0 {
		statusStr += " (" + elapsed.String() + ")"
	}
	return statusStr
}
