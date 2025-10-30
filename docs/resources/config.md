# minio_config Resource

Manages MinIO server configuration settings using the MinIO Admin Go SDK. This resource allows you to configure various MinIO server settings such as API limits, notification endpoints, region settings, and more.

**Important Notes:**
- Some configuration changes may require a MinIO server restart to take effect. The `restart_required` attribute indicates when this is necessary.
- Deleting a configuration resource resets it to the default value, not removing the configuration key entirely.
- Server restarts are out of Terraform's scope and must be performed manually using `mc admin service restart` or similar commands.

## Example Usage

### Basic Region Configuration

```hcl
resource "minio_config" "region" {
  key   = "region"
  value = "name=us-west-1"
}
```

### API Request Throttling

```hcl
resource "minio_config" "api_throttle" {
  key   = "api"
  value = "requests_max=1000 requests_deadline=10s"
}
```

### Webhook Notification Endpoint

```hcl
resource "minio_config" "webhook_notify" {
  key   = "notify_webhook:production"
  value = "endpoint=http://webhook.example.com/events queue_limit=1000"
}
```

### Multiple Configuration Settings

```hcl
# Configure API settings
resource "minio_config" "api" {
  key   = "api"
  value = "requests_max=2000 requests_deadline=15s root_access=off"
}

# Configure region
resource "minio_config" "region" {
  key   = "region"
  value = "name=us-east-1"
}

# Configure Redis notification
resource "minio_config" "redis_notify" {
  key   = "notify_redis:cache"
  value = "address=127.0.0.1:6379 format=namespace key=bucketevents password=secret"
}
```

### Kafka Notification with TLS

```hcl
resource "minio_config" "kafka_notify" {
  key   = "notify_kafka:events"
  value = "brokers=kafka1:9092,kafka2:9092 topic=minio-events tls=on tls_skip_verify=off"
}
```

## Argument Reference

- `key` (Required) - The configuration key. This typically includes the subsystem name (e.g., `api`, `region`, `notify_webhook:name`). Use format `subsystem` or `subsystem:target` for targeted configurations.
- `value` (Required) - The configuration value in `key=value` format. Multiple settings can be specified separated by spaces (e.g., `requests_max=1000 requests_deadline=10s`).

## Timeouts

`minio_config` provides the following configuration options for timeouts:

- `create` - (Default 5 minutes) How long to wait for configuration to be set.
- `read` - (Default 2 minutes) How long to wait for configuration to be read.
- `update` - (Default 5 minutes) How long to wait for configuration to be updated.
- `delete` - (Default 5 minutes) How long to wait for configuration to be deleted.

## Attributes Reference

- `id` - The configuration key.
- `restart_required` - Boolean indicating whether a server restart is required for the configuration change to take effect.

## Common Configuration Keys

Here are some commonly used configuration keys:

### API Settings
- `api` - API request throttling and root access control
  - Example: `requests_max=1000 requests_deadline=10s root_access=off`

### Region
- `region` - Server region name
  - Example: `name=us-west-1`

### Notification Targets
- `notify_webhook:target_name` - Webhook notification endpoint
- `notify_amqp:target_name` - AMQP notification endpoint
- `notify_kafka:target_name` - Kafka notification endpoint
- `notify_mqtt:target_name` - MQTT notification endpoint
- `notify_nats:target_name` - NATS notification endpoint
- `notify_nsq:target_name` - NSQ notification endpoint
- `notify_redis:target_name` - Redis notification endpoint
- `notify_mysql:target_name` - MySQL notification endpoint
- `notify_postgres:target_name` - PostgreSQL notification endpoint
- `notify_elasticsearch:target_name` - Elasticsearch notification endpoint

### Identity and Access
- `identity_openid` - OpenID Connect configuration
- `identity_ldap` - LDAP configuration
- `identity_tls` - TLS certificate authentication

### Storage
- `cache` - Disk caching configuration
- `compression` - Compression settings

## Getting Available Configuration Keys

To see all available configuration keys and their options, use the MinIO client:

```bash
# List all configuration keys
mc admin config set myminio/

# Get help for a specific key
mc admin config set myminio/ api
```

## Import

Configuration settings can be imported using the configuration key:

```sh
terraform import minio_config.example api
terraform import minio_config.webhook notify_webhook:production
```

## Notes

1. **Restart Requirements**: When `restart_required` is `true`, you must restart the MinIO server for changes to take effect:
   ```bash
   mc admin service restart myminio
   ```

2. **Configuration Format**: The `value` attribute should contain space-separated `key=value` pairs. The exact format depends on the configuration subsystem.

3. **Target Names**: For notification endpoints, use the format `notify_<type>:<target_name>` where `<target_name>` is a unique identifier for that endpoint.

4. **Verification**: After applying configuration changes, verify them using:
   ```bash
   mc admin config get myminio/ <key>
   ```

5. **Defaults**: Deleting a config resource with Terraform resets it to MinIO's default value, not removing it entirely from the server configuration.
