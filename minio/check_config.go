package minio

import (
	"fmt"
	"log"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7/pkg/notification"
	"github.com/minio/minio-go/v7/pkg/sse"
)

// ConfigError represents an error that occurred during configuration
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("configuration error for field %q: %s", e.Field, e.Message)
}

// getOptionalField safely gets an optional field from the ResourceData with a default value
func getOptionalField(d *schema.ResourceData, field string, defaultValue interface{}) interface{} {
	if v, ok := d.GetOk(field); ok {
		return v
	}
	return defaultValue
}

// getBucketVersioningConfig parses versioning configuration from Terraform schema
func getBucketVersioningConfig(v []interface{}) *S3MinioBucketVersioningConfiguration {
	if len(v) == 0 || v[0] == nil {
		return nil
	}

	tfMap, ok := v[0].(map[string]interface{})
	if !ok {
		return nil
	}

	result := &S3MinioBucketVersioningConfiguration{}

	if status, ok := tfMap["status"].(string); ok && status != "" {
		result.Status = status
	}

	if excludedPrefixes, ok := tfMap["excluded_prefixes"].([]interface{}); ok {
		for _, prefix := range excludedPrefixes {
			if v, ok := prefix.(string); ok {
				result.ExcludedPrefixes = append(result.ExcludedPrefixes, v)
			}
		}
	}

	if excludeFolders, ok := tfMap["exclude_folders"].(bool); ok {
		result.ExcludeFolders = excludeFolders
	}

	return result
}

// getBucketReplicationConfig parses replication configuration from Terraform schema
func getBucketReplicationConfig(v []interface{}, d *schema.ResourceData) (result []S3MinioBucketReplicationRule, errs diag.Diagnostics) {
	if len(v) == 0 || v[0] == nil {
		return
	}

	result = make([]S3MinioBucketReplicationRule, len(v))
	for i, rule := range v {
		var ok bool
		tfMap, ok := rule.(map[string]interface{})
		if !ok {
			errs = append(errs, diag.Errorf("Unable to extract the rule %d", i)...)
			continue
		}
		log.Printf("[DEBUG] rule[%d] contains %v", i, tfMap)

		result[i].Arn, _ = tfMap["arn"].(string)
		result[i].Id, _ = tfMap["id"].(string)

		if result[i].Enabled, ok = tfMap["enabled"].(bool); !ok {
			log.Printf("[DEBUG] rule[%d].enabled omitted. Defaulting to true", i)
			result[i].Enabled = true
		}

		if result[i].Priority, ok = tfMap["priority"].(int); !ok || result[i].Priority == 0 {
			result[i].Priority = -len(v) + i
			log.Printf("[DEBUG] rule[%d].priority omitted. Defaulting to index (%d)", i, -result[i].Priority)
		}

		result[i].Prefix, _ = tfMap["prefix"].(string)

		if tags, ok := tfMap["tags"].(map[string]interface{}); ok {
			log.Printf("[DEBUG] rule[%d].tags map contains: %v", i, tags)
			tagMap := map[string]string{}
			for k, val := range tags {
				var valOk bool
				tagMap[k], valOk = val.(string)
				if !valOk {
					errs = append(errs, diag.Errorf("rule[%d].tags[%s] value must be a string, not a %s", i, k, reflect.TypeOf(val))...)
				}
			}
			result[i].Tags = tagMap
		} else {
			errs = append(errs, diag.Errorf("unable to extract rule[%d].tags of type %s", i, reflect.TypeOf(tfMap["tags"]))...)
		}

		log.Printf("[DEBUG] rule[%d].tags are: %v", i, result[i].Tags)

		result[i].DeleteReplication, ok = tfMap["delete_replication"].(bool)
		result[i].DeleteReplication = result[i].DeleteReplication && ok
		result[i].DeleteMarkerReplication, ok = tfMap["delete_marker_replication"].(bool)
		result[i].DeleteMarkerReplication = result[i].DeleteMarkerReplication && ok
		result[i].ExistingObjectReplication, ok = tfMap["existing_object_replication"].(bool)
		result[i].ExistingObjectReplication = result[i].ExistingObjectReplication && ok
		result[i].MetadataSync, ok = tfMap["metadata_sync"].(bool)
		result[i].MetadataSync = result[i].MetadataSync && ok

		var targets []interface{}
		if targets, ok = tfMap["target"].([]interface{}); !ok || len(targets) != 1 {
			errs = append(errs, diag.Errorf("Unexpected value type for rule[%d].target. Exactly one target configuration is expected", i)...)
			continue
		}
		var target map[string]interface{}
		if target, ok = targets[0].(map[string]interface{}); !ok {
			errs = append(errs, diag.Errorf("Unexpected value type for rule[%d].target. Unable to convert to a usable type", i)...)
			continue
		}

		if result[i].Target.Bucket, ok = target["bucket"].(string); !ok {
			errs = append(errs, diag.Errorf("rule[%d].target.bucket cannot be omitted", i)...)
		}

		result[i].Target.StorageClass, _ = target["storage_class"].(string)

		if result[i].Target.Host, ok = target["host"].(string); !ok {
			errs = append(errs, diag.Errorf("rule[%d].target.host cannot be omitted", i)...)
		}

		result[i].Target.Path, _ = target["path"].(string)
		result[i].Target.Region, _ = target["region"].(string)

		if result[i].Target.AccessKey, ok = target["access_key"].(string); !ok {
			errs = append(errs, diag.Errorf("rule[%d].target.access_key cannot be omitted", i)...)
		}

		if result[i].Target.SecretKey, ok = target["secret_key"].(string); !ok {
			errs = append(errs, diag.Errorf("rule[%d].target.secret_key cannot be omitted", i)...)
		}

		if result[i].Target.Secure, ok = target["secure"].(bool); !result[i].Target.Secure || !ok {
			errs = append(errs, diag.Diagnostic{
				Severity: diag.Warning,
				Summary:  fmt.Sprintf("rule[%d].target.secure is false. It is unsafe to use bucket replication over HTTP", i),
			})
		}

		rawConfig := d.GetRawConfig()
		syncConfigured := false
		if !rawConfig.IsNull() {
			rulesAttr := rawConfig.GetAttr("rule")
			if !rulesAttr.IsNull() && rulesAttr.IsKnown() && rulesAttr.LengthInt() > i {
				ruleVal := rulesAttr.Index(cty.NumberIntVal(int64(i)))
				if !ruleVal.IsNull() && ruleVal.IsKnown() {
					targetAttr := ruleVal.GetAttr("target")
					if !targetAttr.IsNull() && targetAttr.IsKnown() && targetAttr.LengthInt() > 0 {
						targetVal := targetAttr.Index(cty.NumberIntVal(0))
						if !targetVal.IsNull() && targetVal.IsKnown() {
							syncAttr := targetVal.GetAttr("synchronous")
							if !syncAttr.IsNull() && syncAttr.IsKnown() {
								result[i].Target.Synchronous = syncAttr.True()
								syncConfigured = true
							} else {
								syncronousAttr := targetVal.GetAttr("syncronous")
								if !syncronousAttr.IsNull() && syncronousAttr.IsKnown() {
									result[i].Target.Synchronous = syncronousAttr.True()
									syncConfigured = true
								}
							}
						}
					}
				}
			}
		}
		if !syncConfigured {
			if v, ok := target["synchronous"].(bool); ok {
				result[i].Target.Synchronous = v
			} else if v, ok := target["syncronous"].(bool); ok {
				result[i].Target.Synchronous = v
			}
		}
		result[i].Target.DisableProxy, ok = target["disable_proxy"].(bool)
		result[i].Target.DisableProxy = result[i].Target.DisableProxy && ok

		var bandwidth uint64
		var err error
		bandwidth, ok, parseDiags := ParseBandwidthLimit(target)
		if len(parseDiags) > 0 {
			errs = append(errs, diag.Errorf("rule[%d].target.bandwidth_limit is invalid. Make sure to use k, m, g as prefix only", i)...)
		}

		if ok {
			var bwLimit int64
			if bandwidth > uint64(math.MaxInt64) {
				log.Printf("[WARN] Configured bandwidth limit (%d) exceeds maximum supported value (%d), clamping.", bandwidth, int64(math.MaxInt64))
				bwLimit = math.MaxInt64
			} else {
				bwLimit = int64(bandwidth)
			}
			result[i].Target.BandwidthLimit = bwLimit
		}

		var healthcheckDuration string
		if healthcheckDuration, ok = target["health_check_period"].(string); ok {
			result[i].Target.HealthCheckPeriod, err = time.ParseDuration(healthcheckDuration)
			if err != nil {
				log.Printf("[WARN] invalid healthcheck value %q: %v", result[i].Target.HealthCheckPeriod, err)
				errs = append(errs, diag.Errorf("rule[%d].target.health_check_period is invalid. Make sure to use a valid golang time duration notation", i)...)
			}
		}

		var pathstyle string
		pathstyle, _ = target["path_style"].(string)
		switch strings.TrimSpace(strings.ToLower(pathstyle)) {
		case "on":
			result[i].Target.PathStyle = S3PathStyleOn
		case "off":
			result[i].Target.PathStyle = S3PathStyleOff
		default:
			if pathstyle != "auto" && pathstyle != "" {
				errs = append(errs, diag.Diagnostic{
					Severity: diag.Warning,
					Summary:  fmt.Sprintf("rule[%d].target.path_style must be \"on\", \"off\" or \"auto\". Defaulting to \"auto\"", i),
				})
			}
			result[i].Target.PathStyle = S3PathStyleAuto
		}

	}
	return
}

// BucketConfig creates a new configuration for MinIO buckets.
// It handles the basic bucket configuration including ACL, prefixes, and object locking.
func BucketConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucket {
	m := meta.(*S3MinioClient)

	return &S3MinioBucket{
		MinioClient:          m.S3Client,
		MinioAdmin:           m.S3Admin,
		MinioRegion:          m.S3Region,
		MinioAccess:          m.S3UserAccess,
		MinioBucket:          getOptionalField(d, "bucket", "").(string),
		MinioBucketPrefix:    getOptionalField(d, "bucket_prefix", "").(string),
		MinioACL:             getOptionalField(d, "acl", "private").(string),
		MinioForceDestroy:    getOptionalField(d, "force_destroy", false).(bool),
		ObjectLockingEnabled: getOptionalField(d, "object_locking", false).(bool),
		SkipBucketTagging:    m.SkipBucketTagging,
		S3CompatMode:         m.S3CompatMode,
	}
}

// BucketVersioningConfig creates configuration for managing MinIO bucket versioning.
// It handles versioning configuration including excluded prefixes and folders.
func BucketVersioningConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketVersioning {
	m := meta.(*S3MinioClient)

	versioningConfig := getBucketVersioningConfig(d.Get("versioning_configuration").([]interface{}))

	return &S3MinioBucketVersioning{
		MinioClient:             m.S3Client,
		MinioBucket:             getOptionalField(d, "bucket", "").(string),
		VersioningConfiguration: versioningConfig,
	}
}

// BucketReplicationConfig creates configuration for managing MinIO bucket replication.
// It sets up replication rules between buckets.
func BucketReplicationConfig(d *schema.ResourceData, meta interface{}) (*S3MinioBucketReplication, diag.Diagnostics) {
	m := meta.(*S3MinioClient)

	replicationRules, diags := getBucketReplicationConfig(d.Get("rule").([]interface{}), d)
	if diags.HasError() {
		return nil, diags
	}

	return &S3MinioBucketReplication{
		MinioClient:      m.S3Client,
		MinioAdmin:       m.S3Admin,
		MinioBucket:      getOptionalField(d, "bucket", "").(string),
		ReplicationRules: replicationRules,
	}, nil
}

// getNotificationConfiguration parses notification configuration from Terraform schema
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

// BucketNotificationConfig creates configuration for managing MinIO bucket notifications.
// It sets up event notifications for bucket operations.
func BucketNotificationConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketNotification {
	m := meta.(*S3MinioClient)
	config := getNotificationConfiguration(d)

	return &S3MinioBucketNotification{
		MinioClient:   m.S3Client,
		MinioBucket:   getOptionalField(d, "bucket", "").(string),
		Configuration: &config,
	}
}

func BucketCorsConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketCors {
	m := meta.(*S3MinioClient)

	return &S3MinioBucketCors{
		MinioClient: m.S3Client,
		MinioBucket: getOptionalField(d, "bucket", "").(string),
	}
}

// getBucketServerSideEncryptionConfig parses encryption configuration from Terraform schema
func getBucketServerSideEncryptionConfig(d *schema.ResourceData) *sse.Configuration {
	encryptionType := d.Get("encryption_type").(string)

	if encryptionType == "AES256" {
		return sse.NewConfigurationSSES3()
	}

	keyID, _ := d.Get("kms_key_id").(string)
	return sse.NewConfigurationSSEKMS(keyID)
}

// BucketServerSideEncryptionConfig creates configuration for managing MinIO bucket server-side encryption.
// It handles encryption settings for bucket objects.
func BucketServerSideEncryptionConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketServerSideEncryption {
	m := meta.(*S3MinioClient)

	encryptionConfig := getBucketServerSideEncryptionConfig(d)

	return &S3MinioBucketServerSideEncryption{
		MinioClient:   m.S3Client,
		MinioBucket:   getOptionalField(d, "bucket", "").(string),
		Configuration: encryptionConfig,
	}
}

// BucketObjectLockConfigurationConfig extracts object lock config from resource data.
func BucketObjectLockConfigurationConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketObjectLockConfiguration {
	m := meta.(*S3MinioClient)

	return &S3MinioBucketObjectLockConfiguration{
		MinioClient:       m.S3Client,
		MinioBucket:       getOptionalField(d, "bucket", "").(string),
		ObjectLockEnabled: getOptionalField(d, "object_lock_enabled", "Enabled").(string),
	}
}

// BucketPolicyConfig creates configuration for managing MinIO bucket policies.
func BucketPolicyConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketPolicy {
	m := meta.(*S3MinioClient)

	return &S3MinioBucketPolicy{
		MinioClient:       m.S3Client,
		MinioBucket:       getOptionalField(d, "bucket", "").(string),
		MinioBucketPolicy: getOptionalField(d, "policy", "").(string),
	}
}

// NewConfig creates a new MinIO client configuration.
// It handles authentication and connection settings.
func NewConfig(d *schema.ResourceData) *S3MinioConfig {
	// Get user credentials with fallback to legacy access key
	user := getOptionalField(d, "minio_user", "").(string)
	if user == "" {
		user = getOptionalField(d, "minio_access_key", "").(string)
	}

	// Get password with fallback to legacy secret key
	password := getOptionalField(d, "minio_password", "").(string)
	if password == "" {
		password = getOptionalField(d, "minio_secret_key", "").(string)
	}

	cfg := &S3MinioConfig{
		S3HostPort:        getOptionalField(d, "minio_server", "").(string),
		S3Region:          getOptionalField(d, "minio_region", "us-east-1").(string),
		S3UserAccess:      user,
		S3UserSecret:      password,
		S3SessionToken:    getOptionalField(d, "minio_session_token", "").(string),
		S3APISignature:    getOptionalField(d, "minio_api_version", "v4").(string),
		S3SSL:             getOptionalField(d, "minio_ssl", false).(bool),
		S3SSLCACertFile:   getOptionalField(d, "minio_cacert_file", "").(string),
		S3SSLCertFile:     getOptionalField(d, "minio_cert_file", "").(string),
		S3SSLKeyFile:      getOptionalField(d, "minio_key_file", "").(string),
		S3SSLSkipVerify:   getOptionalField(d, "minio_insecure", false).(bool),
		SkipBucketTagging: getOptionalField(d, "skip_bucket_tagging", false).(bool),
		S3CompatMode:      getOptionalField(d, "s3_compat_mode", false).(bool),
	}

	if v, ok := d.GetOk("assume_role"); ok {
		assumeRoleList := v.([]interface{})
		if len(assumeRoleList) > 0 {
			ar := assumeRoleList[0].(map[string]interface{})
			cfg.AssumeRoleARN = ar["role_arn"].(string)
			cfg.AssumeRoleSessionName = ar["session_name"].(string)
			cfg.AssumeRoleDuration = ar["duration_seconds"].(int)
			if p, ok := ar["policy"].(string); ok {
				cfg.AssumeRolePolicy = p
			}
			if e, ok := ar["external_id"].(string); ok {
				cfg.AssumeRoleExternalID = e
			}
		}
	}

	if v, ok := d.GetOk("assume_role_with_web_identity"); ok {
		wiList := v.([]interface{})
		if len(wiList) > 0 {
			wi := wiList[0].(map[string]interface{})
			cfg.WebIdentityToken = wi["web_identity_token"].(string)
			cfg.WebIdentityTokenFile = wi["web_identity_token_file"].(string)
			cfg.WebIdentityDuration = wi["duration_seconds"].(int)
		}
	}

	return cfg
}

// ServiceAccountConfig creates configuration for MinIO service accounts.
// It handles service account creation and management.
func ServiceAccountConfig(d *schema.ResourceData, meta interface{}) *S3MinioServiceAccountConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioServiceAccountConfig{
		MinioAdmin:       m.S3Admin,
		MinioAccessKey:   getOptionalField(d, "access_key", "").(string),
		MinioSecretKey:   getOptionalField(d, "secret_key", "").(string),
		MinioTargetUser:  getOptionalField(d, "target_user", "").(string),
		MinioDisableUser: getOptionalField(d, "disable_user", false).(bool),
		MinioUpdateKey:   getOptionalField(d, "update_secret", false).(bool),
		MinioSAPolicy:    getOptionalField(d, "policy", "").(string),
		MinioName:        getOptionalField(d, "name", "").(string),
		MinioDescription: getOptionalField(d, "description", "").(string),
		MinioExpiration:  getOptionalField(d, "expiration", "").(string),
	}
}

// IAMUserConfig creates configuration for MinIO IAM users.
// It handles user creation and management in the IAM system.
func IAMUserConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMUserConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMUserConfig{
		MinioAdmin:        m.S3Admin,
		MinioIAMName:      getOptionalField(d, "name", "").(string),
		MinioSecret:       getOptionalField(d, "secret", "").(string),
		MinioDisableUser:  getOptionalField(d, "disable_user", false).(bool),
		MinioUpdateKey:    getOptionalField(d, "update_secret", false).(bool),
		MinioForceDestroy: getOptionalField(d, "force_destroy", false).(bool),
	}
}

// IAMGroupConfig creates configuration for MinIO IAM groups.
// It handles group creation and management.
func IAMGroupConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupConfig{
		MinioAdmin:        m.S3Admin,
		MinioIAMName:      getOptionalField(d, "name", "").(string),
		MinioDisableGroup: getOptionalField(d, "disable_group", false).(bool),
		MinioForceDestroy: getOptionalField(d, "force_destroy", false).(bool),
	}
}

// IAMGroupAttachmentConfig creates configuration for MinIO IAM group attachments.
// It handles attaching a single user to a group.
func IAMGroupAttachmentConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupAttachmentConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupAttachmentConfig{
		MinioAdmin:    m.S3Admin,
		MinioIAMUser:  getOptionalField(d, "user_name", "").(string),
		MinioIAMGroup: getOptionalField(d, "group_name", "").(string),
	}
}

// IAMGroupMembershipConfig creates configuration for MinIO IAM group memberships.
// It handles attaching multiple users to a group.
func IAMGroupMembershipConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupMembershipConfig {
	m := meta.(*S3MinioClient)

	users := getStringList(d.Get("users").(*schema.Set).List())

	return &S3MinioIAMGroupMembershipConfig{
		MinioAdmin:    m.S3Admin,
		MinioIAMName:  getOptionalField(d, "name", "").(string),
		MinioIAMUsers: users,
		MinioIAMGroup: getOptionalField(d, "group", "").(string),
	}
}

// IAMPolicyConfig creates configuration for MinIO IAM policies.
// It handles policy creation and management.
func IAMPolicyConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMPolicyConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMPolicyConfig{
		MinioAdmin:         m.S3Admin,
		MinioIAMName:       getOptionalField(d, "name", "").(string),
		MinioIAMNamePrefix: getOptionalField(d, "name_prefix", "").(string),
		MinioIAMPolicy:     getOptionalField(d, "policy", "").(string),
	}
}

// IAMGroupPolicyConfig creates configuration for MinIO IAM group policies.
// It handles attaching policies to groups.
func IAMGroupPolicyConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupPolicyConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupPolicyConfig{
		MinioAdmin:         m.S3Admin,
		MinioIAMName:       getOptionalField(d, "name", "").(string),
		MinioIAMNamePrefix: getOptionalField(d, "name_prefix", "").(string),
		MinioIAMPolicy:     getOptionalField(d, "policy", "").(string),
		MinioIAMGroup:      getOptionalField(d, "group", "").(string),
	}
}

// KMSKeyConfig creates configuration for MinIO KMS keys.
// It handles key management system configuration.
func KMSKeyConfig(d *schema.ResourceData, meta interface{}) *S3MinioKMSKeyConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioKMSKeyConfig{
		MinioAdmin:    m.S3Admin,
		MinioKMSKeyID: getOptionalField(d, "key_id", "").(string),
	}
}

// ObjectTagsConfig creates configuration for managing object tags.
func ObjectTagsConfig(d *schema.ResourceData, meta interface{}) *S3MinioObjectTags {
	m := meta.(*S3MinioClient)

	return &S3MinioObjectTags{
		MinioClient:    m.S3Client,
		MinioBucket:    getOptionalField(d, "bucket", "").(string),
		MinioObjectKey: getOptionalField(d, "key", "").(string),
	}
}

// ObjectLegalHoldConfig creates configuration for managing object legal hold.
func ObjectLegalHoldConfig(d *schema.ResourceData, meta interface{}) *S3MinioObjectLegalHold {
	m := meta.(*S3MinioClient)

	return &S3MinioObjectLegalHold{
		MinioClient:    m.S3Client,
		MinioBucket:    getOptionalField(d, "bucket", "").(string),
		MinioObjectKey: getOptionalField(d, "key", "").(string),
		MinioVersionID: getOptionalField(d, "version_id", "").(string),
		MinioStatus:    getOptionalField(d, "status", "").(string),
	}
}

// PrometheusBearerTokenConfig creates configuration for MinIO Prometheus bearer token.
func PrometheusBearerTokenConfig(d *schema.ResourceData, meta interface{}) *S3MinioPrometheusBearerToken {
	m := meta.(*S3MinioClient)

	return &S3MinioPrometheusBearerToken{
		MinioAdmin:     m.S3Admin,
		MinioAccessKey: m.S3UserAccess,
		MinioSecretKey: m.S3UserSecret,
		MetricType:     getOptionalField(d, "metric_type", "cluster").(string),
		ExpiresIn:      getOptionalField(d, "expires_in", "87600h").(string),
		Limit:          getOptionalField(d, "limit", 876000).(int),
	}
}

// PrometheusScrapeConfig creates configuration for MinIO Prometheus scrape config.
func PrometheusScrapeConfig(d *schema.ResourceData, meta interface{}) *S3MinioPrometheusScrapeConfig {
	m := meta.(*S3MinioClient)

	payload := &S3MinioPrometheusScrapeConfig{
		MinioEndpoint:  m.S3Endpoint,
		MinioAccessKey: m.S3UserAccess,
		MinioSecretKey: m.S3UserSecret,
		UseSSL:         m.S3SSL,
		MetricType:     getOptionalField(d, "metric_type", "cluster").(string),
		Alias:          getOptionalField(d, "alias", "").(string),
		MetricsVersion: getOptionalField(d, "metrics_version", "v3").(string),
	}

	if val, ok := d.GetOk("bearer_token"); ok {
		payload.BearerToken = val.(string)
	}

	return payload
}

// IdpLdapConfig creates configuration for an LDAP identity provider resource.
func IdpLdapConfig(d *schema.ResourceData, meta interface{}) *S3MinioIdpLdap {
	m := meta.(*S3MinioClient)

	return &S3MinioIdpLdap{
		MinioAdmin:         m.S3Admin,
		ServerAddr:         getOptionalField(d, "server_addr", "").(string),
		LookupBindDN:       getOptionalField(d, "lookup_bind_dn", "").(string),
		LookupBindPassword: getOptionalField(d, "lookup_bind_password", "").(string),
		UserDNSearchBaseDN: getOptionalField(d, "user_dn_search_base_dn", "").(string),
		UserDNSearchFilter: getOptionalField(d, "user_dn_search_filter", "").(string),
		GroupSearchBaseDN:  getOptionalField(d, "group_search_base_dn", "").(string),
		GroupSearchFilter:  getOptionalField(d, "group_search_filter", "").(string),
		TLSSkipVerify:      getOptionalField(d, "tls_skip_verify", false).(bool),
		ServerInsecure:     getOptionalField(d, "server_insecure", false).(bool),
		StartTLS:           getOptionalField(d, "starttls", false).(bool),
		Enable:             getOptionalField(d, "enable", true).(bool),
	}
}

// IdpOpenIdConfig creates configuration for an OpenID Connect identity provider resource.
func IdpOpenIdConfig(d *schema.ResourceData, meta interface{}) *S3MinioIdpOpenId {
	m := meta.(*S3MinioClient)

	return &S3MinioIdpOpenId{
		MinioAdmin:   m.S3Admin,
		Name:         getOptionalField(d, "name", "_").(string),
		ConfigURL:    getOptionalField(d, "config_url", "").(string),
		ClientID:     getOptionalField(d, "client_id", "").(string),
		ClientSecret: getOptionalField(d, "client_secret", "").(string),
		ClaimName:    getOptionalField(d, "claim_name", "").(string),
		ClaimPrefix:  getOptionalField(d, "claim_prefix", "").(string),
		Scopes:       getOptionalField(d, "scopes", "").(string),
		RedirectURI:  getOptionalField(d, "redirect_uri", "").(string),
		DisplayName:  getOptionalField(d, "display_name", "").(string),
		Comment:      getOptionalField(d, "comment", "").(string),
		RolePolicy:   getOptionalField(d, "role_policy", "").(string),
		Enable:       getOptionalField(d, "enable", true).(bool),
	}
}

// AuditWebhookConfig creates configuration for an audit webhook resource.
func AuditWebhookConfig(d *schema.ResourceData, meta interface{}) *S3MinioAuditWebhook {
	m := meta.(*S3MinioClient)

	return &S3MinioAuditWebhook{
		MinioAdmin: m.S3Admin,
		Name:       d.Get("name").(string),
		Endpoint:   d.Get("endpoint").(string),
		AuthToken:  getOptionalField(d, "auth_token", "").(string),
		Enable:     getOptionalField(d, "enable", true).(bool),
		QueueSize:  getOptionalField(d, "queue_size", 0).(int),
		BatchSize:  getOptionalField(d, "batch_size", 0).(int),
		ClientCert: getOptionalField(d, "client_cert", "").(string),
		ClientKey:  getOptionalField(d, "client_key", "").(string),
	}
}

// parseConfigParams parses a config string like "key1=value1 key2=value2" into a map
func parseConfigParams(configStr string) map[string]string {
	result := make(map[string]string)
	if configStr == "" {
		return result
	}

	pairs := strings.Fields(configStr)
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

// parseInt parses a string into an integer
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// toEnableFlag converts a boolean to "enable"/"disable" string
func toEnableFlag(b bool) string {
	if b {
		return "enable"
	}
	return "disable"
}
