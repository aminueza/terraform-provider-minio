package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioNotifyRedis() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_redis",
		buildCfg:   buildNotifyRedisCfg,
		readFields: readNotifyRedisFields,
	}
	return &schema.Resource{
		Description:   "Manages a Redis notification target for MinIO bucket event notifications.",
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
				Description: "Redis server address (e.g., 'localhost:6379').",
			},
			"key": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Redis key name used to store or publish event records.",
			},
			"format": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Output format for event records: 'namespace' or 'access'.",
			},
			"password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Password for Redis authentication. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"user": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Username for Redis ACL authentication.",
			},
		}),
	}
}

func buildNotifyRedisCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	notifyBuildCfgAddParam(&parts, "address", d.Get("address").(string))
	notifyBuildCfgAddParam(&parts, "key", d.Get("key").(string))
	notifyBuildCfgAddParam(&parts, "format", d.Get("format").(string))
	notifyBuildCfgClearableParam(&parts, d, "password")
	notifyBuildCfgClearableParam(&parts, d, "user")

	notifyBuildCommonCfg(&parts, d, meta)

	return strings.Join(parts, " ")
}

func readNotifyRedisFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if diags := notifySetNonEmptyStrings(d, cfgMap, "address", "key", "format"); diags != nil {
		return diags
	}
	if diags := notifySetPresentStrings(d, cfgMap, "user"); diags != nil {
		return diags
	}
	return notifyReadCommonFields(cfgMap, d)
}
