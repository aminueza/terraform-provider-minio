package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioPoolDecommission() *schema.Resource {
	return &schema.Resource{
		Description:   "Decommissions a storage pool from a MinIO cluster.",
		CreateContext: minioCreatePoolDecommission,
		ReadContext:   minioReadPoolDecommission,
		DeleteContext: minioDeletePoolDecommission,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"pool_index": {
				Type:        schema.TypeInt,
				Required:    true,
				ForceNew:    true,
				Description: "Index of the pool to decommission.",
			},
			"started_at": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "RFC3339 timestamp when decommission started.",
			},
			"status": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "JSON representation of the decommission status.",
			},
		},
	}
}

func minioCreatePoolDecommission(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	poolConfig := PoolDecommissionConfig(d, meta)

	poolIndex := poolConfig.PoolIndex

	pools, err := poolConfig.MinioAdmin.ListPoolsStatus(ctx)
	if err != nil {
		return NewResourceError("listing pools", fmt.Sprintf("pool-%d", poolIndex), err)
	}

	poolEndpoint, err := findPoolEndpointByID(pools, poolIndex)
	if err != nil {
		return NewResourceError("finding pool endpoint", fmt.Sprintf("pool-%d", poolIndex), err)
	}

	log.Printf("[DEBUG] Starting decommission for pool index %d (endpoint: %s)", poolIndex, poolEndpoint)

	if err := poolConfig.MinioAdmin.DecommissionPool(ctx, poolEndpoint); err != nil {
		return NewResourceError("starting decommission", fmt.Sprintf("pool-%d", poolIndex), err)
	}

	d.SetId(fmt.Sprintf("pool-%d", poolIndex))

	log.Printf("[DEBUG] Decommission started for pool index %d", poolIndex)

	return minioReadPoolDecommission(ctx, d, meta)
}

func minioReadPoolDecommission(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	poolConfig := PoolDecommissionConfig(d, meta)

	poolIndex := poolConfig.PoolIndex

	pools, err := poolConfig.MinioAdmin.ListPoolsStatus(ctx)
	if err != nil {
		return NewResourceError("listing pools for read", d.Id(), err)
	}

	poolEndpoint, err := findPoolEndpointByID(pools, poolIndex)
	if err != nil {
		log.Printf("[DEBUG] Pool index %d no longer found, marking decommission as complete", poolIndex)

		if err := d.Set("status", "complete"); err != nil {
			return NewResourceError("setting status", d.Id(), err)
		}

		return nil
	}

	poolStatus, err := poolConfig.MinioAdmin.StatusPool(ctx, poolEndpoint)
	if err != nil {
		return NewResourceError("reading pool status", d.Id(), err)
	}

	statusJSON, err := json.Marshal(poolStatus.Decommission)
	if err != nil {
		return NewResourceError("marshaling decommission status", d.Id(), err)
	}

	if err := d.Set("status", string(statusJSON)); err != nil {
		return NewResourceError("setting status", d.Id(), err)
	}

	if poolStatus.Decommission != nil {
		if err := d.Set("started_at", poolStatus.Decommission.StartTime.Format("2006-01-02T15:04:05Z07:00")); err != nil {
			return NewResourceError("setting started_at", d.Id(), err)
		}
	}

	return nil
}

func minioDeletePoolDecommission(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	poolConfig := PoolDecommissionConfig(d, meta)

	poolIndex := poolConfig.PoolIndex

	pools, err := poolConfig.MinioAdmin.ListPoolsStatus(ctx)
	if err != nil {
		log.Printf("[DEBUG] Could not list pools during delete: %v", err)
		d.SetId("")

		return nil
	}

	poolEndpoint, err := findPoolEndpointByID(pools, poolIndex)
	if err != nil {
		log.Printf("[DEBUG] Pool index %d not found during delete, treating as gone", poolIndex)
		d.SetId("")

		return nil
	}

	log.Printf("[DEBUG] Cancelling decommission for pool index %d (endpoint: %s)", poolIndex, poolEndpoint)

	if err := poolConfig.MinioAdmin.CancelDecommissionPool(ctx, poolEndpoint); err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "not in progress") ||
			strings.Contains(errStr, "already complete") ||
			strings.Contains(errStr, "canceled") ||
			strings.Contains(errStr, "not found") {
			log.Printf("[DEBUG] Decommission already complete or not in progress for pool %d: %v", poolIndex, err)
		} else {
			log.Printf("%s", NewResourceErrorStr("cancelling decommission", d.Id(), err))

			return NewResourceError("cancelling decommission", d.Id(), err)
		}
	}

	log.Printf("[DEBUG] Decommission cancelled for pool index %d", poolIndex)

	d.SetId("")

	return nil
}

func findPoolEndpointByID(pools []madmin.PoolStatus, poolIndex int) (string, error) {
	for _, p := range pools {
		if p.ID == poolIndex {
			return p.CmdLine, nil
		}
	}

	return "", fmt.Errorf("pool with index %d not found", poolIndex)
}
