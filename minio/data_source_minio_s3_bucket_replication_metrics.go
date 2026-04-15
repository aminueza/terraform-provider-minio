package minio

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketReplicationMetrics() *schema.Resource {
	return &schema.Resource{
		Description: "Reads replication health metrics for a bucket: pending, failed, replicated and queued sizes and counts. Useful for monitoring replication lag.",
		Read:        dataSourceMinioS3BucketReplicationMetricsRead,
		Schema: map[string]*schema.Schema{
			"bucket":           {Type: schema.TypeString, Required: true},
			"pending_size":     {Type: schema.TypeString, Computed: true},
			"pending_count":    {Type: schema.TypeInt, Computed: true},
			"failed_size":      {Type: schema.TypeString, Computed: true},
			"failed_count":     {Type: schema.TypeInt, Computed: true},
			"replicated_size":  {Type: schema.TypeString, Computed: true},
			"replicated_count": {Type: schema.TypeInt, Computed: true},
			"replica_size":     {Type: schema.TypeString, Computed: true},
			"replica_count":    {Type: schema.TypeInt, Computed: true},
			"queued_size":      {Type: schema.TypeString, Computed: true},
			"queued_count":     {Type: schema.TypeInt, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketReplicationMetricsRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client

	bucket := d.Get("bucket").(string)
	ctx := context.Background()

	d.SetId(bucket)

	m, err := client.GetBucketReplicationMetricsV2(ctx, bucket)
	metrics := m.CurrentStats
	if err != nil {
		_ = d.Set("pending_size", "0")
		_ = d.Set("pending_count", 0)
		_ = d.Set("failed_size", "0")
		_ = d.Set("failed_count", 0)
		_ = d.Set("replicated_size", "0")
		_ = d.Set("replicated_count", 0)
		_ = d.Set("replica_size", "0")
		_ = d.Set("replica_count", 0)
		_ = d.Set("queued_size", "0")
		_ = d.Set("queued_count", 0)
		return nil
	}

	_ = d.Set("pending_size", strconv.FormatUint(metrics.PendingSize, 10))
	_ = d.Set("pending_count", int(metrics.PendingCount))
	_ = d.Set("failed_size", strconv.FormatUint(metrics.FailedSize, 10))
	_ = d.Set("failed_count", int(metrics.FailedCount))
	_ = d.Set("replicated_size", strconv.FormatUint(metrics.ReplicatedSize, 10))
	_ = d.Set("replicated_count", int(metrics.ReplicatedCount))
	_ = d.Set("replica_size", strconv.FormatUint(metrics.ReplicaSize, 10))
	_ = d.Set("replica_count", int(metrics.ReplicaCount))
	_ = d.Set("queued_size", strconv.FormatFloat(metrics.QStats.Curr.Bytes, 'f', 0, 64))
	_ = d.Set("queued_count", int(metrics.QStats.Curr.Count))

	return nil
}
