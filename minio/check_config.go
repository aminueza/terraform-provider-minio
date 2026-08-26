package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("configuration error for field %q: %s", e.Field, e.Message)
}

func getOptionalField(d *schema.ResourceData, field string, defaultValue interface{}) interface{} {
	if v, ok := d.GetOk(field); ok {
		return v
	}
	return defaultValue
}

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

func BucketVersioningConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketVersioning {
	m := meta.(*S3MinioClient)

	versioningConfig := getBucketVersioningConfig(d.Get("versioning_configuration").([]interface{}))

	return &S3MinioBucketVersioning{
		MinioClient:             m.S3Client,
		MinioBucket:             getOptionalField(d, "bucket", "").(string),
		VersioningConfiguration: versioningConfig,
	}
}

func BucketReplicationConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) (*S3MinioBucketReplication, diag.Diagnostics) {
	m := meta.(*S3MinioClient)

	replicationRules, diags := getBucketReplicationConfig(ctx, d.Get("rule").([]interface{}), d)
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

func BucketServerSideEncryptionConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketServerSideEncryption {
	m := meta.(*S3MinioClient)

	encryptionConfig := getBucketServerSideEncryptionConfig(d)

	return &S3MinioBucketServerSideEncryption{
		MinioClient:   m.S3Client,
		MinioBucket:   getOptionalField(d, "bucket", "").(string),
		Configuration: encryptionConfig,
	}
}

func BucketObjectLockConfigurationConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketObjectLockConfiguration {
	m := meta.(*S3MinioClient)

	return &S3MinioBucketObjectLockConfiguration{
		MinioClient:       m.S3Client,
		MinioBucket:       getOptionalField(d, "bucket", "").(string),
		ObjectLockEnabled: getOptionalField(d, "object_lock_enabled", "Enabled").(string),
	}
}

func BucketPolicyConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketPolicy {
	m := meta.(*S3MinioClient)

	return &S3MinioBucketPolicy{
		MinioClient:       m.S3Client,
		MinioBucket:       getOptionalField(d, "bucket", "").(string),
		MinioBucketPolicy: getOptionalField(d, "policy", "").(string),
	}
}

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
		S3HostPort:            getOptionalField(d, "minio_server", "").(string),
		S3Region:              getOptionalField(d, "minio_region", "us-east-1").(string),
		S3UserAccess:          user,
		S3UserSecret:          password,
		S3SessionToken:        getOptionalField(d, "minio_session_token", "").(string),
		S3APISignature:        getOptionalField(d, "minio_api_version", "v4").(string),
		S3SSL:                 getOptionalField(d, "minio_ssl", false).(bool),
		S3SSLCACertFile:       getOptionalField(d, "minio_cacert_file", "").(string),
		S3SSLCertFile:         getOptionalField(d, "minio_cert_file", "").(string),
		S3SSLKeyFile:          getOptionalField(d, "minio_key_file", "").(string),
		S3SSLSkipVerify:       getOptionalField(d, "minio_insecure", false).(bool),
		SkipBucketTagging:     getOptionalField(d, "skip_bucket_tagging", false).(bool),
		S3CompatMode:          getOptionalField(d, "s3_compat_mode", false).(bool),
		Edition:               getOptionalField(d, "minio_edition", "").(string),
		RequestTimeoutSeconds: getOptionalField(d, "request_timeout_seconds", 30).(int),
		MaxRetries:            getOptionalField(d, "max_retries", 6).(int),
		RetryDelayMs:          getOptionalField(d, "retry_delay_ms", 1000).(int),
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

func IAMGroupConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupConfig{
		MinioAdmin:        m.S3Admin,
		MinioIAMName:      getOptionalField(d, "name", "").(string),
		MinioDisableGroup: getOptionalField(d, "disable_group", false).(bool),
		MinioForceDestroy: getOptionalField(d, "force_destroy", false).(bool),
	}
}

func IAMGroupAttachmentConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupAttachmentConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupAttachmentConfig{
		MinioAdmin:    m.S3Admin,
		MinioIAMUser:  getOptionalField(d, "user_name", "").(string),
		MinioIAMGroup: getOptionalField(d, "group_name", "").(string),
	}
}

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

func IAMPolicyConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMPolicyConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMPolicyConfig{
		MinioAdmin:         m.S3Admin,
		MinioIAMName:       getOptionalField(d, "name", "").(string),
		MinioIAMNamePrefix: getOptionalField(d, "name_prefix", "").(string),
		MinioIAMPolicy:     getOptionalField(d, "policy", "").(string),
	}
}

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

func KMSKeyConfig(d *schema.ResourceData, meta interface{}) *S3MinioKMSKeyConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioKMSKeyConfig{
		MinioAdmin:    m.S3Admin,
		MinioKMSKeyID: getOptionalField(d, "key_id", "").(string),
	}
}

func ObjectTagsConfig(d *schema.ResourceData, meta interface{}) *S3MinioObjectTags {
	m := meta.(*S3MinioClient)

	return &S3MinioObjectTags{
		MinioClient:    m.S3Client,
		MinioBucket:    getOptionalField(d, "bucket", "").(string),
		MinioObjectKey: getOptionalField(d, "key", "").(string),
	}
}

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

func IAMImportConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMImport {
	m := meta.(*S3MinioClient)
	return &S3MinioIAMImport{
		MinioAdmin: m.S3Admin,
		IAMData:    getOptionalField(d, "iam_data", "").(string),
	}
}

func IncompleteUploadCleanupConfig(d *schema.ResourceData, meta interface{}) *S3MinioIncompleteUploadCleanup {
	m := meta.(*S3MinioClient)

	bucket := getOptionalField(d, "bucket", "").(string)
	prefix := getOptionalField(d, "prefix", "").(string)

	if bucket == "" && d.Id() != "" {
		id := d.Id()
		if idx := strings.Index(id, "/"); idx != -1 {
			bucket = id[:idx]
			prefix = id[idx+1:]
		} else {
			bucket = id
		}
	}

	return &S3MinioIncompleteUploadCleanup{
		MinioClient: m.S3Client,
		MinioBucket: bucket,
		MinioPrefix: prefix,
	}
}

func BatchJobConfig(d *schema.ResourceData, meta interface{}) *S3MinioBatchJob {
	m := meta.(*S3MinioClient)

	return &S3MinioBatchJob{
		MinioAdmin: m.S3Admin,
		JobType:    getOptionalField(d, "job_type", "").(string),
		JobYAML:    getOptionalField(d, "job_yaml", "").(string),
	}
}
