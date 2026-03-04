package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioServerConfigStorageClass() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages MinIO storage class configuration for erasure coding parity. Controls data protection levels for STANDARD and REDUCED_REDUNDANCY storage classes.",
		CreateContext: minioServerConfigStorageClassSet,
		ReadContext:   minioServerConfigStorageClassRead,
		UpdateContext: minioServerConfigStorageClassSet,
		DeleteContext: minioServerConfigStorageClassDelete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"standard": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Parity for STANDARD storage class (e.g., \"EC:4\" for 4 parity drives).",
			},
			"rrs": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Parity for REDUCED_REDUNDANCY storage class (e.g., \"EC:2\").",
			},
			"comment": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"restart_required": {
				Type:     schema.TypeBool,
				Computed: true,
			},
		},
	}
}

func minioServerConfigStorageClassSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	var parts []string
	for _, f := range []string{"standard", "rrs", "comment"} {
		if v, ok := d.GetOk(f); ok {
			parts = append(parts, fmt.Sprintf("%s=%s", f, v.(string)))
		}
	}

	if len(parts) == 0 {
		d.SetId("storage_class")
		return minioServerConfigStorageClassRead(ctx, d, meta)
	}

	configString := "storage_class " + strings.Join(parts, " ")
	restart, err := admin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("setting storage_class configuration", "storage_class", err)
	}

	d.SetId("storage_class")
	_ = d.Set("restart_required", restart)
	log.Printf("[DEBUG] Set storage_class config (restart_required=%v)", restart)

	return minioServerConfigStorageClassRead(ctx, d, meta)
}

func minioServerConfigStorageClassRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	configData, err := admin.GetConfigKV(ctx, "storage_class")
	if err != nil {
		return NewResourceError("reading storage_class configuration", "storage_class", err)
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, "storage_class ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	d.SetId("storage_class")

	for _, f := range []string{"standard", "rrs", "comment"} {
		if v, ok := cfgMap[f]; ok && v != "" {
			_ = d.Set(f, v)
		}
	}

	return nil
}

func minioServerConfigStorageClassDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	_, err := admin.DelConfigKV(ctx, "storage_class")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			return NewResourceError("resetting storage_class configuration", "storage_class", err)
		}
	}
	d.SetId("")
	return nil
}
