package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioNotifyMqtt() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_mqtt",
		buildCfg:   buildNotifyMqttCfg,
		readFields: readNotifyMqttFields,
	}

	return &schema.Resource{
		Description:   "Manages an MQTT notification target for MinIO bucket event notifications.",
		CreateContext: notifyCreate(nrc),
		ReadContext:   notifyRead(nrc),
		UpdateContext: notifyUpdate(nrc),
		DeleteContext: notifyDelete(nrc),
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: mergeSchemas(notifyCommonSchema(), map[string]*schema.Schema{
			"broker": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "MQTT broker URL (e.g., 'tcp://host:1883').",
			},
			"topic": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "MQTT topic to publish event notifications to.",
			},
			"username": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Username for MQTT broker authentication.",
			},
			"password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Password for MQTT broker authentication. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"qos": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "MQTT Quality of Service level: 0 (at most once), 1 (at least once), or 2 (exactly once).",
			},
			"keep_alive_interval": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "MQTT keep-alive interval duration (e.g., '10s').",
			},
			"reconnect_interval": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "MQTT reconnect interval duration (e.g., '5s').",
			},
		}),
	}
}

func buildNotifyMqttCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	notifyBuildCfgAddParam(&parts, "broker", d.Get("broker").(string))
	notifyBuildCfgAddParam(&parts, "topic", d.Get("topic").(string))
	notifyBuildCfgAddParam(&parts, "username", getOptionalField(d, "username", "").(string))
	notifyBuildCfgAddParam(&parts, "password", getOptionalField(d, "password", "").(string))
	notifyBuildCfgAddInt(&parts, "qos", getOptionalField(d, "qos", 0).(int))
	notifyBuildCfgAddParam(&parts, "keep_alive_interval", getOptionalField(d, "keep_alive_interval", "").(string))
	notifyBuildCfgAddParam(&parts, "reconnect_interval", getOptionalField(d, "reconnect_interval", "").(string))

	notifyBuildCommonCfg(&parts, d, meta)
	return strings.Join(parts, " ")
}

func readNotifyMqttFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if v, ok := cfgMap["broker"]; ok {
		_ = d.Set("broker", v)
	}
	if v, ok := cfgMap["topic"]; ok {
		_ = d.Set("topic", v)
	}
	if v, ok := cfgMap["username"]; ok {
		_ = d.Set("username", v)
	}
	if v, ok := cfgMap["qos"]; ok {
		if n, err := parseInt(v); err == nil {
			_ = d.Set("qos", n)
		}
	}
	if v, ok := cfgMap["keep_alive_interval"]; ok {
		_ = d.Set("keep_alive_interval", v)
	}
	if v, ok := cfgMap["reconnect_interval"]; ok {
		_ = d.Set("reconnect_interval", v)
	}

	notifyReadCommonFields(cfgMap, d)
	return nil
}
