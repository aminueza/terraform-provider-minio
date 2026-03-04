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
	notifyBuildCfgAddParam(&parts, "password", getOptionalField(d, "password", "").(string))
	notifyBuildCfgAddParam(&parts, "user", getOptionalField(d, "user", "").(string))

	notifyBuildCommonCfg(&parts, d, meta)

	return strings.Join(parts, " ")
}

func readNotifyRedisFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {

	if v, ok := cfgMap["address"]; ok && v != "" {
		_ = d.Set("address", v)
	}

	if v, ok := cfgMap["key"]; ok && v != "" {
		_ = d.Set("key", v)
	}

	if v, ok := cfgMap["format"]; ok && v != "" {
		_ = d.Set("format", v)
	}

	if v, ok := cfgMap["user"]; ok && v != "" {
		_ = d.Set("user", v)
	}

	notifyReadCommonFields(cfgMap, d)

	return nil
}
