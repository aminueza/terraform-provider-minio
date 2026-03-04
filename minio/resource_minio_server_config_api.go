package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioServerConfigApi() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages MinIO API server configuration including request throttling, CORS, transition workers, and stale upload cleanup.",
		CreateContext: minioServerConfigApiSet,
		ReadContext:   minioServerConfigApiRead,
		UpdateContext: minioServerConfigApiSet,
		DeleteContext: minioServerConfigApiDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"requests_max": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Maximum concurrent API requests. Use 0 or empty for auto.",
			},
			"cors_allow_origin": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Comma-separated list of allowed CORS origins (e.g., \"https://app.example.com\").",
			},
			"transition_workers": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Number of ILM transition workers.",
			},
			"stale_uploads_expiry": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Duration after which incomplete multipart uploads are cleaned up (e.g., \"24h\").",
			},
			"stale_uploads_cleanup_interval": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Interval between stale upload cleanup runs (e.g., \"6h\").",
			},
			"cluster_deadline": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Deadline for cluster read operations (e.g., \"10s\").",
			},
			"remote_transport_deadline": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Deadline for remote transport operations (e.g., \"2h\").",
			},
			"root_access": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Whether root user (access/secret key) access is enabled for S3 API.",
			},
			"sync_events": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Enable synchronous bucket notification events.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether a MinIO server restart is required.",
			},
		},
	}
}

func minioServerConfigApiSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	var parts []string
	setStr := func(key, attr string) {
		if v, ok := d.GetOk(attr); ok {
			parts = append(parts, fmt.Sprintf("%s=%s", key, v.(string)))
		}
	}
	setBool := func(key, attr string) {
		if d.HasChange(attr) || d.Get(attr) != nil {
			if v, ok := d.GetOk(attr); ok || d.HasChange(attr) {
				if v.(bool) {
					parts = append(parts, key+"=on")
				} else {
					parts = append(parts, key+"=off")
				}
			}
		}
	}

	setStr("requests_max", "requests_max")
	setStr("cors_allow_origin", "cors_allow_origin")
	setStr("transition_workers", "transition_workers")
	setStr("stale_uploads_expiry", "stale_uploads_expiry")
	setStr("stale_uploads_cleanup_interval", "stale_uploads_cleanup_interval")
	setStr("cluster_deadline", "cluster_deadline")
	setStr("remote_transport_deadline", "remote_transport_deadline")
	setBool("root_access", "root_access")
	setBool("sync_events", "sync_events")

	if len(parts) == 0 {
		d.SetId("api")
		return minioServerConfigApiRead(ctx, d, meta)
	}

	configString := "api " + strings.Join(parts, " ")
	restart, err := admin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("setting api configuration", "api", err)
	}

	d.SetId("api")
	_ = d.Set("restart_required", restart)
	log.Printf("[DEBUG] Set api config (restart_required=%v)", restart)

	return minioServerConfigApiRead(ctx, d, meta)
}

func minioServerConfigApiRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	configData, err := admin.GetConfigKV(ctx, "api")
	if err != nil {
		return NewResourceError("reading api configuration", "api", err)
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, "api ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)

	d.SetId("api")

	strFields := []string{
		"requests_max", "cors_allow_origin", "transition_workers",
		"stale_uploads_expiry", "stale_uploads_cleanup_interval",
		"cluster_deadline", "remote_transport_deadline",
	}
	for _, f := range strFields {
		if v, ok := cfgMap[f]; ok {
			_ = d.Set(f, v)
		}
	}

	boolFields := []string{"root_access", "sync_events"}
	for _, f := range boolFields {
		if v, ok := cfgMap[f]; ok {
			_ = d.Set(f, v == "on")
		}
	}

	return nil
}

func minioServerConfigApiDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	_, err := admin.DelConfigKV(ctx, "api")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			return NewResourceError("resetting api configuration", "api", err)
		}
	}
	d.SetId("")
	return nil
}
