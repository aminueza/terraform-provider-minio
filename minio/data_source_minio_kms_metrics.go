package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioKMSMetrics() *schema.Resource {
	return &schema.Resource{
		Description: "Exposes operational metrics for the KMS connected to the MinIO server.",
		ReadContext: dataSourceMinioKMSMetricsRead,
		Schema: map[string]*schema.Schema{
			"request_ok":       {Type: schema.TypeInt, Computed: true, Description: "HTTP requests that completed successfully."},
			"request_err":      {Type: schema.TypeInt, Computed: true, Description: "HTTP requests that returned an error response."},
			"request_fail":     {Type: schema.TypeInt, Computed: true, Description: "HTTP requests that failed to complete."},
			"request_active":   {Type: schema.TypeInt, Computed: true, Description: "HTTP requests currently in flight."},
			"audit_events":     {Type: schema.TypeInt, Computed: true, Description: "Audit log events written."},
			"error_events":     {Type: schema.TypeInt, Computed: true, Description: "Error log events written."},
			"uptime_seconds":   {Type: schema.TypeInt, Computed: true, Description: "KMS process uptime in seconds."},
			"cpus":             {Type: schema.TypeInt, Computed: true, Description: "Logical CPUs visible to the KMS process."},
			"usable_cpus":      {Type: schema.TypeInt, Computed: true, Description: "Logical CPUs the KMS process is allowed to use."},
			"threads":          {Type: schema.TypeInt, Computed: true, Description: "OS threads currently allocated."},
			"heap_alloc_bytes": {Type: schema.TypeInt, Computed: true, Description: "Bytes allocated on the Go heap."},
			"heap_objects":     {Type: schema.TypeInt, Computed: true, Description: "Live objects on the Go heap."},
			"stack_alloc_bytes": {Type: schema.TypeInt, Computed: true, Description: "Bytes allocated for goroutine stacks."},
		},
	}
}

func dataSourceMinioKMSMetricsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	m, err := admin.KMSMetrics(ctx)
	if err != nil {
		return NewResourceError("reading KMS metrics", "kms", err)
	}

	d.SetId("kms")
	_ = d.Set("request_ok", m.RequestOK)
	_ = d.Set("request_err", m.RequestErr)
	_ = d.Set("request_fail", m.RequestFail)
	_ = d.Set("request_active", m.RequestActive)
	_ = d.Set("audit_events", m.AuditEvents)
	_ = d.Set("error_events", m.ErrorEvents)
	_ = d.Set("uptime_seconds", m.UpTime)
	_ = d.Set("cpus", m.CPUs)
	_ = d.Set("usable_cpus", m.UsableCPUs)
	_ = d.Set("threads", m.Threads)
	_ = d.Set("heap_alloc_bytes", m.HeapAlloc)
	_ = d.Set("heap_objects", m.HeapObjects)
	_ = d.Set("stack_alloc_bytes", m.StackAlloc)

	return nil
}
