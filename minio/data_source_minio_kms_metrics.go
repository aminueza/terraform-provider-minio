package minio

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioKMSMetrics() *schema.Resource {
	return &schema.Resource{
		Description: "Exposes operational metrics for the KMS connected to the MinIO server. " +
			"The KES latency histogram is intentionally not mapped; use the KES Prometheus endpoint directly if you need buckets.",
		ReadContext: dataSourceMinioKMSMetricsRead,
		Schema: map[string]*schema.Schema{
			"request_ok":        {Type: schema.TypeInt, Computed: true, Description: "HTTP requests that completed successfully."},
			"request_err":       {Type: schema.TypeInt, Computed: true, Description: "HTTP requests that returned an error response."},
			"request_fail":      {Type: schema.TypeInt, Computed: true, Description: "HTTP requests that failed to complete."},
			"request_active":    {Type: schema.TypeInt, Computed: true, Description: "HTTP requests currently in flight."},
			"audit_events":      {Type: schema.TypeInt, Computed: true, Description: "Audit log events written."},
			"error_events":      {Type: schema.TypeInt, Computed: true, Description: "Error log events written."},
			"uptime_seconds":    {Type: schema.TypeInt, Computed: true, Description: "KMS process uptime in seconds."},
			"cpus":              {Type: schema.TypeInt, Computed: true, Description: "Logical CPUs visible to the KMS process."},
			"usable_cpus":       {Type: schema.TypeInt, Computed: true, Description: "Logical CPUs the KMS process is allowed to use."},
			"threads":           {Type: schema.TypeInt, Computed: true, Description: "OS threads currently allocated."},
			"heap_alloc_bytes":  {Type: schema.TypeInt, Computed: true, Description: "Bytes allocated on the Go heap."},
			"heap_objects":      {Type: schema.TypeInt, Computed: true, Description: "Live objects on the Go heap."},
			"stack_alloc_bytes": {Type: schema.TypeInt, Computed: true, Description: "Bytes allocated for goroutine stacks."},
		},
	}
}

func dataSourceMinioKMSMetricsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	log.Printf("[DEBUG] Reading KMS metrics")

	m, err := admin.KMSMetrics(ctx)
	if err != nil {
		return NewResourceError("reading KMS metrics", "kms", err)
	}

	d.SetId("kms")

	for _, field := range []struct {
		key   string
		value interface{}
	}{
		{"request_ok", m.RequestOK},
		{"request_err", m.RequestErr},
		{"request_fail", m.RequestFail},
		{"request_active", m.RequestActive},
		{"audit_events", m.AuditEvents},
		{"error_events", m.ErrorEvents},
		{"uptime_seconds", m.UpTime},
		{"cpus", m.CPUs},
		{"usable_cpus", m.UsableCPUs},
		{"threads", m.Threads},
		{"heap_alloc_bytes", m.HeapAlloc},
		{"heap_objects", m.HeapObjects},
		{"stack_alloc_bytes", m.StackAlloc},
	} {
		if err := d.Set(field.key, field.value); err != nil {
			return NewResourceError("setting "+field.key, "kms", err)
		}
	}

	return nil
}
