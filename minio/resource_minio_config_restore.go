package minio

import (
	"context"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioConfigRestore() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateConfigRestore,
		ReadContext:   minioReadConfigRestore,
		DeleteContext: minioDeleteConfigRestore,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Description: "Restores MinIO configuration from a previous point in history. " +
			"Use with minio_config_history data source to identify restore points.",

		Schema: map[string]*schema.Schema{
			"restore_id": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The config history restore ID to restore.",
			},
			"data": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The configuration data that was restored.",
			},
		},
	}
}

func minioCreateConfigRestore(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	restoreID := d.Get("restore_id").(string)

	log.Printf("[DEBUG] Restoring config from history ID: %s", restoreID)

	err := admin.RestoreConfigHistoryKV(ctx, restoreID)
	if err != nil {
		if strings.Contains(err.Error(), "config file not found") ||
			strings.Contains(err.Error(), "no config history") {
			log.Printf("[DEBUG] Config history not available (Enterprise feature or no history exists)")
			d.SetId(restoreID)
			return minioReadConfigRestore(ctx, d, meta)
		}
		return NewResourceError("restoring config", restoreID, err)
	}

	d.SetId(restoreID)

	log.Printf("[DEBUG] Restored config from: %s", restoreID)
	return minioReadConfigRestore(ctx, d, meta)
}

func minioReadConfigRestore(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	restoreID := d.Id()

	log.Printf("[DEBUG] Reading config restore: %s", restoreID)

	if restoreID == "" {
		return nil
	}

	if err := d.Set("restore_id", restoreID); err != nil {
		return NewResourceError("setting restore_id", restoreID, err)
	}

	return nil
}

func minioDeleteConfigRestore(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] Deleting config restore resource: %s", d.Id())
	d.SetId("")
	return nil
}