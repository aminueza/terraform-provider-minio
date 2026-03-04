package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioNotifyPostgres() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_postgres",
		buildCfg:   buildNotifyPostgresCfg,
		readFields: readNotifyPostgresFields,
	}
	return &schema.Resource{
		Description:   "Manages a PostgreSQL notification target for MinIO bucket event notifications.",
		CreateContext: notifyCreate(nrc),
		ReadContext:   notifyRead(nrc),
		UpdateContext: notifyUpdate(nrc),
		DeleteContext: notifyDelete(nrc),
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: mergeSchemas(notifyCommonSchema(), map[string]*schema.Schema{
			"connection_string": {
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
				Description: "PostgreSQL connection string (e.g., 'host=localhost port=5432 dbname=minio user=minio password=secret sslmode=disable'). MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"table": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the PostgreSQL table for event records.",
			},
			"format": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Output format for event records: 'namespace' or 'access'.",
			},
			"max_open_connections": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "Maximum number of open connections to the PostgreSQL database.",
			},
		}),
	}
}

func buildNotifyPostgresCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	notifyBuildCfgAddParam(&parts, "connection_string", d.Get("connection_string").(string))
	notifyBuildCfgAddParam(&parts, "table", d.Get("table").(string))
	notifyBuildCfgAddParam(&parts, "format", d.Get("format").(string))
	notifyBuildCfgAddInt(&parts, "max_open_connections", getOptionalField(d, "max_open_connections", 0).(int))

	notifyBuildCommonCfg(&parts, d, meta)

	return strings.Join(parts, " ")
}

func readNotifyPostgresFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {

	if v, ok := cfgMap["table"]; ok && v != "" {
		_ = d.Set("table", v)
	}

	if v, ok := cfgMap["format"]; ok && v != "" {
		_ = d.Set("format", v)
	}

	if v, ok := cfgMap["max_open_connections"]; ok {
		if n, err := parseInt(v); err == nil {
			_ = d.Set("max_open_connections", n)
		}
	}

	notifyReadCommonFields(cfgMap, d)

	return nil
}
