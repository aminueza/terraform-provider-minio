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
				Description: "Path to NATS user credentials file.",
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
			"client_cert": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to the client certificate for mTLS authentication.",
			},
			"client_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to the client private key for mTLS authentication. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
		}),
	}
}

func buildNotifyNatsCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	notifyBuildCfgAddParam(&parts, "address", d.Get("address").(string))
	notifyBuildCfgAddParam(&parts, "subject", d.Get("subject").(string))
	notifyBuildCfgAddParam(&parts, "username", getOptionalField(d, "username", "").(string))
	notifyBuildCfgAddParam(&parts, "password", getOptionalField(d, "password", "").(string))
	notifyBuildCfgAddParam(&parts, "token", getOptionalField(d, "token", "").(string))
	notifyBuildCfgAddParam(&parts, "user_credentials", getOptionalField(d, "user_credentials", "").(string))
	notifyBuildCfgAddBool(&parts, "tls", getOptionalField(d, "tls", false).(bool))
	notifyBuildCfgAddBool(&parts, "tls_skip_verify", getOptionalField(d, "tls_skip_verify", false).(bool))
	notifyBuildCfgAddParam(&parts, "ping_interval", getOptionalField(d, "ping_interval", "").(string))
	notifyBuildCfgAddBool(&parts, "jetstream", getOptionalField(d, "jetstream", false).(bool))
	notifyBuildCfgAddBool(&parts, "streaming", getOptionalField(d, "streaming", false).(bool))
	notifyBuildCfgAddBool(&parts, "streaming_async", getOptionalField(d, "streaming_async", false).(bool))
	notifyBuildCfgAddInt(&parts, "streaming_max_pub_acks_in_flight", getOptionalField(d, "streaming_max_pub_acks_in_flight", 0).(int))
	notifyBuildCfgAddParam(&parts, "streaming_cluster_id", getOptionalField(d, "streaming_cluster_id", "").(string))
	notifyBuildCfgAddParam(&parts, "cert_authority", getOptionalField(d, "cert_authority", "").(string))
	notifyBuildCfgAddParam(&parts, "client_cert", getOptionalField(d, "client_cert", "").(string))
	notifyBuildCfgAddParam(&parts, "client_key", getOptionalField(d, "client_key", "").(string))

	notifyBuildCommonCfg(&parts, d, meta)

	return strings.Join(parts, " ")
}

func readNotifyNatsFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if v, ok := cfgMap["address"]; ok {
		_ = d.Set("address", v)
	}
	if v, ok := cfgMap["subject"]; ok {
		_ = d.Set("subject", v)
	}
	if v, ok := cfgMap["username"]; ok && v != "" {
		_ = d.Set("username", v)
	}
	if v, ok := cfgMap["user_credentials"]; ok && v != "" {
		_ = d.Set("user_credentials", v)
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
	if v, ok := cfgMap["ping_interval"]; ok && v != "" {
		_ = d.Set("ping_interval", v)
	}
	if v, ok := cfgMap["jetstream"]; ok {
		_ = d.Set("jetstream", v == "on")
	} else {
		_ = d.Set("jetstream", false)
	}
	if v, ok := cfgMap["streaming"]; ok {
		_ = d.Set("streaming", v == "on")
	} else {
		_ = d.Set("streaming", false)
	}
	if v, ok := cfgMap["streaming_async"]; ok {
		_ = d.Set("streaming_async", v == "on")
	} else {
		_ = d.Set("streaming_async", false)
	}
	if v, ok := cfgMap["streaming_max_pub_acks_in_flight"]; ok {
		if n, err := parseInt(v); err == nil {
			_ = d.Set("streaming_max_pub_acks_in_flight", n)
		}
	}
	if v, ok := cfgMap["streaming_cluster_id"]; ok && v != "" {
		_ = d.Set("streaming_cluster_id", v)
	}
	if v, ok := cfgMap["cert_authority"]; ok && v != "" {
		_ = d.Set("cert_authority", v)
	}
	if v, ok := cfgMap["client_cert"]; ok && v != "" {
		_ = d.Set("client_cert", v)
	}

	notifyReadCommonFields(cfgMap, d)

	return nil
}
