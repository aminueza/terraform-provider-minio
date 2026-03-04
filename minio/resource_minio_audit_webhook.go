package minio

import (
	"context"
	"fmt"
	"log"
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
		Schema: map[string]*schema.Schema{
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
			"client_cert": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to the X.509 client certificate for mTLS authentication with the webhook endpoint.",
			},
			"client_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to the X.509 private key for mTLS authentication. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Indicates whether a MinIO server restart is required for the configuration to take effect.",
			},
		},
	}
}

func auditWebhookConfigKey(name string) string {
	return fmt.Sprintf("audit_webhook:%s", name)
}

func minioCreateAuditWebhook(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := AuditWebhookConfig(d, meta)

	log.Printf("[DEBUG] Creating audit webhook: %s", config.Name)

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

	log.Printf("[DEBUG] Created audit webhook: %s (restart_required=%v)", config.Name, restart)

	return minioReadAuditWebhook(ctx, d, meta)
}

func minioReadAuditWebhook(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin
	name := d.Id()

	log.Printf("[DEBUG] Reading audit webhook: %s", name)

	configKey := auditWebhookConfigKey(name)
	configData, err := minioAdmin.GetConfigKV(ctx, configKey)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") {
			log.Printf("[WARN] Audit webhook %s no longer exists, removing from state", name)
			d.SetId("")
			return nil
		}
		return NewResourceError("reading audit webhook configuration", name, err)
	}

	configStr := strings.TrimSpace(string(configData))
	log.Printf("[DEBUG] Raw config data for audit_webhook %s: %s", name, configStr)

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

	// Skip auth_token — MinIO does not return this value
	// Skip enable — MinIO does not return this in GetConfigKV response;
	// Terraform retains the user's value from configuration
	// Skip client_key — MinIO does not return this value

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

	if v, ok := cfgMap["client_cert"]; ok && v != "" {
		if setErr := d.Set("client_cert", v); setErr != nil {
			return NewResourceError("setting client_cert", name, setErr)
		}
	}

	return nil
}

func minioUpdateAuditWebhook(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := AuditWebhookConfig(d, meta)

	log.Printf("[DEBUG] Updating audit webhook: %s", config.Name)

	cfgData := buildAuditWebhookCfgData(config)
	configString := fmt.Sprintf("%s %s", auditWebhookConfigKey(config.Name), cfgData)
	restart, err := config.MinioAdmin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("updating audit webhook configuration", config.Name, err)
	}

	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", config.Name, setErr)
	}

	log.Printf("[DEBUG] Updated audit webhook: %s (restart_required=%v)", config.Name, restart)

	return minioReadAuditWebhook(ctx, d, meta)
}

func minioDeleteAuditWebhook(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin
	name := d.Id()

	log.Printf("[DEBUG] Deleting audit webhook: %s", name)

	configKey := auditWebhookConfigKey(name)
	_, err := minioAdmin.DelConfigKV(ctx, configKey)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") {
			log.Printf("[WARN] Audit webhook %s already removed", name)
			d.SetId("")
			return nil
		}
		return NewResourceError("deleting audit webhook configuration", name, err)
	}

	d.SetId("")
	log.Printf("[DEBUG] Deleted audit webhook: %s", name)

	return nil
}

func buildAuditWebhookCfgData(config *S3MinioAuditWebhook) string {
	var parts []string

	addParam := func(key, val string) {
		if val != "" {
			if strings.ContainsAny(val, " \t") {
				parts = append(parts, fmt.Sprintf("%s=%q", key, val))
			} else {
				parts = append(parts, fmt.Sprintf("%s=%s", key, val))
			}
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

	if config.Enable {
		parts = append(parts, "enable=on")
	} else {
		parts = append(parts, "enable=off")
	}

	return strings.Join(parts, " ")
}
