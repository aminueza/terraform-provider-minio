package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioNotifyNsq() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_nsq",
		buildCfg:   buildNotifyNsqCfg,
		readFields: readNotifyNsqFields,
	}
	return &schema.Resource{
		Description:   "Manages an NSQ notification target for MinIO bucket event notifications.",
		CreateContext: notifyCreate(nrc),
		ReadContext:   notifyRead(nrc),
		UpdateContext: notifyUpdate(nrc),
		DeleteContext: notifyDelete(nrc),
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: mergeSchemas(notifyCommonSchema(), map[string]*schema.Schema{
			"nsqd_address": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "NSQ daemon address (e.g., 'localhost:4150').",
			},
			"topic": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "NSQ topic to publish notifications to.",
			},
			"tls": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to enable TLS for the NSQ connection.",
			},
			"tls_skip_verify": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to skip TLS certificate verification.",
			},
		}),
	}
}

func buildNotifyNsqCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	notifyBuildCfgAddParam(&parts, "nsqd_address", d.Get("nsqd_address").(string))
	notifyBuildCfgAddParam(&parts, "topic", d.Get("topic").(string))
	notifyBuildCfgAddBool(&parts, "tls", getOptionalField(d, "tls", false).(bool))
	notifyBuildCfgAddBool(&parts, "tls_skip_verify", getOptionalField(d, "tls_skip_verify", false).(bool))

	notifyBuildCommonCfg(&parts, d, meta)

	return strings.Join(parts, " ")
}

func readNotifyNsqFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if v, ok := cfgMap["nsqd_address"]; ok {
		_ = d.Set("nsqd_address", v)
	}
	if v, ok := cfgMap["topic"]; ok {
		_ = d.Set("topic", v)
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

	notifyReadCommonFields(cfgMap, d)

	return nil
}
