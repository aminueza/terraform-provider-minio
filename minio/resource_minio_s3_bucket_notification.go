package minio

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7/pkg/notification"
)

// bucketNotificationLock serializes read-modify-write operations for bucket notifications
// to prevent concurrent resources from clobbering each other's queues.
var bucketNotificationLock = NewMutexKV()

func resourceMinioBucketNotification() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages event notification configuration for an S3 bucket. Sends bucket events to configured queue targets.",
		CreateContext: minioPutBucketNotification,
		ReadContext:   minioReadBucketNotification,
		UpdateContext: minioPutBucketNotification,
		DeleteContext: minioDeleteBucketNotification,
		Importer: &schema.ResourceImporter{
			StateContext: importBucketNotification,
		},
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the bucket.",
			},
			"queue": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "List of queue notification configurations.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:         schema.TypeString,
							Optional:     true,
							Computed:     true,
							ValidateFunc: validation.StringDoesNotMatch(regexp.MustCompile(`[,|]`), "queue id must not contain ',' or '|'"),
							Description:  "Unique identifier for the queue notification. Must be unique across all resources targeting the same bucket.",
						},
						"filter_prefix": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Object key name prefix to filter notifications.",
						},
						"filter_suffix": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Object key name suffix to filter notifications.",
						},
						"queue_arn": {
							Type:             schema.TypeString,
							Required:         true,
							ValidateDiagFunc: validateMinioArn,
							Description:      "ARN of the queue target.",
						},
						"events": {
							Type:        schema.TypeSet,
							Required:    true,
							Elem:        &schema.Schema{Type: schema.TypeString},
							Set:         schema.HashString,
							Description: "Set of event types to listen for (e.g., s3:ObjectCreated:*).",
						},
					},
				},
			},
		},
	}
}

func minioPutBucketNotification(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketNotificationConfig := BucketNotificationConfig(d, meta)

	bucketName := d.Get("bucket").(string)
	tflog.Debug(ctx, fmt.Sprintf("S3 bucket: %s, put notification configuration: %v", bucketName, bucketNotificationConfig.Configuration))

	// Lock to prevent concurrent read-modify-write races when multiple resources
	// target the same bucket. This serializes within one provider process only.
	bucketNotificationLock.Lock(bucketName)
	defer bucketNotificationLock.Unlock(bucketName)

	// Read-modify-write: read the current bucket notification config, remove this
	// resource's old queues (from state), add the new queues (from config), then
	// write back. This prevents clobbering other resources' notifications on the
	// same bucket.
	currentConfig, err := bucketNotificationConfig.MinioClient.GetBucketNotification(ctx, bucketName)
	if err != nil {
		return NewResourceError("error reading bucket notifications before update", bucketName, err)
	}

	// Identify queues to remove: those whose IDs are in the current state but not
	// in the new config. This handles the case where a queue is removed from HCL.
	oldQueueIDs := getQueueIDsFromState(d)

	// Build the new config: keep all queues from the current config that don't
	// belong to this resource, then add the new queues from the resource config.
	newConfig := notification.Configuration{}

	// Preserve non-queue configs (TopicConfigs, LambdaConfigs) from the current config
	newConfig.LambdaConfigs = currentConfig.LambdaConfigs
	newConfig.TopicConfigs = currentConfig.TopicConfigs

	// Keep queues that don't belong to this resource — append the original struct
	// verbatim to preserve the Queue (ARN) field that AddQueue would otherwise lose.
	for _, q := range currentConfig.QueueConfigs {
		keep := true
		for _, id := range oldQueueIDs {
			if q.ID == id {
				keep = false
				break
			}
		}
		if keep {
			newConfig.QueueConfigs = append(newConfig.QueueConfigs, q)
		}
	}

	// Add the new queues from this resource's config
	for _, c := range bucketNotificationConfig.Configuration.QueueConfigs {
		newConfig.AddQueue(c.Config)
	}

	err = bucketNotificationConfig.MinioClient.SetBucketNotification(ctx, bucketName, newConfig)
	if err != nil {
		return NewResourceError("error putting bucket notification configuration", bucketName, err)
	}

	// Write back the queue IDs into state so identity is stable.
	// The server assigns IDs to queues that didn't have one; we need to capture
	// those so subsequent reads and deletes work correctly.
	// Convert QueueConfig -> Config for the write-back helper.
	configList := make([]notification.Config, len(bucketNotificationConfig.Configuration.QueueConfigs))
	for i, qc := range bucketNotificationConfig.Configuration.QueueConfigs {
		configList[i] = qc.Config
	}
	if err := writeBackQueueIDs(configList, d); err != nil {
		return NewResourceError("writing back queue IDs", bucketName, err)
	}

	d.SetId(generateBucketNotificationID(bucketName, d))

	return nil
}

func minioReadBucketNotification(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketNotificationConfig := BucketNotificationConfig(d, meta)

	bucketName := d.Get("bucket").(string)
	tflog.Debug(ctx, fmt.Sprintf("S3 bucket notification configuration, read for bucket: %s", bucketName))

	client := meta.(*S3MinioClient)
	notificationConfig, err := bucketNotificationConfig.MinioClient.GetBucketNotification(ctx, bucketName)
	if err != nil {
		if isS3CompatNotSupported(client, err) {
			tflog.Info(ctx, "Bucket notification not supported by backend; skipping")
			return nil
		}
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "NoSuchBucket") {
			tflog.Warn(ctx, fmt.Sprintf("Bucket %s no longer exists, removing notification resource from state", d.Id()))
			d.SetId("")
			return nil
		}
		return NewResourceError("failed to load bucket notification configuration", d.Id(), err)
	}

	if err := d.Set("bucket", bucketName); err != nil {
		return NewResourceError("setting bucket", d.Id(), err)
	}

	// Only set the queue(s) that belong to this resource by matching queue IDs.
	// Since multiple resources can target the same bucket, each resource must
	// only manage its own queue entries — setting all queues would cause
	// resources to overwrite each other's state on Read.
	resourceQueueIDs := getQueueIDsFromResource(d)
	filteredConfigs := filterQueueConfigsByIDs(notificationConfig.QueueConfigs, resourceQueueIDs)
	if err := d.Set("queue", flattenQueueNotificationConfiguration(filteredConfigs)); err != nil {
		return NewResourceError("failed to load bucket queue notifications", d.Id(), err)
	}

	return nil
}

func minioDeleteBucketNotification(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketNotificationConfig := BucketNotificationConfig(d, meta)

	bucketName := d.Get("bucket").(string)
	tflog.Debug(ctx, fmt.Sprintf("S3 bucket: %s, removing notification configuration", bucketName))

	// Lock to prevent concurrent read-modify-write races.
	bucketNotificationLock.Lock(bucketName)
	defer bucketNotificationLock.Unlock(bucketName)

	// Read current config, remove only this resource's queues, write back.
	// This avoids clobbering other resources' notifications on the same bucket.
	currentConfig, err := bucketNotificationConfig.MinioClient.GetBucketNotification(ctx, bucketName)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "NoSuchBucket") {
			tflog.Warn(ctx, fmt.Sprintf("Bucket %s no longer exists, considering notification deletion successful", bucketName))
			return nil
		}
		return NewResourceError("error reading bucket notifications before deletion", bucketName, err)
	}

	resourceQueueIDs := getQueueIDsFromResource(d)

	// Build a new config preserving non-queue configs and queues not belonging to this resource
	newConfig := notification.Configuration{}
	newConfig.LambdaConfigs = currentConfig.LambdaConfigs
	newConfig.TopicConfigs = currentConfig.TopicConfigs

	// Append the original struct verbatim to preserve the Queue (ARN) field.
	for _, q := range currentConfig.QueueConfigs {
		keep := true
		for _, id := range resourceQueueIDs {
			if q.ID == id {
				keep = false
				break
			}
		}
		if keep {
			newConfig.QueueConfigs = append(newConfig.QueueConfigs, q)
		}
	}

	err = bucketNotificationConfig.MinioClient.SetBucketNotification(ctx, bucketName, newConfig)
	if err != nil {
		return NewResourceError("error removing bucket notifications", bucketName, err)
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

func importBucketNotification(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	// Import ID format: "bucket|queue_id1,queue_id2,..." or just "bucket" for backward compatibility
	id := d.Id()
	bucketName := id
	var resourceQueueIDs []string
	if idx := strings.Index(id, "|"); idx != -1 {
		bucketName = id[:idx]
		rest := id[idx+1:]
		if rest != "" {
			resourceQueueIDs = strings.Split(rest, ",")
		}
	}
	if err := d.Set("bucket", bucketName); err != nil {
		return nil, fmt.Errorf("setting bucket during import: %w", err)
	}

	// Read the bucket's notification config to populate queue data.
	m := meta.(*S3MinioClient)
	notificationConfig, err := m.S3Client.GetBucketNotification(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("reading bucket notification during import: %w", err)
	}

	// When the import ID includes queue IDs (bucket|id1,id2), only import those
	// specific queues to avoid claiming ownership of queues managed by sibling
	// resources. When the ID is a bare bucket name, import all queues for
	// backward compatibility with single-resource setups.
	configs := notificationConfig.QueueConfigs
	if resourceQueueIDs != nil {
		configs = filterQueueConfigsByIDs(configs, resourceQueueIDs)
	}
	if err := d.Set("queue", flattenQueueNotificationConfiguration(configs)); err != nil {
		return nil, fmt.Errorf("setting queue during import: %w", err)
	}

	d.SetId(generateBucketNotificationID(bucketName, d))

	return []*schema.ResourceData{d}, nil
}

func generateBucketNotificationID(bucket string, d *schema.ResourceData) string {
	queueIDs := make([]string, 0)
	for _, q := range d.Get("queue").([]interface{}) {
		if c, ok := q.(map[string]interface{}); ok {
			if qid, ok := c["id"].(string); ok && qid != "" {
				queueIDs = append(queueIDs, qid)
			}
		}
	}
	return fmt.Sprintf("%s|%s", bucket, strings.Join(queueIDs, ","))
}

// getQueueIDsFromResource extracts the queue IDs defined in the resource's queue blocks.
func getQueueIDsFromResource(d *schema.ResourceData) []string {
	ids := make([]string, 0)
	for _, q := range d.Get("queue").([]interface{}) {
		if c, ok := q.(map[string]interface{}); ok {
			if qid, ok := c["id"].(string); ok && qid != "" {
				ids = append(ids, qid)
			}
		}
	}
	return ids
}

// getQueueIDsFromState extracts the queue IDs from the *old* state (before update).
// Used during Update to identify which queues to remove before adding new ones.
func getQueueIDsFromState(d *schema.ResourceData) []string {
	oldQueues, _ := d.GetChange("queue")
	oldQueueList := oldQueues.([]interface{})
	ids := make([]string, 0, len(oldQueueList))
	for _, q := range oldQueueList {
		if c, ok := q.(map[string]interface{}); ok {
			if qid, ok := c["id"].(string); ok && qid != "" {
				ids = append(ids, qid)
			}
		}
	}
	return ids
}

// filterQueueConfigsByIDs returns only the queue configs whose ID is in the given set.
func filterQueueConfigsByIDs(configs []notification.QueueConfig, ids []string) []notification.QueueConfig {
	idSet := make(map[string]struct{}, len(ids))
	for _, qid := range ids {
		idSet[qid] = struct{}{}
	}
	result := make([]notification.QueueConfig, 0, len(configs))
	for _, c := range configs {
		if _, ok := idSet[c.ID]; ok {
			result = append(result, c)
		}
	}
	return result
}

// writeBackQueueIDs writes the queue IDs back into the resource state.
// It correlates by list index: the i-th queue block sent maps to the i-th queue block in state.
// This avoids collisions when multiple resources share the same ARN.
func writeBackQueueIDs(sent []notification.Config, d *schema.ResourceData) error {
	queues := d.Get("queue").([]interface{})
	if len(sent) != len(queues) {
		return fmt.Errorf("queue count mismatch: sent %d, state %d", len(sent), len(queues))
	}
	for i := range queues {
		queues[i].(map[string]interface{})["id"] = sent[i].ID
	}
	if err := d.Set("queue", queues); err != nil {
		return fmt.Errorf("setting queue IDs during writeback: %w", err)
	}
	return nil
}

func validateMinioArn(v interface{}, p cty.Path) (errors diag.Diagnostics) {
	value := v.(string)
	_, err := notification.NewArnFromString(value)

	if err != nil {
		return diag.Errorf("value: %s is not a valid ARN", value)
	}

	return nil
}
