package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioServerConfigRegion() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages MinIO server region/site name configuration.",
		CreateContext: minioServerConfigRegionSet,
		ReadContext:   minioServerConfigRegionRead,
		UpdateContext: minioServerConfigRegionSet,
		DeleteContext: minioServerConfigRegionDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Region or site name (e.g., \"us-east-1\", \"dc1-rack2\").",
			},
			"comment": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Region description.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether a MinIO server restart is required.",
			},
		},
	}
}

func minioServerConfigRegionSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	var parts []string
	parts = append(parts, fmt.Sprintf("name=%s", d.Get("name").(string)))
	if v, ok := d.GetOk("comment"); ok && v.(string) != "" {
		val := v.(string)
		if strings.ContainsAny(val, " \t") {
			parts = append(parts, fmt.Sprintf("comment=%q", val))
		} else {
			parts = append(parts, fmt.Sprintf("comment=%s", val))
		}
	}

	configString := "region " + strings.Join(parts, " ")
	restart, err := admin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("setting region configuration", "region", err)
	}

	d.SetId("region")
	_ = d.Set("restart_required", restart)
	log.Printf("[DEBUG] Set region config (restart_required=%v)", restart)

	if restart {
		return nil
	}
	return minioServerConfigRegionRead(ctx, d, meta)
}

func minioServerConfigRegionRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	configData, err := admin.GetConfigKV(ctx, "region")
	if err != nil {
		return NewResourceError("reading region configuration", "region", err)
	}

	configStr := strings.TrimSpace(string(configData))
	log.Printf("[DEBUG] Raw config data for region: %s", configStr)
	var valueStr string
	if strings.HasPrefix(configStr, "region ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	d.SetId("region")

	if v, ok := cfgMap["name"]; ok && v != "" {
		_ = d.Set("name", v)
	}
	if v, ok := cfgMap["comment"]; ok && v != "" {
		_ = d.Set("comment", v)
	}

	return nil
}

func minioServerConfigRegionDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	_, err := admin.DelConfigKV(ctx, "region")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			return NewResourceError("resetting region configuration", "region", err)
		}
	}
	d.SetId("")
	return nil
}
