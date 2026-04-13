# Terraform Plugin Framework Migration Task List

## Completed ✅
- [x] minio_s3_bucket (resource)
- [x] minio_s3_bucket (data source)
- [x] minio_iam_user
- [x] minio_iam_policy
- [x] minio_service_account
- [x] minio_s3_bucket_policy

## High Priority - Core S3 Resources (10)
- [ ] minio_s3_bucket_versioning
- [ ] minio_s3_bucket_server_side_encryption_configuration
- [ ] minio_s3_bucket_object_lock_configuration
- [ ] minio_s3_bucket_replication
- [ ] minio_s3_bucket_notification
- [ ] minio_s3_bucket_quota
- [ ] minio_s3_bucket_tags
- [ ] minio_s3_bucket_cors
- [ ] minio_s3_bucket_anonymous_access
- [ ] minio_s3_bucket_retention
- [ ] minio_s3_object
- [ ] minio_s3_object_tags
- [ ] minio_s3_object_legal_hold
- [ ] minio_s3_object_retention

## High Priority - IAM Resources (10)
- [ ] minio_iam_group
- [ ] minio_iam_group_membership
- [ ] minio_iam_group_policy
- [ ] minio_iam_group_policy_attachment
- [ ] minio_iam_user_group_membership
- [ ] minio_iam_user_policy_attachment
- [ ] minio_iam_group_user_attachment
- [ ] minio_iam_ldap_user_policy_attachment
- [ ] minio_iam_ldap_group_policy_attachment
- [ ] minio_accesskey (service account alternative)

## Medium Priority - ILM Resources (3)
- [ ] minio_ilm_policy
- [ ] minio_ilm_tier
- [ ] minio_kms_key

## Medium Priority - Configuration Resources (8)
- [ ] minio_config_kvs
- [ ] minio_server_config_api
- [ ] minio_server_config_etcd
- [ ] minio_server_config_heal
- [ ] minio_server_config_region
- [ ] minio_server_config_scanner
- [ ] minio_server_config_storage_class
- [ ] minio_site_replication

## Lower Priority - Notification Targets (13)
- [ ] minio_notify_amqp
- [ ] minio_notify_elasticsearch
- [ ] minio_notify_kafka
- [ ] minio_notify_mqtt
- [ ] minio_notify_mysql
- [ ] minio_notify_nats
- [ ] minio_notify_nsq
- [ ] minio_notify_postgres
- [ ] minio_notify_redis
- [ ] minio_notify_webhook
- [ ] minio_audit_webhook
- [ ] minio_audit_kafka
- [ ] minio_logger_webhook

## Lower Priority - Identity Providers (2)
- [ ] minio_iam_idp_openid
- [ ] minio_iam_idp_ldap

## Other (2)
- [ ] minio_prometheus_bearer_token
- [ ] minio_minio

## Data Sources (28)
- [ ] minio_account_info
- [ ] minio_data_usage
- [ ] minio_health_status
- [ ] minio_iam_group
- [ ] minio_iam_groups
- [ ] minio_iam_policy
- [ ] minio_iam_policy_document
- [ ] minio_iam_service_accounts
- [ ] minio_iam_user_policies
- [ ] minio_ilm_policy
- [ ] minio_ilm_tier_stats
- [ ] minio_ilm_tiers
- [ ] minio_license_info
- [ ] minio_prometheus_scrape_config
- [ ] minio_s3_bucket_cors_config
- [ ] minio_s3_bucket_encryption
- [ ] minio_s3_bucket_notification_config
- [ ] minio_s3_bucket_object_lock_configuration
- [ ] minio_s3_bucket_policy
- [ ] minio_s3_bucket_quota
- [ ] minio_s3_bucket_replication
- [ ] minio_s3_bucket_replication_status
- [ ] minio_s3_bucket_retention
- [ ] minio_s3_bucket_tags
- [ ] minio_s3_bucket_versioning
- [ ] minio_s3_buckets
- [ ] minio_s3_object
- [ ] minio_s3_objects
- [ ] minio_server_info
- [ ] minio_storage_info

## Notes
- Total remaining resources: ~52
- Total remaining data sources: ~28
- Estimated time: 2-4 weeks for full migration
- Each resource should have acceptance tests added after migration
- Update documentation templates after each resource migration
