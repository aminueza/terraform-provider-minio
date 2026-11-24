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
			// RequiredWith keeps a cert-only configuration out of the plan: the
			// readback would put the server's key path into state and the empty
			// config value would then clear it, which MinIO rejects as a broken
			// pair on enabled targets and resolves to a cert without its key
			// on disabled ones. The key alone stays allowed: removing the
			// certificate while the key remains is how the pair gets cleared.
			"client_cert": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"client_key"},
				Description:  "Path to the X.509 client certificate for mTLS authentication.",
			},
			"client_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to the X.509 private key for mTLS authentication.",
			},
		}),
	}
}

func buildNotifyWebhookCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string
	notifyBuildCfgAddParam(&parts, "endpoint", d.Get("endpoint").(string))
	notifyBuildCfgClearableParam(&parts, d, "auth_token")
	notifyBuildCfgClearablePair(&parts, d, "client_cert", "client_key")
	notifyBuildCommonCfg(&parts, d, meta)
	return strings.Join(parts, " ")
}

func readNotifyWebhookFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if diags := notifySetPresentStrings(d, cfgMap, "endpoint", "client_cert", "client_key"); diags != nil {
		return diags
	}
	return notifyReadCommonFields(cfgMap, d)
}
