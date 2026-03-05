package minio

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketNotificationConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the event notification configuration of an existing S3 bucket.",
		Read:        dataSourceMinioS3BucketNotificationConfigRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true},
			"queue": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"arn":    {Type: schema.TypeString, Computed: true},
						"events": {Type: schema.TypeList, Computed: true, Elem: &schema.Schema{Type: schema.TypeString}},
						"prefix": {Type: schema.TypeString, Computed: true},
						"suffix": {Type: schema.TypeString, Computed: true},
					},
				},
			},
		},
	}
}

func dataSourceMinioS3BucketNotificationConfigRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	cfg, err := client.GetBucketNotification(context.Background(), bucket)
	if err != nil {
		d.SetId(bucket)
		return nil
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

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

	_ = d.Set("bucket", bucket)
	_ = d.Set("queue", queues)
	return nil
}
