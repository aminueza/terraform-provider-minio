package minio

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
)

func dataSourceMinioS3BucketReplicationMetrics() *schema.Resource {
	return &schema.Resource{
		Description: "Reads replication health metrics for a bucket: pending, failed, replicated and queued sizes and counts. Useful for monitoring replication lag. Counts and sizes are returned as strings to represent uint64 values safely.",
		ReadContext: dataSourceMinioS3BucketReplicationMetricsRead,
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

func dataSourceMinioS3BucketReplicationMetricsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	bucket := d.Get("bucket").(string)

	var diags diag.Diagnostics
	m, err := client.S3Client.GetBucketReplicationMetricsV2(ctx, bucket)
	switch {
	case err == nil:
		metrics := m.CurrentStats

		if errDiags := setResourceField(d, "pending_size", strconv.FormatUint(metrics.PendingSize, 10), bucket); errDiags.HasError() {
			return errDiags
		}
		if errDiags := setResourceField(d, "pending_count", strconv.FormatUint(metrics.PendingCount, 10), bucket); errDiags.HasError() {
			return errDiags
		}
		if errDiags := setResourceField(d, "failed_size", strconv.FormatUint(metrics.FailedSize, 10), bucket); errDiags.HasError() {
			return errDiags
		}
		if errDiags := setResourceField(d, "failed_count", strconv.FormatUint(metrics.FailedCount, 10), bucket); errDiags.HasError() {
			return errDiags
		}
		if errDiags := setResourceField(d, "replicated_size", strconv.FormatUint(metrics.ReplicatedSize, 10), bucket); errDiags.HasError() {
			return errDiags
		}
		if errDiags := setResourceField(d, "replicated_count", strconv.FormatInt(metrics.ReplicatedCount, 10), bucket); errDiags.HasError() {
			return errDiags
		}
		if errDiags := setResourceField(d, "replica_size", strconv.FormatUint(metrics.ReplicaSize, 10), bucket); errDiags.HasError() {
			return errDiags
		}
		if errDiags := setResourceField(d, "replica_count", strconv.FormatInt(metrics.ReplicaCount, 10), bucket); errDiags.HasError() {
			return errDiags
		}
		if errDiags := setResourceField(d, "queued_size", strconv.FormatFloat(metrics.QStats.Curr.Bytes, 'f', 0, 64), bucket); errDiags.HasError() {
			return errDiags
		}
		if errDiags := setResourceField(d, "queued_count", strconv.FormatFloat(metrics.QStats.Curr.Count, 'f', 0, 64), bucket); errDiags.HasError() {
			return errDiags
		}
	case isReplicationConfigNotFoundError(err):
		if errDiags := setBucketReplicationMetricsZero(d, bucket); errDiags.HasError() {
			return errDiags
		}
	case isS3CompatJSONEndpointNotSupported(client, err):
		if errDiags := setBucketReplicationMetricsZero(d, bucket); errDiags.HasError() {
			return errDiags
		}
		diags = append(diags, dataSourceS3CompatGap(bucket, "pending_size", "bucket replication metrics"))
	default:
		return NewResourceError("reading bucket replication metrics", bucket, err)
	}

	d.SetId(bucket)
	return diags
}

func isReplicationConfigNotFoundError(err error) bool {
	return err != nil && minio.ToErrorResponse(err).Code == "ReplicationConfigurationNotFoundError"
}

func setBucketReplicationMetricsZero(d *schema.ResourceData, bucket string) diag.Diagnostics {
	for _, k := range []string{
		"pending_size", "pending_count",
		"failed_size", "failed_count",
		"replicated_size", "replicated_count",
		"replica_size", "replica_count",
		"queued_size", "queued_count",
	} {
		if diags := setResourceField(d, k, "0", bucket); diags.HasError() {
			return diags
		}
	}
	return nil
}
