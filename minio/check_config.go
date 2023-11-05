package minio

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// BucketConfig creates a new config for minio buckets
func BucketConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucket {
	m := meta.(*S3MinioClient)

	return &S3MinioBucket{
		MinioClient:          m.S3Client,
		MinioAdmin:           m.S3Admin,
		MinioRegion:          m.S3Region,
		MinioAccess:          m.S3UserAccess,
		MinioBucket:          d.Get("bucket").(string),
		MinioBucketPrefix:    d.Get("bucket_prefix").(string),
		MinioACL:             d.Get("acl").(string),
		MinioForceDestroy:    d.Get("force_destroy").(bool),
		ObjectLockingEnabled: d.Get("object_locking").(bool),
	}
}

// BucketPolicyConfig creates config for managing minio bucket policies
func BucketPolicyConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketPolicy {
	m := meta.(*S3MinioClient)

	return &S3MinioBucketPolicy{
		MinioClient:       m.S3Client,
		MinioBucket:       d.Get("bucket").(string),
		MinioBucketPolicy: d.Get("policy").(string),
	}
}

// BucketVersioningConfig creates config for managing minio bucket versioning
func BucketVersioningConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketVersioning {
	m := meta.(*S3MinioClient)

	versioningConfig := getBucketVersioningConfig(d.Get("versioning_configuration").([]interface{}))

	return &S3MinioBucketVersioning{
		MinioClient:             m.S3Client,
		MinioBucket:             d.Get("bucket").(string),
		VersioningConfiguration: versioningConfig,
	}
}

// BucketVersioningConfig creates config for managing minio bucket versioning
func BucketReplicationConfig(d *schema.ResourceData, meta interface{}) (*S3MinioBucketReplication, diag.Diagnostics) {
	m := meta.(*S3MinioClient)

	replicationRules, diags := getBucketReplicationConfig(d.Get("rule").([]interface{}))

	return &S3MinioBucketReplication{
		MinioClient:      m.S3Client,
		MinioAdmin:       m.S3Admin,
		MinioBucket:      d.Get("bucket").(string),
		ReplicationRules: replicationRules,
	}, diags
}

// BucketNotificationConfig creates config for managing minio bucket notifications
func BucketNotificationConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketNotification {
	m := meta.(*S3MinioClient)
	config := getNotificationConfiguration(d)

	return &S3MinioBucketNotification{
		MinioClient:   m.S3Client,
		MinioBucket:   d.Get("bucket").(string),
		Configuration: &config,
	}
}

// BucketServerSideEncryptionConfig creates config for managing minio bucket server side encryption
func BucketServerSideEncryptionConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketServerSideEncryption {
	m := meta.(*S3MinioClient)

	encryptionConfig := getBucketServerSideEncryptionConfig(d)

	return &S3MinioBucketServerSideEncryption{
		MinioClient:   m.S3Client,
		MinioBucket:   d.Get("bucket").(string),
		Configuration: encryptionConfig,
	}
}

// NewConfig creates a new config for minio
func NewConfig(d *schema.ResourceData) *S3MinioConfig {
	user := d.Get("minio_user").(string)
	if user == "" {
		user = d.Get("minio_access_key").(string)
	}

	password := d.Get("minio_password").(string)
	if password == "" {
		password = d.Get("minio_secret_key").(string)
	}

	return &S3MinioConfig{
		S3HostPort:      d.Get("minio_server").(string),
		S3Region:        d.Get("minio_region").(string),
		S3UserAccess:    user,
		S3UserSecret:    password,
		S3SessionToken:  d.Get("minio_session_token").(string),
		S3APISignature:  d.Get("minio_api_version").(string),
		S3SSL:           d.Get("minio_ssl").(bool),
		S3SSLCACertFile: d.Get("minio_cacert_file").(string),
		S3SSLCertFile:   d.Get("minio_cert_file").(string),
		S3SSLKeyFile:    d.Get("minio_key_file").(string),
		S3SSLSkipVerify: d.Get("minio_insecure").(bool),
	}
}

// ServiceAccountConfig creates new service account config
func ServiceAccountConfig(d *schema.ResourceData, meta interface{}) *S3MinioServiceAccountConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioServiceAccountConfig{
		MinioAdmin:       m.S3Admin,
		MinioAccessKey:   d.Get("access_key").(string),
		MinioTargetUser:  d.Get("target_user").(string),
		MinioDisableUser: d.Get("disable_user").(bool),
		MinioUpdateKey:   d.Get("update_secret").(bool),
		MinioSAPolicy:    d.Get("policy").(string),
	}
}

// IAMUserConfig creates new user config
func IAMUserConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMUserConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMUserConfig{
		MinioAdmin:        m.S3Admin,
		MinioIAMName:      d.Get("name").(string),
		MinioSecret:       d.Get("secret").(string),
		MinioDisableUser:  d.Get("disable_user").(bool),
		MinioUpdateKey:    d.Get("update_secret").(bool),
		MinioForceDestroy: d.Get("force_destroy").(bool),
	}
}

// IAMGroupConfig creates new group config
func IAMGroupConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupConfig{
		MinioAdmin:        m.S3Admin,
		MinioIAMName:      d.Get("name").(string),
		MinioDisableGroup: d.Get("disable_group").(bool),
		MinioForceDestroy: d.Get("force_destroy").(bool),
	}
}

// IAMGroupAttachmentConfig creates new membership config for a single user
func IAMGroupAttachmentConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupAttachmentConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupAttachmentConfig{
		MinioAdmin:    m.S3Admin,
		MinioIAMUser:  d.Get("user_name").(string),
		MinioIAMGroup: d.Get("group_name").(string),
	}
}

// IAMGroupMembersipConfig creates new membership config
func IAMGroupMembersipConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupMembershipConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupMembershipConfig{
		MinioAdmin:    m.S3Admin,
		MinioIAMName:  d.Get("name").(string),
		MinioIAMUsers: getStringList(d.Get("users").(*schema.Set).List()),
		MinioIAMGroup: d.Get("group").(string),
	}
}

// IAMPolicyConfig creates new policy config
func IAMPolicyConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMPolicyConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMPolicyConfig{
		MinioAdmin:         m.S3Admin,
		MinioIAMName:       d.Get("name").(string),
		MinioIAMNamePrefix: d.Get("name_prefix").(string),
		MinioIAMPolicy:     d.Get("policy").(string),
	}
}

// IAMGroupPolicyConfig creates new group policy config
func IAMGroupPolicyConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupPolicyConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupPolicyConfig{
		MinioAdmin:         m.S3Admin,
		MinioIAMName:       d.Get("name").(string),
		MinioIAMNamePrefix: d.Get("name_prefix").(string),
		MinioIAMPolicy:     d.Get("policy").(string),
		MinioIAMGroup:      d.Get("group").(string),
	}
}

// KMSKeyConfig creates new service account config
func KMSKeyConfig(d *schema.ResourceData, meta interface{}) *S3MinioKMSKeyConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioKMSKeyConfig{
		MinioAdmin:    m.S3Admin,
		MinioKMSKeyID: d.Get("key_id").(string),
	}
}
