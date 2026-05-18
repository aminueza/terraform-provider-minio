package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioPoolDecommission() *schema.Resource {
	return &schema.Resource{
		Description:   "Decommissions a storage pool from a MinIO cluster. WARNING: This is a destructive and irreversible operation. Once decommission completes, the pool cannot be recovered via Terraform. Ensure you have migrated all data before applying.",
		CreateContext: minioCreatePoolDecommission,
		ReadContext:   minioReadPoolDecommission,
		DeleteContext: minioDeletePoolDecommission,
		Importer: &schema.ResourceImporter{
			StateContext: minioImportPoolDecommission,
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
	admin := meta.(*S3MinioClient).S3Admin

	poolIndex := d.Get("pool_index").(int)

	pools, err := admin.ListPoolsStatus(ctx)
	if err != nil {
		return NewResourceError("listing pools", fmt.Sprintf("pool-%d", poolIndex), err)
	}

	poolEndpoint, err := findPoolEndpointByID(pools, poolIndex)
	if err != nil {
		return NewResourceError("finding pool endpoint", fmt.Sprintf("pool-%d", poolIndex), err)
	}

	log.Printf("[DEBUG] Starting decommission for pool index %d (endpoint: %s)", poolIndex, poolEndpoint)

	if err := admin.DecommissionPool(ctx, poolEndpoint); err != nil {
		return NewResourceError("starting decommission", fmt.Sprintf("pool-%d", poolIndex), err)
	}

	d.SetId(fmt.Sprintf("pool-%d", poolIndex))

	log.Printf("[DEBUG] Decommission started for pool index %d", poolIndex)

	return minioReadPoolDecommission(ctx, d, meta)
}

func minioReadPoolDecommission(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	poolIndex := d.Get("pool_index").(int)

	log.Printf("[DEBUG] Reading decommission status for pool index %d", poolIndex)

	pools, err := admin.ListPoolsStatus(ctx)
	if err != nil {
		return NewResourceError("listing pools for read", d.Id(), err)
	}

	poolEndpoint, err := findPoolEndpointByID(pools, poolIndex)
	if err != nil {
		log.Printf("[DEBUG] Pool index %d no longer found, marking decommission as complete", poolIndex)

		statusJSON, marshalErr := json.Marshal(map[string]string{"state": "complete"})
		if marshalErr != nil {
			return NewResourceError("marshaling complete status", d.Id(), marshalErr)
		}

		if err := d.Set("status", string(statusJSON)); err != nil {
			return NewResourceError("setting status", d.Id(), err)
		}

		return nil
	}

	poolStatus, err := admin.StatusPool(ctx, poolEndpoint)
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
		if err := d.Set("started_at", poolStatus.Decommission.StartTime.Format(time.RFC3339)); err != nil {
			return NewResourceError("setting started_at", d.Id(), err)
		}
	}

	return nil
}

func minioDeletePoolDecommission(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	poolIndex := d.Get("pool_index").(int)

	pools, err := admin.ListPoolsStatus(ctx)
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

	if err := admin.CancelDecommissionPool(ctx, poolEndpoint); err != nil {
		if isDecommissionCancelError(err) {
			log.Printf("[DEBUG] Decommission already complete or not in progress for pool %d: %v", poolIndex, err)
		} else {
			return NewResourceError("cancelling decommission", d.Id(), err)
		}
	}

	log.Printf("[DEBUG] Decommission cancelled for pool index %d", poolIndex)

	d.SetId("")

	return nil
}

func minioImportPoolDecommission(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	id := d.Id()

	if !strings.HasPrefix(id, "pool-") {
		return nil, fmt.Errorf("invalid import ID %q, expected format: pool-N (e.g., pool-0)", id)
	}

	poolIndexStr := strings.TrimPrefix(id, "pool-")

	poolIndex, err := strconv.Atoi(poolIndexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid pool index in import ID %q: %w", id, err)
	}

	if err := d.Set("pool_index", poolIndex); err != nil {
		return nil, err
	}

	return []*schema.ResourceData{d}, nil
}

func findPoolEndpointByID(pools []madmin.PoolStatus, poolIndex int) (string, error) {
	for _, p := range pools {
		if p.ID == poolIndex {
			return p.CmdLine, nil
		}
	}

	return "", fmt.Errorf("pool with index %d not found", poolIndex)
}

func isDecommissionCancelError(err error) bool {
	if err == nil {
		return false
	}
	if errResp := madmin.ToErrorResponse(err); errResp.Code != "" {
		switch errResp.Code {
		case "XMinioAdminDecommissionNotInProgress",
			"XMinioAdminDecommissionAlreadyComplete",
			"XMinioAdminDecommissionCanceled",
			"XMinioAdminPoolNotFound":
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not in progress") ||
		strings.Contains(msg, "already complete") ||
		strings.Contains(msg, "canceled") ||
		strings.Contains(msg, "cancelled") ||
		strings.Contains(msg, "not found")
}
