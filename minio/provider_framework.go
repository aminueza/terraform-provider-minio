package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// minioFrameworkProvider defines the provider implementation
type minioFrameworkProvider struct {
	version string
}

// NewFrameworkProvider creates a new framework provider instance
func NewFrameworkProvider(version string) func() provider.Provider {
	return func() provider.Provider {
		return &minioFrameworkProvider{version: version}
	}
}

// Metadata returns provider metadata
func (p *minioFrameworkProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "minio"
	resp.Version = p.version
}

// Schema returns an empty provider schema
func (p *minioFrameworkProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{}
}

// Configure sets up the provider
func (p *minioFrameworkProvider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
}

// Resources returns the list of resources
func (p *minioFrameworkProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newS3BucketResource,
		newIAMUserResource,
		newIAMPolicyResource,
		newServiceAccountResource,
		newIAMGroupResource,
		newIAMGroupMembershipResource,
		newS3ObjectResource,
		newIAMUserPolicyAttachmentResource,
		newIAMGroupPolicyAttachmentResource,
		newIAMLDAPUserPolicyAttachmentResource,
		newIAMLDAPGroupPolicyAttachmentResource,
		newIAMGroupUserAttachmentResource,
		newS3ObjectTagsResource,
		newS3ObjectLegalHoldResource,
		newS3ObjectRetentionResource,
		newBucketPolicyResource,
		newBucketVersioningResource,
		newBucketEncryptionResource,
		newBucketObjectLockConfigurationResource,
		newBucketQuotaResource,
		newBucketTagsResource,
		newBucketCorsResource,
		newBucketRetentionResource,
		newBucketNotificationResource,
		newILMPolicyResource,
		newILMTierResource,
		newIAMGroupPolicyResource,
		newIAMUserGroupMembershipResource,
		newKMSKeyResource,
		newConfigResource,
		newServerConfigRegionResource,
		newServerConfigHealResource,
		newServerConfigStorageClassResource,
		newServerConfigScannerResource,
		newServerConfigApiResource,
		newServerConfigEtcdResource,
		newAccessKeyResource,
		newPrometheusBearerTokenResource,
		newIAMIdpLdapResource,
		newS3BucketAnonymousAccessResource,
		newSiteReplicationResource,
		newBucketReplicationResource(),
		resourceMinioNotifyWebhookFramework,
		resourceMinioNotifyAmqpFramework,
		resourceMinioNotifyKafkaFramework,
		resourceMinioNotifyMqttFramework,
		resourceMinioNotifyNatsFramework,
		resourceMinioNotifyNsqFramework,
		resourceMinioNotifyMysqlFramework,
		resourceMinioNotifyPostgresFramework,
		resourceMinioNotifyElasticsearchFramework,
		resourceMinioNotifyRedisFramework,
		resourceMinioAuditWebhookFramework,
		resourceMinioAuditKafkaFramework,
		resourceMinioLoggerWebhookFramework,
	}
}

// DataSources returns the list of data sources
func (p *minioFrameworkProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newS3BucketDataSource,
	}
}
