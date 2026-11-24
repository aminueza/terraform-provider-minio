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
				Description: "AMQP connection URL (e.g., 'amqp://user:pass@host:5672'). Read back from the server.",
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
			"publisher_confirms": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether MinIO waits for the broker to confirm each published message and treats a rejected publish as an error. Use it together with `queue_dir` for guaranteed delivery.",
			},
		}),
	}
}

func buildNotifyAmqpCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	notifyBuildCfgAddParam(&parts, "url", d.Get("url").(string))
	notifyBuildCfgClearableParam(&parts, d, "exchange")
	notifyBuildCfgClearableParam(&parts, d, "exchange_type")
	notifyBuildCfgClearableParam(&parts, d, "routing_key")
	notifyBuildCfgAddBool(&parts, "mandatory", getOptionalField(d, "mandatory", false).(bool))
	notifyBuildCfgAddBool(&parts, "durable", getOptionalField(d, "durable", false).(bool))
	notifyBuildCfgAddBool(&parts, "no_wait", getOptionalField(d, "no_wait", false).(bool))
	notifyBuildCfgAddBool(&parts, "internal", getOptionalField(d, "internal", false).(bool))
	notifyBuildCfgAddBool(&parts, "auto_deleted", getOptionalField(d, "auto_deleted", false).(bool))
	notifyBuildCfgAddInt(&parts, "delivery_mode", getOptionalField(d, "delivery_mode", 0).(int))
	notifyBuildCfgAddBool(&parts, "publisher_confirms", d.Get("publisher_confirms").(bool))

	notifyBuildCommonCfg(&parts, d, meta)
	return strings.Join(parts, " ")
}

func readNotifyAmqpFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if diags := notifySetPresentStrings(d, cfgMap, "url", "exchange", "exchange_type", "routing_key"); diags != nil {
		return diags
	}
	if diags := notifySetOnBools(d, cfgMap, "mandatory", "durable", "no_wait", "internal", "auto_deleted"); diags != nil {
		return diags
	}
	if diags := notifySetInt(d, cfgMap, "delivery_mode", "delivery_mode"); diags != nil {
		return diags
	}
	if diags := notifySetOnBools(d, cfgMap, "publisher_confirms"); diags != nil {
		return diags
	}
	return notifyReadCommonFields(cfgMap, d)
}
