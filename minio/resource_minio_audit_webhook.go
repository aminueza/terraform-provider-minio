package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioAuditWebhook() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages an audit webhook target for MinIO audit logging. Audit webhooks send detailed API audit events to HTTP endpoints for compliance, SIEM integration, and security monitoring.",
		CreateContext: minioCreateAuditWebhook,
		ReadContext:   minioReadAuditWebhook,
		UpdateContext: minioUpdateAuditWebhook,
		DeleteContext: minioDeleteAuditWebhook,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		CustomizeDiff: customizeDiffWebhookRetry,
		// mergeSchemas adds validateConfigValue to every string field, so the
		// fields below must not carry it themselves or a bad value would
		// report the same error twice.
		Schema: mergeSchemas(webhookRetryFields(), map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Target name for the audit webhook (e.g., 'splunk', 'elk'). Used as the identifier in the configuration key 'audit_webhook:<name>'.",
			},
			"endpoint": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "HTTP(S) endpoint URL to send audit events to.",
			},
			"auth_token": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Authentication token for the webhook endpoint (e.g., Bearer token). MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"enable": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Whether this audit webhook target is enabled.",
			},
			"queue_size": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "Maximum number of audit events to queue before dropping. Defaults to MinIO server default if not set.",
			},
			"batch_size": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "Number of audit events to send in a single batch to the endpoint.",
			},
			"queue_dir": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Directory path for persistent event store when the target is offline. MinIO creates it when the target is applied, so it must be creatable and writable by the MinIO process on every node; an unwritable path crashes the server.",
			},
			// RequiredWith keeps a cert-only configuration out of the plan: the
			// readback would put the server's key path into state and the empty
			// config value would then clear it, which MinIO rejects as a broken
			// pair. The key alone stays allowed: removing the certificate while
			// the key remains is how the pair gets cleared.
			"client_cert": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"client_key"},
				Description:  "Path to the X.509 client certificate for mTLS authentication with the webhook endpoint.",
			},
			"client_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to the X.509 private key for mTLS authentication with the webhook endpoint.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Indicates whether a MinIO server restart is required for the configuration to take effect.",
			},
		}),
	}
}

func auditWebhookConfigKey(name string) string {
	return fmt.Sprintf("audit_webhook:%s", name)
}

func minioCreateAuditWebhook(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := AuditWebhookConfig(d, meta)

	tflog.Debug(ctx, fmt.Sprintf("Creating audit webhook: %s", config.Name))

	lockKey := auditWebhookConfigKey(config.Name)
	dynamicTargetCreateLock.Lock(lockKey)
	defer dynamicTargetCreateLock.Unlock(lockKey)
	if err := rejectExistingDynamicTarget(ctx, config.MinioAdmin, "audit_webhook", config.Name); err != nil {
		return NewResourceError("creating audit webhook configuration", config.Name, err)
	}

	cfgData := buildAuditWebhookCfgData(config)
	configString := fmt.Sprintf("%s %s", auditWebhookConfigKey(config.Name), cfgData)
	restart, err := config.MinioAdmin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("creating audit webhook configuration", config.Name, err)
	}

	d.SetId(config.Name)
	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", config.Name, setErr)
	}

	tflog.Debug(ctx, fmt.Sprintf("Created audit webhook: %s (restart_required=%v)", config.Name, restart))

	return minioReadAuditWebhook(ctx, d, meta)
}

func minioReadAuditWebhook(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin
	name := d.Id()

	tflog.Debug(ctx, fmt.Sprintf("Reading audit webhook: %s", name))

	configKey := auditWebhookConfigKey(name)
	configData, err := minioAdmin.GetConfigKV(ctx, configKey)
	if err != nil {
		if isNotifyTargetMissingError(err) {
			tflog.Warn(ctx, fmt.Sprintf("Audit webhook %s no longer exists, removing from state", name))
			d.SetId("")
			return nil
		}
		return NewResourceError("reading audit webhook configuration", name, err)
	}

	configStr := strings.TrimSpace(string(configData))
	tflog.Debug(ctx, fmt.Sprintf("Raw config data for audit_webhook %s: %s", name, configStr))

	// Parse the config string: "audit_webhook:NAME key1=value1 key2=value2"
	var valueStr string
	if strings.HasPrefix(configStr, configKey+" ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	} else {
		valueStr = configStr
	}

	cfgMap := parseConfigParams(valueStr)

	if setErr := d.Set("name", name); setErr != nil {
		return NewResourceError("setting name", name, setErr)
	}

	if v, ok := cfgMap["endpoint"]; ok {
		if setErr := d.Set("endpoint", v); setErr != nil {
			return NewResourceError("setting endpoint", name, setErr)
		}
	}

	// The server prints enable only when it is off (SubsysInfo.WriteTo), so a
	// response that lists the target without it means the webhook is on. See
	// issue #1317.
	if len(cfgMap) > 0 {
		enabled := true
		if v, ok := cfgMap["enable"]; ok {
			enabled = v != "off"
		}
		if setErr := d.Set("enable", enabled); setErr != nil {
			return NewResourceError("setting enable", name, setErr)
		}
	}

	if v, ok := cfgMap["queue_size"]; ok {
		if n, parseErr := strconv.Atoi(v); parseErr == nil {
			if setErr := d.Set("queue_size", n); setErr != nil {
				return NewResourceError("setting queue_size", name, setErr)
			}
		}
	}

	if v, ok := cfgMap["batch_size"]; ok {
		if n, parseErr := strconv.Atoi(v); parseErr == nil {
			if setErr := d.Set("batch_size", n); setErr != nil {
				return NewResourceError("setting batch_size", name, setErr)
			}
		}
	}

	if v, ok := cfgMap["queue_dir"]; ok {
		if setErr := d.Set("queue_dir", v); setErr != nil {
			return NewResourceError("setting queue_dir", name, setErr)
		}
	}

	if diags := readWebhookRetryFields(cfgMap, d); diags != nil {
		return diags
	}

	if v, ok := cfgMap["client_cert"]; ok {
		if setErr := d.Set("client_cert", v); setErr != nil {
			return NewResourceError("setting client_cert", name, setErr)
		}
	}

	if v, ok := cfgMap["client_key"]; ok {
		if setErr := d.Set("client_key", v); setErr != nil {
			return NewResourceError("setting client_key", name, setErr)
		}
	}

	return nil
}

func minioUpdateAuditWebhook(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := AuditWebhookConfig(d, meta)

	tflog.Debug(ctx, fmt.Sprintf("Updating audit webhook: %s", config.Name))

	cfgData := buildAuditWebhookCfgData(config)
	for _, key := range auditWebhookClearedKeys(d) {
		cfgData += " " + key + "="
	}
	// max_retry=0 means "retry forever" and is what MinIO seeds into a new
	// target, so a removal must write it back explicitly: the server merges
	// omitted keys and would otherwise keep the old retry count.
	if d.HasChange("max_retry") && d.Get("max_retry").(int) == 0 {
		cfgData += " max_retry=0"
	}
	configString := fmt.Sprintf("%s %s", auditWebhookConfigKey(config.Name), cfgData)
	restart, err := config.MinioAdmin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("updating audit webhook configuration", config.Name, err)
	}

	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", config.Name, setErr)
	}

	tflog.Debug(ctx, fmt.Sprintf("Updated audit webhook: %s (restart_required=%v)", config.Name, restart))

	return minioReadAuditWebhook(ctx, d, meta)
}

func minioDeleteAuditWebhook(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin
	name := d.Id()

	tflog.Debug(ctx, fmt.Sprintf("Deleting audit webhook: %s", name))

	configKey := auditWebhookConfigKey(name)
	_, err := minioAdmin.DelConfigKV(ctx, configKey)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") {
			tflog.Warn(ctx, fmt.Sprintf("Audit webhook %s already removed", name))
			d.SetId("")
			return nil
		}
		return NewResourceError("deleting audit webhook configuration", name, err)
	}

	d.SetId("")
	tflog.Debug(ctx, fmt.Sprintf("Deleted audit webhook: %s", name))

	return nil
}

// auditWebhookClearedKeys lists the optional strings that were removed from
// the configuration. MinIO merges set-config-kv into the target's current
// settings, so a key that is left out keeps its old value; these are sent as
// an empty key= instead.
func auditWebhookClearedKeys(d *schema.ResourceData) []string {
	var keys []string
	for _, key := range []string{"auth_token", "client_cert", "client_key", "queue_dir"} {
		if d.HasChange(key) && d.Get(key).(string) == "" {
			keys = append(keys, key)
		}
	}
	// MinIO only accepts client_cert and client_key as a pair; older servers may
	// omit client_key on read so it stays empty in imported state, in which case
	// removing the certificate clears the key with it.
	if Contains(keys, "client_cert") && !Contains(keys, "client_key") && d.Get("client_key").(string) == "" {
		keys = append(keys, "client_key")
	}
	return keys
}

func buildAuditWebhookCfgData(config *S3MinioAuditWebhook) string {
	var parts []string

	addParam := func(key, val string) {
		if val != "" {
			parts = append(parts, buildConfigParam(key, val))
		}
	}

	addParam("endpoint", config.Endpoint)
	addParam("auth_token", config.AuthToken)
	addParam("client_cert", config.ClientCert)
	addParam("client_key", config.ClientKey)

	if config.QueueSize > 0 {
		parts = append(parts, fmt.Sprintf("queue_size=%d", config.QueueSize))
	}

	if config.BatchSize > 0 {
		parts = append(parts, fmt.Sprintf("batch_size=%d", config.BatchSize))
	}

	addParam("queue_dir", config.QueueDir)

	// max_retry=0 is written by minioUpdateAuditWebhook on removal, not here:
	// the builder has no diff to tell a removal from a target that never set it.
	if config.MaxRetry > 0 {
		parts = append(parts, fmt.Sprintf("max_retry=%d", config.MaxRetry))
	}
	addParam("retry_interval", config.RetryInterval)
	addParam("http_timeout", config.HTTPTimeout)

	if config.Enable {
		parts = append(parts, "enable=on")
	} else {
		parts = append(parts, "enable=off")
	}

	return strings.Join(parts, " ")
}
