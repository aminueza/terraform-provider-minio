package minio

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7/pkg/notification"
)

func resourceMinioBucketNotification() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioPutBucketNotification,
		ReadContext:   minioReadBucketNotification,
		UpdateContext: minioPutBucketNotification,
		DeleteContext: minioDeleteBucketNotification,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"queue": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
						"filter_prefix": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"filter_suffix": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"queue_arn": {
							Type:             schema.TypeString,
							Required:         true,
							ValidateDiagFunc: validateMinioArn,
						},
						"events": {
							Type:     schema.TypeSet,
							Required: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
							Set:      schema.HashString,
						},
					},
				},
			},
		},
	}
}

func minioPutBucketNotification(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketNotificationConfig := BucketNotificationConfig(d, meta)

	log.Printf("[DEBUG] S3 bucket: %s, put notification configuration: %v", bucketNotificationConfig.MinioBucket, bucketNotificationConfig.Configuration)

	err := bucketNotificationConfig.MinioClient.SetBucketNotification(
		ctx,
		bucketNotificationConfig.MinioBucket,
		*bucketNotificationConfig.Configuration,
	)

	if err != nil {
		return NewResourceError("error putting bucket notification configuration: %v", d.Id(), err)
	}

	d.SetId(bucketNotificationConfig.MinioBucket)

	return nil
}

func minioReadBucketNotification(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketNotificationConfig := BucketNotificationConfig(d, meta)

	log.Printf("[DEBUG] S3 bucket notification configuration, read for bucket: %s", d.Id())

	notificationConfig, err := bucketNotificationConfig.MinioClient.GetBucketNotification(ctx, d.Id())
	if err != nil {
		return NewResourceError("failed to load bucket notification configuration", d.Id(), err)
	}

	_ = d.Set("bucket", d.Id())

	if err := d.Set("queue", flattenQueueNotificationConfiguration(notificationConfig.QueueConfigs)); err != nil {
		return NewResourceError("failed to load bucket queue notifications", d.Id(), err)
	}

	return nil
}

func minioDeleteBucketNotification(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketNotificationConfig := BucketNotificationConfig(d, meta)

	log.Printf("[DEBUG] S3 bucket: %s, removing notification configuration", bucketNotificationConfig.MinioBucket)

	err := bucketNotificationConfig.MinioClient.SetBucketNotification(
		ctx,
		bucketNotificationConfig.MinioBucket,
		notification.Configuration{},
	)

	if err != nil {
		return NewResourceError("error removing bucket notifications: %s", bucketNotificationConfig.MinioBucket, err)
	}

	return nil
}

func flattenNotificationConfigurationFilter(filter *notification.Filter) map[string]interface{} {
	filterRules := map[string]interface{}{}
	if filter.S3Key.FilterRules == nil {
		return filterRules
	}

	for _, f := range filter.S3Key.FilterRules {
		if f.Name == "prefix" {
			filterRules["filter_prefix"] = f.Value
		}
		if f.Name == "suffix" {
			filterRules["filter_suffix"] = f.Value
		}
	}
	return filterRules
}

func flattenQueueNotificationConfiguration(configs []notification.QueueConfig) []map[string]interface{} {
	queueNotifications := make([]map[string]interface{}, 0, len(configs))
	for _, notification := range configs {
		var conf map[string]interface{}
		if filter := notification.Filter; filter != nil {
			conf = flattenNotificationConfigurationFilter(filter)
		} else {
			conf = map[string]interface{}{}
		}

		conf["id"] = notification.ID
		conf["events"] = notification.Events
		// The Config.Arn value is not set to the queue ARN even though it's
		// expected in the submission, so we're getting the correct value
		// from the Queue attribute on the response object
		conf["queue_arn"] = notification.Queue
		queueNotifications = append(queueNotifications, conf)
	}

	return queueNotifications
}

func getNotificationConfiguration(d *schema.ResourceData) notification.Configuration {
	var config notification.Configuration
	queueConfigs := getNotificationQueueConfigs(d)

	for _, c := range queueConfigs {
		config.AddQueue(c)
	}

	return config
}

func getNotificationQueueConfigs(d *schema.ResourceData) []notification.Config {
	queueFunctionNotifications := d.Get("queue").([]interface{})
	configs := make([]notification.Config, 0, len(queueFunctionNotifications))

	for i, c := range queueFunctionNotifications {
		config := notification.Config{Filter: &notification.Filter{}}
		c := c.(map[string]interface{})

		if queueArnStr, ok := c["queue_arn"].(string); ok {
			queueArn, err := notification.NewArnFromString(queueArnStr)
			if err != nil {
				continue
			}
			config.Arn = queueArn
		}

		if val, ok := c["id"].(string); ok && val != "" {
			config.ID = val
		} else {
			config.ID = id.PrefixedUniqueId("tf-s3-queue-")
		}

		events := d.Get(fmt.Sprintf("queue.%d.events", i)).(*schema.Set).List()
		for _, e := range events {
			config.AddEvents(notification.EventType(e.(string)))
		}

		if val, ok := c["filter_prefix"].(string); ok && val != "" {
			config.AddFilterPrefix(val)
		}
		if val, ok := c["filter_suffix"].(string); ok && val != "" {
			config.AddFilterSuffix(val)
		}

		configs = append(configs, config)
	}

	return configs
}

func validateMinioArn(v interface{}, p cty.Path) (errors diag.Diagnostics) {
	value := v.(string)
	_, err := notification.NewArnFromString(value)

	if err != nil {
		return diag.Errorf("value: %s is not a valid ARN", value)
	}

	return nil
}
