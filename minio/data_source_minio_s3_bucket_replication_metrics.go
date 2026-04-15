package minio

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketReplicationMetrics() *schema.Resource {
	return &schema.Resource{
		Description: "Reads replication health metrics for a bucket: pending, failed, replicated and queued sizes and counts. Useful for monitoring replication lag. Counts and sizes are returned as strings to represent uint64 values safely.",
		Read:        dataSourceMinioS3BucketReplicationMetricsRead,
		Schema: map[string]*schema.Schema{
			"bucket":           {Type: schema.TypeString, Required: true},
			"pending_size":     {Type: schema.TypeString, Computed: true},
			"pending_count":    {Type: schema.TypeString, Computed: true},
			"failed_size":      {Type: schema.TypeString, Computed: true},
			"failed_count":     {Type: schema.TypeString, Computed: true},
			"replicated_size":  {Type: schema.TypeString, Computed: true},
			"replicated_count": {Type: schema.TypeString, Computed: true},
			"replica_size":     {Type: schema.TypeString, Computed: true},
			"replica_count":    {Type: schema.TypeString, Computed: true},
			"queued_size":      {Type: schema.TypeString, Computed: true},
			"queued_count":     {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketReplicationMetricsRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client

	bucket := d.Get("bucket").(string)
	ctx := context.Background()

	d.SetId(bucket)

	zeros := []string{
		"pending_size", "pending_count",
		"failed_size", "failed_count",
		"replicated_size", "replicated_count",
		"replica_size", "replica_count",
		"queued_size", "queued_count",
	}

	m, err := client.GetBucketReplicationMetricsV2(ctx, bucket)
	if err != nil {
		for _, k := range zeros {
			_ = d.Set(k, "0")
		}
		return nil
	}
	metrics := m.CurrentStats

	_ = d.Set("pending_size", strconv.FormatUint(metrics.PendingSize, 10))
	_ = d.Set("pending_count", strconv.FormatUint(metrics.PendingCount, 10))
	_ = d.Set("failed_size", strconv.FormatUint(metrics.FailedSize, 10))
	_ = d.Set("failed_count", strconv.FormatUint(metrics.FailedCount, 10))
	_ = d.Set("replicated_size", strconv.FormatUint(metrics.ReplicatedSize, 10))
	_ = d.Set("replicated_count", strconv.FormatInt(metrics.ReplicatedCount, 10))
	_ = d.Set("replica_size", strconv.FormatUint(metrics.ReplicaSize, 10))
	_ = d.Set("replica_count", strconv.FormatInt(metrics.ReplicaCount, 10))
	_ = d.Set("queued_size", strconv.FormatFloat(metrics.QStats.Curr.Bytes, 'f', 0, 64))
	_ = d.Set("queued_count", strconv.FormatFloat(metrics.QStats.Curr.Count, 'f', 0, 64))

	return nil
}
