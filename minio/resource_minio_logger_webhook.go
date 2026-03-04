package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioLoggerWebhook() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "logger_webhook",
		buildCfg:   buildLoggerWebhookCfg,
		readFields: readLoggerWebhookFields,
	}

	return &schema.Resource{
		Description:   "Manages a logger webhook target for MinIO system log forwarding. Logger webhooks send server log events to HTTP endpoints for centralized logging.",
		CreateContext: notifyCreate(nrc),
		ReadContext:   notifyRead(nrc),
		UpdateContext: notifyUpdate(nrc),
		DeleteContext: notifyDelete(nrc),
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: mergeSchemas(notifyCommonSchema(), map[string]*schema.Schema{
			"endpoint": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "HTTP(S) endpoint URL to send log events to.",
			},
			"auth_token": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Authentication token for the endpoint.",
			},
			"batch_size": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "Number of log events per batch.",
			},
			"client_cert": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to X.509 client certificate for mTLS.",
			},
			"client_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to X.509 private key for mTLS.",
			},
			"proxy": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Proxy URL for the webhook endpoint.",
			},
		}),
	}
}

func buildLoggerWebhookCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string
	notifyBuildCfgAddParam(&parts, "endpoint", d.Get("endpoint").(string))
	notifyBuildCfgAddParam(&parts, "auth_token", getOptionalField(d, "auth_token", "").(string))
	notifyBuildCfgAddParam(&parts, "client_cert", getOptionalField(d, "client_cert", "").(string))
	notifyBuildCfgAddParam(&parts, "client_key", getOptionalField(d, "client_key", "").(string))
	notifyBuildCfgAddParam(&parts, "proxy", getOptionalField(d, "proxy", "").(string))
	notifyBuildCfgAddInt(&parts, "batch_size", getOptionalField(d, "batch_size", 0).(int))
	notifyBuildCommonCfg(&parts, d, meta)
	return strings.Join(parts, " ")
}

func readLoggerWebhookFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if v, ok := cfgMap["endpoint"]; ok {
		_ = d.Set("endpoint", v)
	}
	if v, ok := cfgMap["client_cert"]; ok && v != "" {
		_ = d.Set("client_cert", v)
	}
	if v, ok := cfgMap["proxy"]; ok && v != "" {
		_ = d.Set("proxy", v)
	}
	if v, ok := cfgMap["batch_size"]; ok {
		if n, err := parseInt(v); err == nil {
			_ = d.Set("batch_size", n)
		}
	}
	notifyReadCommonFields(cfgMap, d)
	return nil
}
