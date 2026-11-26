package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioLoggerWebhook() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:             "logger_webhook",
		buildCfg:              buildLoggerWebhookCfg,
		readFields:            readLoggerWebhookFields,
		appliesWithoutRestart: true,
	}

	return &schema.Resource{
		Description:   "Manages a logger webhook target for MinIO system log forwarding. Logger webhooks send server log events to HTTP endpoints for centralized logging.",
		CreateContext: notifyCreate(nrc),
		ReadContext:   notifyRead(nrc),
		UpdateContext: notifyUpdate(nrc),
		DeleteContext: notifyDelete(nrc),
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		CustomizeDiff: customizeDiffWebhookRetry,
		Schema: mergeSchemas(notifyCommonSchema(), notifyWithQueueSize(addWebhookRetryFields(map[string]*schema.Schema{
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
			// RequiredWith keeps a cert-only configuration out of the plan: the
			// readback would put the server's key path into state and the empty
			// config value would then clear it, which MinIO rejects as a broken
			// pair. The key alone stays allowed: removing the certificate while
			// the key remains is how the pair gets cleared.
			"client_cert": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"client_key"},
				Description:  "Path to X.509 client certificate for mTLS.",
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
		}))),
	}
}

func buildLoggerWebhookCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string
	notifyBuildCfgAddParam(&parts, "endpoint", d.Get("endpoint").(string))
	notifyBuildCfgClearableParam(&parts, d, "auth_token")
	notifyBuildCfgClearablePair(&parts, d, "client_cert", "client_key")
	notifyBuildCfgClearableParam(&parts, d, "proxy")
	notifyBuildCfgAddInt(&parts, "batch_size", getOptionalField(d, "batch_size", 0).(int))
	buildWebhookRetryCfg(&parts, d)
	notifyBuildQueueSizeCommonCfg(&parts, d)
	return strings.Join(parts, " ")
}

func readLoggerWebhookFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if diags := notifySetPresentStrings(d, cfgMap, "endpoint", "client_cert", "client_key", "proxy"); diags != nil {
		return diags
	}
	if diags := notifySetInt(d, cfgMap, "batch_size", "batch_size"); diags != nil {
		return diags
	}
	if diags := readWebhookRetryFields(cfgMap, d); diags != nil {
		return diags
	}
	if diags := notifyReadQueueSize(cfgMap, d); diags != nil {
		return diags
	}
	return notifyReadCommonFields(cfgMap, d)
}
