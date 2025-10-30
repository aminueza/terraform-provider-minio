# MinIO Config Example

This example demonstrates how to manage MinIO server configuration using Terraform.

## Features Demonstrated

1. **Region Configuration**: Set the server region name
2. **API Throttling**: Configure API request limits and deadlines
3. **Notification Endpoints**: Set up webhook notifications for bucket events

## Prerequisites

- MinIO server running (default: `localhost:9000`)
- Admin credentials (default: `minioadmin/minioadmin`)
- Terraform installed

## Usage

1. Initialize Terraform:
   ```bash
   terraform init
   ```

2. Review the planned changes:
   ```bash
   terraform plan
   ```

3. Apply the configuration:
   ```bash
   terraform apply
   ```

4. If any config requires restart (check outputs), restart MinIO:
   ```bash
   mc admin service restart myminio
   ```

## Configuration Keys Reference

### Common Configuration Keys

- `region` - Server region name
- `api` - API throttling and access control
- `notify_webhook:name` - Webhook notification endpoint
- `notify_kafka:name` - Kafka notification endpoint
- `notify_redis:name` - Redis notification endpoint
- `notify_amqp:name` - AMQP notification endpoint

### Checking Current Configuration

To view the current configuration:

```bash
# View all configuration
mc admin config get myminio

# View specific configuration key
mc admin config get myminio api
mc admin config get myminio notify_webhook:production
```

### Getting Help for Configuration Keys

```bash
# Get help for API configuration
mc admin config set myminio api

# Get help for webhook notifications
mc admin config set myminio notify_webhook
```

## Important Notes

1. **Restart Requirements**: Some configuration changes require a server restart. Check the `restart_required` output to determine if a restart is needed.

2. **Configuration Verification**: After applying changes and restarting (if needed), verify the configuration:
   ```bash
   mc admin config get myminio <key>
   ```

3. **Removing Configuration**: Using `terraform destroy` will reset configurations to their defaults, not remove them entirely.

## Additional Examples

### Kafka Notification

```hcl
resource "minio_config" "kafka_notification" {
  key   = "notify_kafka:events"
  value = "brokers=kafka1:9092,kafka2:9092 topic=minio-events"
}
```

### Redis Notification with Authentication

```hcl
resource "minio_config" "redis_notification" {
  key   = "notify_redis:cache"
  value = "address=127.0.0.1:6379 key=bucketevents password=secret format=namespace"
}
```

### API Configuration with Root Access Control

```hcl
resource "minio_config" "api_secure" {
  key   = "api"
  value = "requests_max=2000 root_access=off"
}
```
