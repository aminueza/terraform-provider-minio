package minio

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
)

func dataSourceMinioS3BucketNotificationConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the event notification configuration of an existing S3 bucket.",
		ReadContext: dataSourceMinioS3BucketNotificationConfigRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true, Description: "Bucket name"},
			"queue": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"arn":    {Type: schema.TypeString, Computed: true, Description: "ARN of the queue"},
						"events": {Type: schema.TypeList, Computed: true, Elem: &schema.Schema{Type: schema.TypeString}, Description: "Events to notify on"},
						"prefix": {Type: schema.TypeString, Computed: true, Description: "Object key prefix filter"},
						"suffix": {Type: schema.TypeString, Computed: true, Description: "Object key suffix filter"},
					},
				},
			},
		},
	}
}

func dataSourceMinioS3BucketNotificationConfigRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	cfg, err := client.GetBucketNotification(ctx, bucket)
	if err != nil {
		var respErr minio.ErrorResponse
		if errors.As(err, &respErr) && respErr.Code == "NoSuchBucket" {
			d.SetId("")
			return nil
		}
		return NewResourceError("reading bucket notification configuration", bucket, err)
	}

	d.SetId(bucket)

	var queues []map[string]interface{}
	for _, q := range cfg.QueueConfigs {
		events := make([]string, len(q.Events))
		for i, e := range q.Events {
			events[i] = string(e)
		}
		prefix := ""
		suffix := ""
		for _, rule := range q.Filter.S3Key.FilterRules {
			switch rule.Name {
			case "prefix":
				prefix = rule.Value
			case "suffix":
				suffix = rule.Value
			}
		}
		queues = append(queues, map[string]interface{}{
			"arn":    q.Queue,
			"events": events,
			"prefix": prefix,
			"suffix": suffix,
		})
	}

	if err := d.Set("bucket", bucket); err != nil {
		return NewResourceError("setting bucket", d.Id(), err)
	}
	if err := d.Set("queue", queues); err != nil {
		return NewResourceError("setting queue", d.Id(), err)
	}
	return nil
}
