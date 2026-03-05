package minio

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketReplicationStatus() *schema.Resource {
	return &schema.Resource{
		Description: "Reads replication status and metrics for a bucket including rule count, replicated/pending sizes, and error counts.",
		Read:        dataSourceMinioS3BucketReplicationStatusRead,
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

func dataSourceMinioS3BucketReplicationStatusRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client

	bucket := d.Get("bucket").(string)
	ctx := context.Background()

	d.SetId(bucket)

	cfg, err := client.GetBucketReplication(ctx, bucket)
	if err != nil {
		_ = d.Set("rule_count", 0)
		return nil
	}

	_ = d.Set("rule_count", len(cfg.Rules))

	var rules []map[string]interface{}
	for _, r := range cfg.Rules {
		rules = append(rules, map[string]interface{}{
			"id":       r.ID,
			"status":   string(r.Status),
			"priority": r.Priority,
			"target":   r.Destination.Bucket,
		})
	}
	_ = d.Set("rules", rules)

	metrics, err := client.GetBucketReplicationMetrics(ctx, bucket)
	if err == nil {
		_ = d.Set("replicated_size", strconv.FormatUint(metrics.ReplicatedSize, 10))
		_ = d.Set("replica_size", strconv.FormatUint(metrics.ReplicaSize, 10))
		_ = d.Set("replicated_count", int(metrics.ReplicatedCount))
		_ = d.Set("replica_count", int(metrics.ReplicaCount))
	}

	return nil
}
