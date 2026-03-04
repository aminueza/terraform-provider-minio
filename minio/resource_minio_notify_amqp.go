package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioNotifyAmqp() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_amqp",
		buildCfg:   buildNotifyAmqpCfg,
		readFields: readNotifyAmqpFields,
	}

	return &schema.Resource{
		Description:   "Manages an AMQP/RabbitMQ notification target for MinIO bucket event notifications.",
		CreateContext: notifyCreate(nrc),
		ReadContext:   notifyRead(nrc),
		UpdateContext: notifyUpdate(nrc),
		DeleteContext: notifyDelete(nrc),
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: mergeSchemas(notifyCommonSchema(), map[string]*schema.Schema{
			"url": {
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
				Description: "AMQP connection URL (e.g., 'amqp://user:pass@host:5672'). MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"exchange": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "AMQP exchange name.",
			},
			"exchange_type": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "AMQP exchange type (e.g., 'direct', 'fanout', 'topic', 'headers').",
			},
			"routing_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "AMQP routing key for message delivery.",
			},
			"mandatory": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to set the mandatory flag on published messages.",
			},
			"durable": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether the AMQP queue is durable (survives broker restart).",
			},
			"no_wait": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to set the no-wait flag on queue declaration.",
			},
			"internal": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether the exchange is internal.",
			},
			"auto_deleted": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether the queue is auto-deleted when the last consumer unsubscribes.",
			},
			"delivery_mode": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "AMQP delivery mode: 1 for non-persistent, 2 for persistent.",
			},
		}),
	}
}

func buildNotifyAmqpCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	notifyBuildCfgAddParam(&parts, "url", d.Get("url").(string))
	notifyBuildCfgAddParam(&parts, "exchange", getOptionalField(d, "exchange", "").(string))
	notifyBuildCfgAddParam(&parts, "exchange_type", getOptionalField(d, "exchange_type", "").(string))
	notifyBuildCfgAddParam(&parts, "routing_key", getOptionalField(d, "routing_key", "").(string))
	notifyBuildCfgAddBool(&parts, "mandatory", getOptionalField(d, "mandatory", false).(bool))
	notifyBuildCfgAddBool(&parts, "durable", getOptionalField(d, "durable", false).(bool))
	notifyBuildCfgAddBool(&parts, "no_wait", getOptionalField(d, "no_wait", false).(bool))
	notifyBuildCfgAddBool(&parts, "internal", getOptionalField(d, "internal", false).(bool))
	notifyBuildCfgAddBool(&parts, "auto_deleted", getOptionalField(d, "auto_deleted", false).(bool))
	notifyBuildCfgAddInt(&parts, "delivery_mode", getOptionalField(d, "delivery_mode", 0).(int))

	notifyBuildCommonCfg(&parts, d, meta)
	return strings.Join(parts, " ")
}

func readNotifyAmqpFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {

	if v, ok := cfgMap["exchange"]; ok {
		_ = d.Set("exchange", v)
	}
	if v, ok := cfgMap["exchange_type"]; ok {
		_ = d.Set("exchange_type", v)
	}
	if v, ok := cfgMap["routing_key"]; ok {
		_ = d.Set("routing_key", v)
	}
	if v, ok := cfgMap["mandatory"]; ok {
		_ = d.Set("mandatory", v == "on")
	} else {
		_ = d.Set("mandatory", false)
	}
	if v, ok := cfgMap["durable"]; ok {
		_ = d.Set("durable", v == "on")
	} else {
		_ = d.Set("durable", false)
	}
	if v, ok := cfgMap["no_wait"]; ok {
		_ = d.Set("no_wait", v == "on")
	} else {
		_ = d.Set("no_wait", false)
	}
	if v, ok := cfgMap["internal"]; ok {
		_ = d.Set("internal", v == "on")
	} else {
		_ = d.Set("internal", false)
	}
	if v, ok := cfgMap["auto_deleted"]; ok {
		_ = d.Set("auto_deleted", v == "on")
	} else {
		_ = d.Set("auto_deleted", false)
	}
	if v, ok := cfgMap["delivery_mode"]; ok {
		if n, err := parseInt(v); err == nil {
			_ = d.Set("delivery_mode", n)
		}
	}

	notifyReadCommonFields(cfgMap, d)
	return nil
}
