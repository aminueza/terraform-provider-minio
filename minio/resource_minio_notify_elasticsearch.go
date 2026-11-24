package minio

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioNotifyElasticsearch() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_elasticsearch",
		buildCfg:   buildNotifyElasticsearchCfg,
		readFields: readNotifyElasticsearchFields,
	}
	return &schema.Resource{
		Description:   "Manages an Elasticsearch notification target for MinIO bucket event notifications.",
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
				Description: "Elasticsearch server URL (e.g., 'http://localhost:9200'). Read back from the server.",
			},
			"index": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the Elasticsearch index for event records.",
			},
			"format": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Output format for event records: 'namespace' or 'access'.",
			},
			"username": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Username for Elasticsearch authentication.",
			},
			"password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Password for Elasticsearch authentication. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
		}),
	}
}

func buildNotifyElasticsearchCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	notifyBuildCfgAddParam(&parts, "url", d.Get("url").(string))
	notifyBuildCfgAddParam(&parts, "index", d.Get("index").(string))
	notifyBuildCfgAddParam(&parts, "format", d.Get("format").(string))
	notifyBuildCfgClearablePair(&parts, d, "username", "password")

	notifyBuildCommonCfg(&parts, d, meta)

	return strings.Join(parts, " ")
}

func readNotifyElasticsearchFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if diags := notifySetPresentStrings(d, cfgMap, "url"); diags != nil {
		return diags
	}
	if diags := notifySetNonEmptyStrings(d, cfgMap, "index", "format"); diags != nil {
		return diags
	}
	if diags := notifySetPresentStrings(d, cfgMap, "username"); diags != nil {
		return diags
	}
	return notifyReadCommonFields(cfgMap, d)
}
