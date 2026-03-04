package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioServerConfigHeal() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages MinIO object healing configuration. Controls background bitrot scanning and data repair settings.",
		CreateContext: minioServerConfigHealSet,
		ReadContext:   minioServerConfigHealRead,
		UpdateContext: minioServerConfigHealSet,
		DeleteContext: minioServerConfigHealDelete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"bitrotscan": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Bitrot scan mode: \"on\", \"off\", or cycle duration (e.g., \"12m\" for monthly).",
			},
			"max_sleep": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Maximum sleep between heal operations (e.g., \"250ms\").",
			},
			"max_io": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Maximum concurrent I/O operations for healing.",
			},
			"drive_workers": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Number of workers per drive for healing. Empty for auto (1/4 CPU cores).",
			},
			"restart_required": {
				Type:     schema.TypeBool,
				Computed: true,
			},
		},
	}
}

func minioServerConfigHealSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	var parts []string
	for _, f := range []string{"bitrotscan", "max_sleep", "max_io", "drive_workers"} {
		if v, ok := d.GetOk(f); ok {
			parts = append(parts, fmt.Sprintf("%s=%s", f, v.(string)))
		}
	}

	if len(parts) == 0 {
		d.SetId("heal")
		return minioServerConfigHealRead(ctx, d, meta)
	}

	configString := "heal " + strings.Join(parts, " ")
	restart, err := admin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("setting heal configuration", "heal", err)
	}

	d.SetId("heal")
	_ = d.Set("restart_required", restart)
	log.Printf("[DEBUG] Set heal config (restart_required=%v)", restart)

	return minioServerConfigHealRead(ctx, d, meta)
}

func minioServerConfigHealRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	configData, err := admin.GetConfigKV(ctx, "heal")
	if err != nil {
		return NewResourceError("reading heal configuration", "heal", err)
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, "heal ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	d.SetId("heal")

	for _, f := range []string{"bitrotscan", "max_sleep", "max_io", "drive_workers"} {
		if v, ok := cfgMap[f]; ok && v != "" {
			_ = d.Set(f, v)
		}
	}

	return nil
}

func minioServerConfigHealDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	_, err := admin.DelConfigKV(ctx, "heal")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			return NewResourceError("resetting heal configuration", "heal", err)
		}
	}
	d.SetId("")
	return nil
}
