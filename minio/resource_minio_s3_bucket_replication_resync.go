package minio

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioBucketReplicationResync() *schema.Resource {
	return &schema.Resource{
		Description:   "Triggers a resync of existing objects for a bucket replication rule. This replays all objects that existed before replication was configured or that failed to replicate.",
		CreateContext: minioCreateBucketReplicationResync,
		ReadContext:   minioReadBucketReplicationResync,
		DeleteContext: minioDeleteBucketReplicationResync,
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Source bucket name.",
			},
			"target_arn": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "ARN of a specific replication target to resync. If omitted, all targets are resynced.",
			},
			"older_than": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Default:     "0s",
				Description: "Only resync objects older than this duration (e.g., \"24h\", \"7d\"). Default resyncs all objects.",
			},
			"reset_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Server-generated reset ID for tracking this resync operation.",
			},
		},
	}
}

func minioCreateBucketReplicationResync(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)
	targetArn := d.Get("target_arn").(string)
	olderThanStr := d.Get("older_than").(string)

	olderThan, err := time.ParseDuration(olderThanStr)
	if err != nil {
		return NewResourceError("parsing older_than duration", bucket, err)
	}

	log.Printf("[DEBUG] Triggering replication resync for bucket %s", bucket)

	if targetArn != "" {
		info, err := client.ResetBucketReplicationOnTarget(ctx, bucket, olderThan, targetArn)
		if err != nil {
			return NewResourceError("triggering replication resync", bucket, err)
		}
		d.SetId(fmt.Sprintf("%s/%s", bucket, targetArn))
		if len(info.Targets) > 0 {
			_ = d.Set("reset_id", info.Targets[0].ResetID)
		}
	} else {
		resetID, err := client.ResetBucketReplication(ctx, bucket, olderThan)
		if err != nil {
			return NewResourceError("triggering replication resync", bucket, err)
		}
		d.SetId(bucket)
		_ = d.Set("reset_id", resetID)
	}

	log.Printf("[DEBUG] Replication resync triggered for bucket %s", bucket)

	return nil
}

func minioReadBucketReplicationResync(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

func minioDeleteBucketReplicationResync(_ context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
	d.SetId("")
	return nil
}
