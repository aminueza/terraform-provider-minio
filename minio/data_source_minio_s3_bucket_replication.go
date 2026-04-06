package minio

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7/pkg/replication"
)

func dataSourceMinioS3BucketReplication() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the replication configuration of an existing S3 bucket.",
		Read:        dataSourceMinioS3BucketReplicationRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true},
			"rule": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id":                          {Type: schema.TypeString, Computed: true},
						"enabled":                     {Type: schema.TypeBool, Computed: true},
						"priority":                    {Type: schema.TypeInt, Computed: true},
						"prefix":                      {Type: schema.TypeString, Computed: true},
						"tags":                        {Type: schema.TypeMap, Computed: true, Elem: &schema.Schema{Type: schema.TypeString}},
						"delete_replication":          {Type: schema.TypeBool, Computed: true},
						"delete_marker_replication":   {Type: schema.TypeBool, Computed: true},
						"existing_object_replication": {Type: schema.TypeBool, Computed: true},
						"metadata_sync":               {Type: schema.TypeBool, Computed: true},
						"destination": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"bucket":        {Type: schema.TypeString, Computed: true},
									"storage_class": {Type: schema.TypeString, Computed: true},
									"host":          {Type: schema.TypeString, Computed: true},
									"secure":        {Type: schema.TypeBool, Computed: true},
									"path_style":    {Type: schema.TypeString, Computed: true},
									"path":          {Type: schema.TypeString, Computed: true},
									"synchronous":   {Type: schema.TypeBool, Computed: true},
									"region":        {Type: schema.TypeString, Computed: true},
								},
							},
						},
					},
				},
			},
		},
	}
}

func dataSourceMinioS3BucketReplicationRead(d *schema.ResourceData, meta interface{}) error {
	m := meta.(*S3MinioClient)
	client := m.S3Client
	admin := m.S3Admin
	bucket := d.Get("bucket").(string)
	ctx := context.Background()

	d.SetId(bucket)

	cfg, err := client.GetBucketReplication(ctx, bucket)
	if err != nil {
		_ = d.Set("rule", []interface{}{})
		return nil
	}

	arnToTarget := map[string]int{}
	remoteTargets, err := admin.ListRemoteTargets(ctx, bucket, "")
	if err == nil {
		for i := range remoteTargets {
			arnToTarget[remoteTargets[i].Arn] = i
		}
	}

	rules := make([]map[string]interface{}, 0, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		r := map[string]interface{}{
			"id":                          rule.ID,
			"enabled":                     rule.Status == replication.Enabled,
			"priority":                    rule.Priority,
			"prefix":                      rule.Prefix(),
			"delete_replication":          rule.DeleteReplication.Status == replication.Enabled,
			"delete_marker_replication":   rule.DeleteMarkerReplication.Status == replication.Enabled,
			"existing_object_replication": rule.ExistingObjectReplication.Status == replication.Enabled,
			"metadata_sync":               rule.SourceSelectionCriteria.ReplicaModifications.Status == replication.Enabled,
		}

		if len(rule.Filter.And.Tags) != 0 || rule.Filter.And.Prefix != "" {
			tags := map[string]string{}
			for _, tag := range rule.Filter.And.Tags {
				if tag.IsEmpty() {
					continue
				}
				tags[tag.Key] = tag.Value
			}
			r["tags"] = tags
		} else if rule.Filter.Tag.Key != "" {
			r["tags"] = map[string]string{rule.Filter.Tag.Key: rule.Filter.Tag.Value}
		}

		dest := map[string]interface{}{
			"storage_class": rule.Destination.StorageClass,
		}

		if idx, ok := arnToTarget[rule.Destination.Bucket]; ok {
			rt := remoteTargets[idx]
			pathComponents := strings.Split(rt.TargetBucket, "/")
			dest["bucket"] = pathComponents[len(pathComponents)-1]
			dest["host"] = rt.Endpoint
			dest["secure"] = rt.Secure
			dest["path_style"] = rt.Path
			dest["path"] = strings.Join(pathComponents[:len(pathComponents)-1], "/")
			dest["synchronous"] = rt.ReplicationSync
			dest["region"] = rt.Region
		}

		r["destination"] = []interface{}{dest}
		rules = append(rules, r)
	}

	_ = d.Set("rule", rules)
	return nil
}
