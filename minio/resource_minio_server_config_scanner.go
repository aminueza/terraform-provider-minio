package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceMinioServerConfigScanner() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages MinIO object scanner configuration. The scanner handles background tasks like lifecycle expiration, healing, and versioning cleanup.",
		CreateContext: minioServerConfigScannerSet,
		ReadContext:   minioServerConfigScannerRead,
		UpdateContext: minioServerConfigScannerSet,
		DeleteContext: minioServerConfigScannerDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"speed": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validation.StringInSlice([]string{"fastest", "fast", "default", "slow", "slowest"}, false),
				Description:  "Scanner speed preset: fastest, fast, default, slow, or slowest.",
			},
			"delay": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Scanner delay multiplier between operations.",
			},
			"max_wait": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Maximum wait between scanner cycles (e.g., \"15s\").",
			},
			"cycle": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Time between full scanner cycles (e.g., \"1m\").",
			},
			"excess_versions": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Alert threshold for excess object versions per prefix.",
			},
			"excess_folders": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Alert threshold for excess folders per prefix.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether a MinIO server restart is required.",
			},
		},
	}
}

func minioServerConfigScannerSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	var parts []string
	fields := []string{"speed", "delay", "max_wait", "cycle", "excess_versions", "excess_folders"}
	for _, f := range fields {
		if v, ok := d.GetOk(f); ok {
			parts = append(parts, fmt.Sprintf("%s=%s", f, v.(string)))
		}
	}

	if len(parts) == 0 {
		d.SetId("scanner")
		return minioServerConfigScannerRead(ctx, d, meta)
	}

	configString := "scanner " + strings.Join(parts, " ")
	restart, err := admin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("setting scanner configuration", "scanner", err)
	}

	d.SetId("scanner")
	_ = d.Set("restart_required", restart)
	log.Printf("[DEBUG] Set scanner config (restart_required=%v)", restart)

	return minioServerConfigScannerRead(ctx, d, meta)
}

func minioServerConfigScannerRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	configData, err := admin.GetConfigKV(ctx, "scanner")
	if err != nil {
		return NewResourceError("reading scanner configuration", "scanner", err)
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, "scanner ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	d.SetId("scanner")

	for _, f := range []string{"speed", "delay", "max_wait", "cycle", "excess_versions", "excess_folders"} {
		if v, ok := cfgMap[f]; ok {
			_ = d.Set(f, v)
		}
	}

	return nil
}

func minioServerConfigScannerDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	_, err := admin.DelConfigKV(ctx, "scanner")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			return NewResourceError("resetting scanner configuration", "scanner", err)
		}
	}
	d.SetId("")
	return nil
}
