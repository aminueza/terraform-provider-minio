package minio

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

//BucketConfig creates a new config for minio buckets
func BucketConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucket {
	m := meta.(*S3MinioClient)

	return &S3MinioBucket{
		MinioClient:       m.S3Client,
		MinioAdmin:        m.S3Admin,
		MinioRegion:       m.S3Region,
		MinioAccess:       m.S3UserAccess,
		MinioBucket:       d.Get("bucket").(string),
		MinioBucketPrefix: d.Get("bucket_prefix").(string),
		MinioACL:          d.Get("acl").(string),
		MinioForceDestroy: d.Get("force_destroy").(bool),
	}
}

//NewConfig creates a new config for minio
func NewConfig(d *schema.ResourceData) *S3MinioConfig {
	return &S3MinioConfig{
		S3HostPort:     d.Get("minio_server").(string),
		S3Region:       d.Get("minio_region").(string),
		S3UserAccess:   d.Get("minio_access_key").(string),
		S3UserSecret:   d.Get("minio_secret_key").(string),
		S3APISignature: d.Get("minio_api_version").(string),
		S3SSL:          d.Get("minio_ssl").(bool),
	}
}

//IAMUserConfig creates new user config
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

//IAMGroupConfig creates new group config
func IAMGroupConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupConfig{
		MinioAdmin:        m.S3Admin,
		MinioIAMName:      d.Get("name").(string),
		MinioDisableGroup: d.Get("disable_group").(bool),
		MinioForceDestroy: d.Get("force_destroy").(bool),
	}
}

//IAMGroupAttachmentConfig creates new membership config for a single user
func IAMGroupAttachmentConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupAttachmentConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupAttachmentConfig{
		MinioAdmin:    m.S3Admin,
		MinioIAMUser:  d.Get("user_name").(string),
		MinioIAMGroup: d.Get("group_name").(string),
	}
}

//IAMGroupMembersipConfig creates new membership config
func IAMGroupMembersipConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMGroupMembershipConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMGroupMembershipConfig{
		MinioAdmin:    m.S3Admin,
		MinioIAMName:  d.Get("name").(string),
		MinioIAMUsers: getStringList(d.Get("users").(*schema.Set).List()),
		MinioIAMGroup: d.Get("group").(string),
	}
}

//IAMPolicyConfig creates new policy config
func IAMPolicyConfig(d *schema.ResourceData, meta interface{}) *S3MinioIAMPolicyConfig {
	m := meta.(*S3MinioClient)

	return &S3MinioIAMPolicyConfig{
		MinioAdmin:         m.S3Admin,
		MinioIAMName:       d.Get("name").(string),
		MinioIAMNamePrefix: d.Get("name_prefix").(string),
		MinioIAMPolicy:     d.Get("policy").(string),
	}
}

//IAMGroupPolicyConfig creates new group policy config
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
