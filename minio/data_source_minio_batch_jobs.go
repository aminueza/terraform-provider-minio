package minio

import (
	"context"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
	"golang.org/x/sync/errgroup"
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
							Description: "Current job status (started, completed, failed).",
						},
						"elapsed": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Elapsed time since the job started, formatted as a Go duration string.",
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

	statuses := make([]string, len(result.Jobs))
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	for i, job := range result.Jobs {
		i, job := i, job
		g.Go(func() error {
			status, err := admin.BatchJobStatus(gCtx, job.ID)
			if err != nil {
				log.Printf("[DEBUG] BatchJobStatus unavailable for %s: %v", job.ID, err)
				statuses[i] = "started"
				return nil
			}
			statuses[i] = bareStatus(status)
			return nil
		})
	}
	_ = g.Wait()

	statusFilter := d.Get("status").(string)
	var filteredJobs []map[string]interface{}
	for i, job := range result.Jobs {
		jobStatus := statuses[i]

		if statusFilter != "" && jobStatus != statusFilter {
			continue
		}

		startedStr := ""
		if !job.Started.IsZero() {
			startedStr = job.Started.Format(time.RFC3339)
		}

		elapsedStr := ""
		if job.Elapsed > 0 {
			elapsedStr = job.Elapsed.String()
		}

		jobMap := map[string]interface{}{
			"job_id":   job.ID,
			"job_type": string(job.Type),
			"status":   jobStatus,
			"elapsed":  elapsedStr,
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

func bareStatus(status madmin.BatchJobStatus) string {
	if status.LastMetric.Failed {
		return "failed"
	}
	if status.LastMetric.Complete {
		return "completed"
	}
	return "started"
}
