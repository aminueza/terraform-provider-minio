package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioNotifyNats() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_nats",
		buildCfg:   buildNotifyNatsCfg,
		readFields: readNotifyNatsFields,
	}
	return &schema.Resource{
		Description:   "Manages a NATS notification target for MinIO bucket event notifications.",
		CreateContext: notifyCreate(nrc),
		ReadContext:   notifyRead(nrc),
		UpdateContext: notifyUpdate(nrc),
		DeleteContext: notifyDelete(nrc),
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: mergeSchemas(notifyCommonSchema(), map[string]*schema.Schema{
			"address": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "NATS server address (e.g., 'nats://localhost:4222').",
			},
			"subject": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "NATS subject to publish notifications to.",
			},
			"username": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Username for NATS authentication.",
			},
			"password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Password for NATS authentication. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"token": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Token for NATS authentication. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"user_credentials": {
				Type:        schema.TypeString,
				Optional:    true,
				Deprecated:  "MinIO's notify_nats has no user_credentials setting, so this value is not sent to the server. Use username and password, or token, instead. This attribute will be removed in a future major version.",
				Description: "Deprecated: MinIO has no NATS user credentials setting, so this value is ignored.",
			},
			"tls": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to enable TLS for the NATS connection.",
			},
			"tls_skip_verify": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to skip TLS certificate verification.",
			},
			"ping_interval": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Duration interval between NATS ping requests (e.g., '0s').",
			},
			"jetstream": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to enable JetStream support for NATS.",
			},
			"streaming": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to enable NATS Streaming (STAN) mode.",
			},
			"streaming_async": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to enable asynchronous publishing for NATS Streaming.",
			},
			"streaming_max_pub_acks_in_flight": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Maximum number of unacknowledged messages in flight for NATS Streaming.",
			},
			"streaming_cluster_id": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Cluster ID for NATS Streaming.",
			},
			"cert_authority": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to the certificate authority file for TLS verification.",
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
				Description:  "Path to the client certificate for mTLS authentication.",
			},
			"client_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to the client private key for mTLS authentication.",
			},
		}),
	}
}

func buildNotifyNatsCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	notifyBuildCfgAddParam(&parts, "address", d.Get("address").(string))
	notifyBuildCfgAddParam(&parts, "subject", d.Get("subject").(string))
	notifyBuildCfgClearablePair(&parts, d, "username", "password")
	notifyBuildCfgClearableParam(&parts, d, "token")
	// user_credentials is not sent: notify_nats has no such key, and MinIO
	// glues an unknown key onto the value written before it, which corrupted
	// the subject, the password or the token.
	notifyBuildCfgAddBool(&parts, "tls", getOptionalField(d, "tls", false).(bool))
	notifyBuildCfgAddBool(&parts, "tls_skip_verify", getOptionalField(d, "tls_skip_verify", false).(bool))
	notifyBuildCfgAddParam(&parts, "ping_interval", getOptionalField(d, "ping_interval", "").(string))
	notifyBuildCfgAddBool(&parts, "jetstream", getOptionalField(d, "jetstream", false).(bool))
	notifyBuildCfgAddBool(&parts, "streaming", getOptionalField(d, "streaming", false).(bool))
	notifyBuildCfgAddBool(&parts, "streaming_async", getOptionalField(d, "streaming_async", false).(bool))
	notifyBuildCfgAddInt(&parts, "streaming_max_pub_acks_in_flight", getOptionalField(d, "streaming_max_pub_acks_in_flight", 0).(int))
	notifyBuildCfgClearableParam(&parts, d, "streaming_cluster_id")
	notifyBuildCfgClearableParam(&parts, d, "cert_authority")
	notifyBuildCfgClearablePair(&parts, d, "client_cert", "client_key")

	notifyBuildCommonCfg(&parts, d, meta)

	return strings.Join(parts, " ")
}

func readNotifyNatsFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if diags := notifySetPresentStrings(d, cfgMap, "address", "subject", "username"); diags != nil {
		return diags
	}
	if diags := notifySetOnBools(d, cfgMap, "tls", "tls_skip_verify"); diags != nil {
		return diags
	}
	if diags := notifySetNonEmptyStrings(d, cfgMap, "ping_interval"); diags != nil {
		return diags
	}
	if diags := notifySetOnBools(d, cfgMap, "jetstream", "streaming", "streaming_async"); diags != nil {
		return diags
	}
	if diags := notifySetInt(d, cfgMap, "streaming_max_pub_acks_in_flight", "streaming_max_pub_acks_in_flight"); diags != nil {
		return diags
	}
	if diags := notifySetPresentStrings(d, cfgMap, "streaming_cluster_id", "cert_authority", "client_cert", "client_key"); diags != nil {
		return diags
	}
	return notifyReadCommonFields(cfgMap, d)
}
