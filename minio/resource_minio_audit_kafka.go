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
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: mergeSchemas(notifyCommonSchema(), map[string]*schema.Schema{
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
			"sasl_username": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "SASL username for Kafka authentication.",
			},
			"sasl_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "SASL password for Kafka authentication.",
			},
			"sasl_mechanism": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "SASL mechanism (plain, scram-sha-256, scram-sha-512).",
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
			"client_tls_cert": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to client TLS certificate.",
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
		}),
	}
}

func buildAuditKafkaCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string
	notifyBuildCfgAddParam(&parts, "brokers", d.Get("brokers").(string))
	notifyBuildCfgAddParam(&parts, "topic", d.Get("topic").(string))
	notifyBuildCfgAddParam(&parts, "sasl_username", getOptionalField(d, "sasl_username", "").(string))
	notifyBuildCfgAddParam(&parts, "sasl_password", getOptionalField(d, "sasl_password", "").(string))
	notifyBuildCfgAddParam(&parts, "sasl_mechanism", getOptionalField(d, "sasl_mechanism", "").(string))
	notifyBuildCfgAddParam(&parts, "client_tls_cert", getOptionalField(d, "client_tls_cert", "").(string))
	notifyBuildCfgAddParam(&parts, "client_tls_key", getOptionalField(d, "client_tls_key", "").(string))
	notifyBuildCfgAddParam(&parts, "version", getOptionalField(d, "version", "").(string))
	if v, ok := d.GetOk("tls"); ok {
		notifyBuildCfgAddBool(&parts, "tls", v.(bool))
	}
	if v, ok := d.GetOk("tls_skip_verify"); ok {
		notifyBuildCfgAddBool(&parts, "tls_skip_verify", v.(bool))
	}
	notifyBuildCommonCfg(&parts, d, meta)
	return strings.Join(parts, " ")
}

func readAuditKafkaFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if v, ok := cfgMap["brokers"]; ok {
		_ = d.Set("brokers", v)
	}
	if v, ok := cfgMap["topic"]; ok {
		_ = d.Set("topic", v)
	}
	if v, ok := cfgMap["sasl_username"]; ok && v != "" {
		_ = d.Set("sasl_username", v)
	}
	if v, ok := cfgMap["sasl_mechanism"]; ok && v != "" {
		_ = d.Set("sasl_mechanism", v)
	}
	if v, ok := cfgMap["client_tls_cert"]; ok && v != "" {
		_ = d.Set("client_tls_cert", v)
	}
	if v, ok := cfgMap["version"]; ok && v != "" {
		_ = d.Set("version", v)
	}
	if v, ok := cfgMap["tls"]; ok {
		_ = d.Set("tls", v == "on")
	}
	if v, ok := cfgMap["tls_skip_verify"]; ok {
		_ = d.Set("tls_skip_verify", v == "on")
	}
	notifyReadCommonFields(cfgMap, d)
	return nil
}
