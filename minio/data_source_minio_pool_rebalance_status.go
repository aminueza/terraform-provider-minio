package minio

import (
	"context"
	"encoding/json"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioPoolRebalanceStatus() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceMinioPoolRebalanceStatusRead,
		Schema: map[string]*schema.Schema{
			"status": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "JSON-marshalled rebalance status response.",
			},
			"pools": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Per-pool rebalance status details.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:        schema.TypeInt,
							Computed:    true,
							Description: "Zero-based pool index.",
						},
						"state": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Rebalance state for this pool (e.g. 'Active').",
						},
						"used": {
							Type:        schema.TypeFloat,
							Computed:    true,
							Description: "Percentage of used space in the pool.",
						},
						"progress": {
							Type:        schema.TypeList,
							Computed:    true,
							Description: "Progress metrics for the pool's rebalance operation.",
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"num_objects": {
										Type:        schema.TypeInt,
										Computed:    true,
										Description: "Number of objects rebalanced so far.",
									},
									"num_versions": {
										Type:        schema.TypeInt,
										Computed:    true,
										Description: "Number of object versions rebalanced so far.",
									},
									"bytes": {
										Type:        schema.TypeInt,
										Computed:    true,
										Description: "Number of bytes rebalanced so far.",
									},
									"bucket": {
										Type:        schema.TypeString,
										Computed:    true,
										Description: "Current bucket being rebalanced.",
									},
									"object": {
										Type:        schema.TypeString,
										Computed:    true,
										Description: "Current object being rebalanced.",
									},
									"elapsed": {
										Type:        schema.TypeString,
										Computed:    true,
										Description: "Elapsed duration of the rebalance operation.",
									},
									"eta": {
										Type:        schema.TypeString,
										Computed:    true,
										Description: "Estimated time remaining for the rebalance operation.",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func dataSourceMinioPoolRebalanceStatusRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	log.Printf("[DEBUG] Reading pool rebalance status")

	status, err := admin.RebalanceStatus(ctx)
	if err != nil {
		return NewResourceError("reading pool rebalance status", "pool_rebalance_status", err)
	}

	statusJSON, err := json.Marshal(status)
	if err != nil {
		return NewResourceError("marshalling rebalance status", "pool_rebalance_status", err)
	}

	if err := d.Set("status", string(statusJSON)); err != nil {
		return NewResourceError("setting status", "pool_rebalance_status", err)
	}

	pools := make([]map[string]interface{}, len(status.Pools))
	for i, p := range status.Pools {
		poolMap := map[string]interface{}{
			"id":    p.ID,
			"state": p.Status,
			"used":  p.Used,
		}

		objects, ok := SafeUint64ToInt64(p.Progress.NumObjects)
		if !ok {
			objects = int64(p.Progress.NumObjects)
		}
		versions, ok := SafeUint64ToInt64(p.Progress.NumVersions)
		if !ok {
			versions = int64(p.Progress.NumVersions)
		}
		bytes, ok := SafeUint64ToInt64(p.Progress.Bytes)
		if !ok {
			bytes = int64(p.Progress.Bytes)
		}

		poolMap["progress"] = []map[string]interface{}{{
			"num_objects":  objects,
			"num_versions": versions,
			"bytes":        bytes,
			"bucket":       p.Progress.Bucket,
			"object":       p.Progress.Object,
			"elapsed":      p.Progress.Elapsed.String(),
			"eta":          p.Progress.ETA.String(),
		}}

		pools[i] = poolMap
	}

	if err := d.Set("pools", pools); err != nil {
		return NewResourceError("setting pools", "pool_rebalance_status", err)
	}

	d.SetId(status.ID)
	if d.Id() == "" {
		d.SetId("pool_rebalance_status")
	}

	return nil
}
