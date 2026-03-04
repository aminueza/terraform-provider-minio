package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioNotifyWebhook() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_webhook",
		buildCfg:   buildNotifyWebhookCfg,
		readFields: readNotifyWebhookFields,
	}

	return &schema.Resource{
		Description:   "Manages a webhook notification target for MinIO bucket event notifications. Webhook targets receive bucket events (object created, deleted, etc.) via HTTP POST requests.",
		CreateContext: notifyCreate(nrc),
		ReadContext:   notifyRead(nrc),
		UpdateContext: notifyUpdate(nrc),
		DeleteContext: notifyDelete(nrc),
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: mergeSchemas(notifyCommonSchema(), map[string]*schema.Schema{
			"endpoint": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "HTTP(S) endpoint URL to send bucket event notifications to.",
			},
			"auth_token": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Authentication token for the webhook endpoint. MinIO does not return this value on read.",
			},
			"client_cert": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to the X.509 client certificate for mTLS authentication.",
			},
			"client_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to the X.509 private key for mTLS authentication. MinIO does not return this value on read.",
			},
		}),
	}
}

func buildNotifyWebhookCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string
	notifyBuildCfgAddParam(&parts, "endpoint", d.Get("endpoint").(string))
	notifyBuildCfgAddParam(&parts, "auth_token", getOptionalField(d, "auth_token", "").(string))
	notifyBuildCfgAddParam(&parts, "client_cert", getOptionalField(d, "client_cert", "").(string))
	notifyBuildCfgAddParam(&parts, "client_key", getOptionalField(d, "client_key", "").(string))
	notifyBuildCommonCfg(&parts, d, meta)
	return strings.Join(parts, " ")
}

func readNotifyWebhookFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if v, ok := cfgMap["endpoint"]; ok {
		_ = d.Set("endpoint", v)
	}
	if v, ok := cfgMap["client_cert"]; ok && v != "" {
		_ = d.Set("client_cert", v)
	}
	notifyReadCommonFields(cfgMap, d)
	return nil
}
