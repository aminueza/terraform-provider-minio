package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioAuditKafka() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "audit_kafka",
		buildCfg:   buildAuditKafkaCfg,
		readFields: readAuditKafkaFields,
	}

	return &schema.Resource{
		Description:   "Manages a Kafka target for MinIO audit log forwarding. Audit events are published to the specified Kafka topic.",
		CreateContext: notifyCreate(nrc),
		ReadContext:   notifyRead(nrc),
		UpdateContext: notifyUpdate(nrc),
		DeleteContext: notifyDelete(nrc),
		CustomizeDiff: customizeDiffKafkaSasl,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: mergeSchemas(notifyCommonSchema(), notifyWithQueueSize(map[string]*schema.Schema{
			"brokers": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Comma-separated list of Kafka broker addresses.",
			},
			"topic": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Kafka topic for audit events.",
			},
			"sasl": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable SASL authentication for Kafka connections.",
			},
			"sasl_username": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "SASL username for Kafka authentication. Only used when `sasl` is `true`.",
			},
			"sasl_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "SASL password for Kafka authentication. Only used when `sasl` is `true`.",
			},
			"sasl_mechanism": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "SASL mechanism (plain, scram-sha-256, scram-sha-512). Only used when `sasl` is `true`.",
			},
			"tls": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable TLS for Kafka connections.",
			},
			"tls_skip_verify": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Skip TLS certificate verification.",
			},
			// RequiredWith keeps a cert-only configuration out of the plan: the
			// readback would put the server's key path into state and the empty
			// config value would then clear it, which MinIO rejects as a broken
			// pair. The key alone stays allowed: removing the certificate while
			// the key remains is how the pair gets cleared.
			"client_tls_cert": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"client_tls_key"},
				Description:  "Path to client TLS certificate.",
			},
			"client_tls_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to client TLS private key.",
			},
			"version": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Kafka protocol version.",
			},
		})),
	}
}

func buildAuditKafkaCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string
	notifyBuildCfgAddParam(&parts, "brokers", d.Get("brokers").(string))
	notifyBuildCfgAddParam(&parts, "topic", d.Get("topic").(string))
	notifyBuildCfgClearablePair(&parts, d, "sasl_username", "sasl_password")
	notifyBuildCfgAddParam(&parts, "sasl_mechanism", getOptionalField(d, "sasl_mechanism", "").(string))
	notifyBuildCfgClearablePair(&parts, d, "client_tls_cert", "client_tls_key")
	notifyBuildCfgClearableParam(&parts, d, "version")
	// d.Get, not GetOk: GetOk reads false as unset, so a target written
	// with tls=on kept tls=on on the server after the configuration
	// turned it off. See issue #1331.
	notifyBuildCfgAddBool(&parts, "sasl", d.Get("sasl").(bool))
	notifyBuildCfgAddBool(&parts, "tls", d.Get("tls").(bool))
	notifyBuildCfgAddBool(&parts, "tls_skip_verify", d.Get("tls_skip_verify").(bool))
	notifyBuildQueueSizeCommonCfg(&parts, d)
	return strings.Join(parts, " ")
}

func readAuditKafkaFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if v, ok := cfgMap["brokers"]; ok {
		_ = d.Set("brokers", v)
	}
	if v, ok := cfgMap["topic"]; ok {
		_ = d.Set("topic", v)
	}
	if v, ok := cfgMap["sasl_username"]; ok {
		_ = d.Set("sasl_username", v)
	}
	if v, ok := cfgMap["sasl_mechanism"]; ok {
		_ = d.Set("sasl_mechanism", v)
	}
	if err := d.Set("sasl", cfgMap["sasl"] == "on"); err != nil {
		return NewResourceError("setting sasl", d.Id(), err)
	}
	if v, ok := cfgMap["client_tls_cert"]; ok {
		_ = d.Set("client_tls_cert", v)
	}
	if v, ok := cfgMap["client_tls_key"]; ok {
		if setErr := d.Set("client_tls_key", v); setErr != nil {
			return diag.FromErr(setErr)
		}
	}
	if v, ok := cfgMap["version"]; ok {
		_ = d.Set("version", v)
	}
	if v, ok := cfgMap["tls"]; ok {
		_ = d.Set("tls", v == "on")
	}
	if v, ok := cfgMap["tls_skip_verify"]; ok {
		_ = d.Set("tls_skip_verify", v == "on")
	}
	notifyReadQueueSize(cfgMap, d)
	notifyReadCommonFields(cfgMap, d)
	return nil
}
