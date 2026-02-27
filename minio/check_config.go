package minio

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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

	return &S3MinioConfig{
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
	}
}

// ServiceAccountConfig creates configuration for MinIO service accounts.
// It handles service account creation and management.
func ServiceAccountConfig(d *schema.ResourceData, meta interface{}) *S3MinioServiceAccountConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioServiceAccountConfig{
		MinioAdmin:       m.S3Admin,
		MinioAccessKey:   getOptionalField(d, "access_key", "").(string),
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

// IdpOpenIdConfig creates configuration for an OpenID Connect identity provider resource.
func IdpOpenIdConfig(d *schema.ResourceData, meta interface{}) *S3MinioIdpOpenId {
	m := meta.(*S3MinioClient)

	return &S3MinioIdpOpenId{
		MinioAdmin:   m.S3Admin,
		Name:         getOptionalField(d, "name", "_").(string),
		ConfigURL:    getOptionalField(d, "config_url", "").(string),
		ClientID:     getOptionalField(d, "client_id", "").(string),
		ClientSecret: getOptionalField(d, "client_secret", "").(string),
		ClaimName:    getOptionalField(d, "claim_name", "policy").(string),
		ClaimPrefix:  getOptionalField(d, "claim_prefix", "").(string),
		Scopes:       getOptionalField(d, "scopes", "").(string),
		RedirectURI:  getOptionalField(d, "redirect_uri", "").(string),
		DisplayName:  getOptionalField(d, "display_name", "").(string),
		Comment:      getOptionalField(d, "comment", "").(string),
		RolePolicy:   getOptionalField(d, "role_policy", "").(string),
		Enable:       getOptionalField(d, "enable", true).(bool),
	}
}
