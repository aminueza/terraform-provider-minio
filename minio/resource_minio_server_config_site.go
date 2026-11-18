package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const serverConfigSiteKey = "site"

func resourceMinioServerConfigSite() *schema.Resource {
	return &schema.Resource{
		Description: "Manages the MinIO site configuration (site name and region). The site " +
			"subsystem replaces the legacy region subsystem managed by " +
			"`minio_server_config_region`; when `region` is set here it takes precedence " +
			"over the legacy region for the region MinIO reports.",
		CreateContext: minioServerConfigSiteSet,
		ReadContext:   minioServerConfigSiteRead,
		UpdateContext: minioServerConfigSiteSet,
		DeleteContext: minioServerConfigSiteDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Site name (e.g., \"dc1-rack2\").",
			},
			"region": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateConfigValue,
				Description:  "Region of the site (e.g., \"us-east-1\"). When set, it overrides the legacy `region` subsystem for the region MinIO reports. Removing the attribute or setting it to \"\" clears the site region.",
			},
			"comment": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateConfigValue,
				Description:  "Comment or description for the site configuration.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether a MinIO server restart is required.",
			},
		},
	}
}

func minioServerConfigSiteSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigSiteConfig(d, meta)

	tflog.Debug(ctx, "Setting site configuration")

	cfgData := buildServerConfigSiteCfgData(config)
	restart, err := config.MinioAdmin.SetConfigKV(ctx, serverConfigSiteKey+" "+cfgData)
	if err != nil {
		return NewResourceError("setting site configuration", serverConfigSiteKey, err)
	}

	d.SetId(serverConfigSiteKey)
	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", serverConfigSiteKey, setErr)
	}

	tflog.Debug(ctx, fmt.Sprintf("Set site config (restart_required=%v)", restart))

	return minioServerConfigSiteRead(ctx, d, meta)
}

func minioServerConfigSiteRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigSiteConfig(d, meta)

	tflog.Debug(ctx, "Reading site configuration")

	configData, err := config.MinioAdmin.GetConfigKV(ctx, serverConfigSiteKey)
	if err != nil {
		return NewResourceError("reading site configuration", serverConfigSiteKey, err)
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, serverConfigSiteKey+" ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	d.SetId(serverConfigSiteKey)

	if v, ok := cfgMap["name"]; ok {
		if setErr := d.Set("name", v); setErr != nil {
			return NewResourceError("setting name", serverConfigSiteKey, setErr)
		}
	}
	if v, ok := cfgMap["region"]; ok {
		if setErr := d.Set("region", v); setErr != nil {
			return NewResourceError("setting region", serverConfigSiteKey, setErr)
		}
	}
	if v, ok := cfgMap["comment"]; ok {
		if setErr := d.Set("comment", v); setErr != nil {
			return NewResourceError("setting comment", serverConfigSiteKey, setErr)
		}
	}

	return nil
}

func minioServerConfigSiteDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigSiteConfig(d, meta)

	tflog.Debug(ctx, "Deleting site configuration")

	_, err := config.MinioAdmin.DelConfigKV(ctx, serverConfigSiteKey)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			return NewResourceError("resetting site configuration", serverConfigSiteKey, err)
		}
	}

	d.SetId("")
	tflog.Debug(ctx, "Deleted site configuration")

	return nil
}

// buildServerConfigSiteCfgData builds the key=value string sent to
// set-config-kv. The site subsystem is a singleton, so the resource owns every
// key: a key the configuration leaves empty is sent as key= so the server
// clears whatever it stored, including a value set outside Terraform.
func buildServerConfigSiteCfgData(config *S3MinioServerConfigSite) string {
	parts := make([]string, 0, 3)

	parts = append(parts, buildConfigParam("name", config.Name))

	if config.Region != "" {
		parts = append(parts, buildConfigParam("region", config.Region))
	} else {
		parts = append(parts, "region=")
	}

	if config.Comment != "" {
		parts = append(parts, buildConfigParam("comment", config.Comment))
	} else {
		parts = append(parts, "comment=")
	}

	return strings.Join(parts, " ")
}
