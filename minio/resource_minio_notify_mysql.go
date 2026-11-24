package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioNotifyMysql() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_mysql",
		buildCfg:   buildNotifyMysqlCfg,
		readFields: readNotifyMysqlFields,
	}
	return &schema.Resource{
		Description:   "Manages a MySQL notification target for MinIO bucket event notifications.",
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
				Description: "MySQL DSN connection string (e.g., 'user:password@tcp(host:port)/database'). Stored on the server as the `dsn_string` key.",
			},
			"table": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "MySQL table name for storing event records.",
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
				Description: "Maximum number of open connections to the MySQL database.",
			},
		}),
	}
}

func buildNotifyMysqlCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	// MinIO's notify_mysql key for the DSN is dsn_string; the server drops a
	// connection_string key, which left every target without a DSN.
	notifyBuildCfgAddParam(&parts, "dsn_string", d.Get("connection_string").(string))
	notifyBuildCfgAddParam(&parts, "table", d.Get("table").(string))
	notifyBuildCfgAddParam(&parts, "format", d.Get("format").(string))
	notifyBuildCfgAddInt(&parts, "max_open_connections", getOptionalField(d, "max_open_connections", 0).(int))

	notifyBuildCommonCfg(&parts, d, meta)

	return strings.Join(parts, " ")
}

func readNotifyMysqlFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if v, ok := cfgMap["dsn_string"]; ok {
		if diags := setResourceField(d, "connection_string", v, d.Id()); diags != nil {
			return diags
		}
	}
	if diags := notifySetPresentStrings(d, cfgMap, "table", "format"); diags != nil {
		return diags
	}
	if diags := notifySetInt(d, cfgMap, "max_open_connections", "max_open_connections"); diags != nil {
		return diags
	}
	return notifyReadCommonFields(cfgMap, d)
}
