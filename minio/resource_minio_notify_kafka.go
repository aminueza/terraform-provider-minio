package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioNotifyKafka() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_kafka",
		buildCfg:   buildNotifyKafkaCfg,
		readFields: readNotifyKafkaFields,
	}

	return &schema.Resource{
		Description:   "Manages a Kafka notification target for MinIO bucket event notifications.",
		CreateContext: notifyCreate(nrc),
		ReadContext:   notifyRead(nrc),
		UpdateContext: notifyUpdate(nrc),
		DeleteContext: notifyDelete(nrc),
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: mergeSchemas(notifyCommonSchema(), map[string]*schema.Schema{
			"brokers": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Comma-separated list of Kafka broker addresses (e.g., 'host1:9092,host2:9092').",
			},
			"topic": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Kafka topic to publish event notifications to.",
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
				Description: "SASL password for Kafka authentication. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"sasl_mechanism": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "SASL authentication mechanism: 'plain', 'scram-sha-256', or 'scram-sha-512'.",
			},
			"tls": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to enable TLS for the Kafka connection.",
			},
			"tls_skip_verify": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to skip TLS certificate verification.",
			},
			"tls_client_auth": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "TLS client authentication type (0=NoClientCert, 1=RequestClientCert, etc.).",
			},
			"client_tls_cert": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to the client TLS certificate for mTLS authentication.",
			},
			"client_tls_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to the client TLS private key. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"version": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Kafka cluster version (e.g., '2.8.0').",
			},
			"batch_size": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "Number of messages to batch before sending to Kafka.",
			},
		}),
	}
}

func buildNotifyKafkaCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	notifyBuildCfgAddParam(&parts, "brokers", d.Get("brokers").(string))
	notifyBuildCfgAddParam(&parts, "topic", d.Get("topic").(string))
	notifyBuildCfgAddParam(&parts, "sasl_username", getOptionalField(d, "sasl_username", "").(string))
	notifyBuildCfgAddParam(&parts, "sasl_password", getOptionalField(d, "sasl_password", "").(string))
	notifyBuildCfgAddParam(&parts, "sasl_mechanism", getOptionalField(d, "sasl_mechanism", "").(string))
	notifyBuildCfgAddBool(&parts, "tls", getOptionalField(d, "tls", false).(bool))
	notifyBuildCfgAddBool(&parts, "tls_skip_verify", getOptionalField(d, "tls_skip_verify", false).(bool))
	notifyBuildCfgAddInt(&parts, "tls_client_auth", getOptionalField(d, "tls_client_auth", 0).(int))
	notifyBuildCfgAddParam(&parts, "client_tls_cert", getOptionalField(d, "client_tls_cert", "").(string))
	notifyBuildCfgAddParam(&parts, "client_tls_key", getOptionalField(d, "client_tls_key", "").(string))
	notifyBuildCfgAddParam(&parts, "version", getOptionalField(d, "version", "").(string))
	notifyBuildCfgAddInt(&parts, "batch_size", getOptionalField(d, "batch_size", 0).(int))

	notifyBuildCommonCfg(&parts, d, meta)
	return strings.Join(parts, " ")
}

func readNotifyKafkaFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
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
	if v, ok := cfgMap["tls"]; ok {
		_ = d.Set("tls", v == "on")
	} else {
		_ = d.Set("tls", false)
	}
	if v, ok := cfgMap["tls_skip_verify"]; ok {
		_ = d.Set("tls_skip_verify", v == "on")
	} else {
		_ = d.Set("tls_skip_verify", false)
	}
	if v, ok := cfgMap["tls_client_auth"]; ok {
		if n, err := parseInt(v); err == nil {
			_ = d.Set("tls_client_auth", n)
		}
	}
	if v, ok := cfgMap["client_tls_cert"]; ok {
		_ = d.Set("client_tls_cert", v)
	}
	if v, ok := cfgMap["version"]; ok {
		_ = d.Set("version", v)
	}
	if v, ok := cfgMap["batch_size"]; ok {
		if n, err := parseInt(v); err == nil {
			_ = d.Set("batch_size", n)
		}
	}

	notifyReadCommonFields(cfgMap, d)
	return nil
}
