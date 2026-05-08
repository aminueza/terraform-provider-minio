package minio

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioConfigHistory() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceMinioConfigHistoryRead,
		Description: "Lists MinIO configuration change history. " +
			"Useful for auditing config changes and identifying restore points.",

		Schema: map[string]*schema.Schema{
			"limit": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     -1,
				Description: "Maximum number of history entries to return (-1 for all).",
			},
			"entries": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of configuration history entries.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"restore_id": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Unique identifier for this config restore point.",
						},
						"create_time": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Timestamp when this config was created.",
						},
						"data": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Configuration data at this restore point.",
						},
					},
				},
			},
		},
	}
}

func dataSourceMinioConfigHistoryRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	limit := d.Get("limit").(int)

	log.Printf("[DEBUG] Listing config history with limit: %d", limit)

	history, err := admin.ListConfigHistoryKV(ctx, limit)
	if err != nil {
		if strings.Contains(err.Error(), "admin' API is not supported") ||
			strings.Contains(err.Error(), "mode-server-xl") {
			log.Printf("[DEBUG] Config history API not available (Enterprise feature)")
			d.SetId("config_history")
			if setErr := d.Set("entries", []interface{}{}); setErr != nil {
				return NewResourceError("setting entries", "config_history", setErr)
			}
			return diag.Diagnostics{{
				Severity: diag.Warning,
				Summary:  "Config history API not available",
				Detail:   "The config history API is not supported on this MinIO server. This may be an Enterprise-only feature.",
			}}
		}
		return NewResourceError("listing config history", "config_history", err)
	}

	entries := make([]interface{}, 0, len(history))
	for _, entry := range history {
		entries = append(entries, map[string]interface{}{
			"restore_id":  entry.RestoreID,
			"create_time": entry.CreateTime.Format(time.RFC3339),
			"data":        entry.Data,
		})
	}

	if err := d.Set("entries", entries); err != nil {
		return NewResourceError("setting entries", "config_history", err)
	}

	d.SetId("config_history")
	log.Printf("[DEBUG] Listed %d config history entries", len(history))

	return nil
}
