package minio

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

func dataSourceMinioBatchJobTemplate() *schema.Resource {
	return &schema.Resource{
		Description: "Returns a starter YAML template for a given MinIO batch job type. Calls `GenerateBatchJobV2` (EOS-only API) and falls back to the SDK's bundled static template when the server does not advertise the endpoint.",

		ReadContext: dataSourceMinioBatchJobTemplateRead,

		Schema: map[string]*schema.Schema{
			"job_type": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Batch job type to generate a template for (e.g. `replicate`, `expire`, `keyrotate`).",
			},
			"yaml": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "YAML job-definition template.",
			},
			"source": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Where the template came from: `server` (via `GenerateBatchJobV2`) or `sdk` (bundled fallback).",
			},
		},
	}
}

func dataSourceMinioBatchJobTemplateRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	jobType := d.Get("job_type").(string)
	opts := madmin.GenerateBatchJobOpts{Type: madmin.BatchJobType(jobType)}

	log.Printf("[DEBUG] Generating batch job template for type: %s", jobType)

	tmpl, apiUnavailable, err := admin.GenerateBatchJobV2(ctx, opts)
	source := "server"
	if err != nil {
		return NewResourceError("generating batch job template", jobType, err)
	}
	if apiUnavailable {
		log.Printf("[DEBUG] GenerateBatchJobV2 unavailable, falling back to SDK template for type: %s", jobType)
		tmpl, err = admin.GenerateBatchJob(ctx, opts)
		if err != nil {
			return NewResourceError("generating batch job template (fallback)", jobType, err)
		}
		source = "sdk"
	}

	if err := d.Set("yaml", tmpl); err != nil {
		return NewResourceError("setting yaml", jobType, err)
	}
	if err := d.Set("source", source); err != nil {
		return NewResourceError("setting source", jobType, err)
	}

	d.SetId("batch_job_template:" + jobType)

	return nil
}
