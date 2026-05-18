package minio

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioPoolStatus() *schema.Resource {
	return &schema.Resource{
		Description: "Reports status information about the storage pools in a MinIO cluster.",
		ReadContext: dataSourceMinioPoolStatusRead,
		Schema: map[string]*schema.Schema{
			"pools": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of pools and their current status.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"index": {
							Type:        schema.TypeInt,
							Computed:    true,
							Description: "Pool index.",
						},
						"endpoint": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Pool endpoint command line string.",
						},
						"state": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Current state of the pool: `active`, `decommissioning`, `decommissioned`, `canceled`, or `failed`.",
						},
						"last_update": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "RFC3339 timestamp of the last status update.",
						},
						"decommission_info": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "JSON representation of decommission progress (empty if not decommissioning).",
						},
					},
				},
			},
		},
	}
}

func dataSourceMinioPoolStatusRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	log.Printf("[DEBUG] Reading pool status")

	pools, err := admin.ListPoolsStatus(ctx)
	if err != nil {
		return NewResourceError("reading pool status", "pools", err)
	}

	poolList := make([]map[string]interface{}, 0, len(pools))

	for _, p := range pools {
		state := "active"
		if p.Decommission != nil {
			if p.Decommission.Canceled {
				state = "canceled"
			} else if p.Decommission.Failed {
				state = "failed"
			} else if p.Decommission.Complete {
				state = "decommissioned"
			} else {
				state = "decommissioning"
			}
		}

		decommissionJSON := ""
		if p.Decommission != nil {
			data, err := json.Marshal(p.Decommission)
			if err != nil {
				return NewResourceError("marshaling decommission info", "pools", err)
			}
			decommissionJSON = string(data)
		}

		poolList = append(poolList, map[string]interface{}{
			"index":             p.ID,
			"endpoint":          p.CmdLine,
			"state":             state,
			"last_update":       p.LastUpdate.Format(time.RFC3339),
			"decommission_info": decommissionJSON,
		})
	}

	d.SetId("pool-status")

	if err := d.Set("pools", poolList); err != nil {
		return NewResourceError("setting pools", "pool-status", err)
	}

	return nil
}
