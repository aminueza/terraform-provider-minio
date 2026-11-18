package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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
		CustomizeDiff: scannerCorruptedCycleCustomizeDiff,
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

// scannerCorruptedCycleCustomizeDiff forces a cycle diff when an older
// provider left the excess thresholds glued onto cycle (issue #1322) and the
// configuration does not set cycle: cycle is Optional and Computed, so the
// glued state value would otherwise never show in a plan, and the apply that
// rewrites a clean cycle would never run.
func scannerCorruptedCycleCustomizeDiff(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
	cycle := d.Get("cycle").(string)
	clean := cycle
	for _, glued := range []string{" excess_versions=", " excess_folders="} {
		if i := strings.Index(clean, glued); i >= 0 {
			clean = clean[:i]
		}
	}
	if clean == cycle {
		return nil
	}
	// The raw config is a null object when no configuration is available, for
	// example on the plan right after an import, and GetAttr panics on it.
	rawConfig := d.GetRawConfig()
	if rawConfig.IsNull() || !rawConfig.GetAttr("cycle").IsNull() {
		return nil
	}
	return d.SetNew("cycle", clean)
}

// scannerConfigFields maps the Terraform attributes to the scanner
// subsystem's keys. The excess thresholds are stored as alert_excess_*:
// MinIO accepts an unknown key without an error and glues it onto the value
// written before it, here cycle (issue #1322).
var scannerConfigFields = []struct {
	attr string
	key  string
}{
	{"speed", "speed"},
	{"delay", "delay"},
	{"max_wait", "max_wait"},
	{"cycle", "cycle"},
	{"excess_versions", "alert_excess_versions"},
	{"excess_folders", "alert_excess_folders"},
}

func buildScannerCfg(d *schema.ResourceData) string {
	var parts []string
	for _, f := range scannerConfigFields {
		if v, ok := d.GetOk(f.attr); ok {
			parts = append(parts, fmt.Sprintf("%s=%s", f.key, v.(string)))
		}
	}
	return strings.Join(parts, " ")
}

func minioServerConfigScannerSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	cfg := buildScannerCfg(d)
	if cfg == "" {
		d.SetId("scanner")
		return minioServerConfigScannerRead(ctx, d, meta)
	}

	restart, err := admin.SetConfigKV(ctx, "scanner "+cfg)
	if err != nil {
		return NewResourceError("setting scanner configuration", "scanner", err)
	}

	d.SetId("scanner")
	_ = d.Set("restart_required", restart)
	tflog.Debug(ctx, fmt.Sprintf("Set scanner config (restart_required=%v)", restart))

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

	for _, f := range scannerConfigFields {
		v, ok := cfgMap[f.key]
		if !ok {
			continue
		}
		if err := d.Set(f.attr, v); err != nil {
			return NewResourceError("setting "+f.attr, d.Id(), err)
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
