package minio

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketReplicationStatus() *schema.Resource {
	return &schema.Resource{
		Description: "Reads replication status and metrics for a bucket including rule count, replicated/pending sizes, and error counts.",
		ReadContext: dataSourceMinioS3BucketReplicationStatusRead,
		Schema: map[string]*schema.Schema{
			"bucket":           {Type: schema.TypeString, Required: true},
			"rule_count":       {Type: schema.TypeInt, Computed: true},
			"replicated_size":  {Type: schema.TypeString, Computed: true},
			"replica_size":     {Type: schema.TypeString, Computed: true},
			"replicated_count": {Type: schema.TypeInt, Computed: true},
			"replica_count":    {Type: schema.TypeInt, Computed: true},
			"rules": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id":       {Type: schema.TypeString, Computed: true},
						"status":   {Type: schema.TypeString, Computed: true},
						"priority": {Type: schema.TypeInt, Computed: true},
						"target":   {Type: schema.TypeString, Computed: true},
					},
				},
			},
		},
	}
}

func dataSourceMinioS3BucketReplicationStatusRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	bucket := d.Get("bucket").(string)

	var diags diag.Diagnostics
	cfg, err := client.S3Client.GetBucketReplication(ctx, bucket)
	switch {
	case err == nil:
	case isS3CompatNotSupported(client, s3Compat405IsMissingFeature, err):
		diags = append(diags, dataSourceS3CompatGap(bucket, "rule_count", "bucket replication status"))
		d.SetId(bucket)
		_ = d.Set("rule_count", 0)
		return diags
	default:
		return NewResourceError("reading bucket replication status", bucket, err)
	}

	d.SetId(bucket)
	_ = d.Set("rule_count", len(cfg.Rules))

	rules := make([]map[string]interface{}, 0, len(cfg.Rules))
	for _, r := range cfg.Rules {
		rules = append(rules, map[string]interface{}{
			"id":       r.ID,
			"status":   string(r.Status),
			"priority": r.Priority,
			"target":   r.Destination.Bucket,
		})
	}
	_ = d.Set("rules", rules)

	metrics, err := client.S3Client.GetBucketReplicationMetrics(ctx, bucket)
	switch {
	case err == nil:
		_ = d.Set("replicated_size", strconv.FormatUint(metrics.ReplicatedSize, 10))
		_ = d.Set("replica_size", strconv.FormatUint(metrics.ReplicaSize, 10))
		_ = d.Set("replicated_count", int(metrics.ReplicatedCount))
		_ = d.Set("replica_count", int(metrics.ReplicaCount))
	case isReplicationConfigNotFoundError(err):
		// An unconfigured bucket returns ReplicationConfigurationNotFoundError on metrics too.
	case isS3CompatJSONEndpointNotSupported(client, err):
		diags = append(diags, dataSourceS3CompatGap(bucket, "replicated_size", "bucket replication metrics"))
	default:
		return NewResourceError("reading bucket replication metrics", bucket, err)
	}

	return diags
}
