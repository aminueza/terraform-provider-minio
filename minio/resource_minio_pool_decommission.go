package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

const (
	poolStateDecommissioning = "decommissioning"
	poolStateDecommissioned  = "decommissioned"
	poolStateCanceled        = "canceled"
	poolStateFailed          = "failed"
)

func resourceMinioPoolDecommission() *schema.Resource {
	return &schema.Resource{
		Description: "Decommissions a storage pool from a MinIO cluster. WARNING: this is a destructive and irreversible operation — once decommission completes, MinIO removes the pool and the data on it cannot be recovered via Terraform. Ensure all data has been migrated before applying.\n\n" +
			"Create returns as soon as MinIO accepts the request; the decommission then runs asynchronously on the server. Refresh the resource (or read the `minio_pool_status` data source) to observe progress in the `state` and `status` attributes.\n\n" +
			"Destroying the resource cancels an in-progress decommission (no-op once the pool has already been removed).",
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
				Description: "Pool ID to decommission, as reported by the `index` attribute of the `minio_pool_status` data source.",
			},
			"started_at": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "RFC3339 timestamp when decommission started. Empty when imported after the pool has already been removed.",
			},
			"state": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Current state of the decommission: `decommissioning`, `decommissioned`, `canceled`, or `failed`.",
			},
			"status": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "JSON-marshalled MinIO `PoolDecommissionInfo` snapshot, or an empty string when the pool has already been removed from the cluster.",
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

	tflog.Debug(ctx, fmt.Sprintf("Starting decommission for pool index %d (endpoint: %s)", poolIndex, poolEndpoint))

	if err := admin.DecommissionPool(ctx, poolEndpoint); err != nil {
		return NewResourceError("starting decommission", fmt.Sprintf("pool-%d", poolIndex), err)
	}

	d.SetId(fmt.Sprintf("pool-%d", poolIndex))

	now := time.Now().UTC().Format(time.RFC3339)
	if err := d.Set("started_at", now); err != nil {
		return NewResourceError("setting started_at", d.Id(), err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Decommission started for pool index %d at %s", poolIndex, now))

	return minioReadPoolDecommission(ctx, d, meta)
}

func minioReadPoolDecommission(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	poolIndex := d.Get("pool_index").(int)

	tflog.Debug(ctx, fmt.Sprintf("Reading decommission status for pool index %d", poolIndex))

	pools, err := admin.ListPoolsStatus(ctx)
	if err != nil {
		return NewResourceError("listing pools for read", d.Id(), err)
	}

	poolEndpoint, err := findPoolEndpointByID(pools, poolIndex)
	if err != nil {
		tflog.Debug(ctx, fmt.Sprintf("Pool index %d no longer found, marking decommission as complete", poolIndex))

		if err := d.Set("state", poolStateDecommissioned); err != nil {
			return NewResourceError("setting state", d.Id(), err)
		}
		if err := d.Set("status", ""); err != nil {
			return NewResourceError("setting status", d.Id(), err)
		}

		return nil
	}

	poolStatus, err := admin.StatusPool(ctx, poolEndpoint)
	if err != nil {
		return NewResourceError("reading pool status", d.Id(), err)
	}

	if err := d.Set("state", decommissionState(poolStatus.Decommission)); err != nil {
		return NewResourceError("setting state", d.Id(), err)
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
		tflog.Debug(ctx, fmt.Sprintf("Could not list pools during delete: %v", err))
		d.SetId("")

		return nil
	}

	poolEndpoint, err := findPoolEndpointByID(pools, poolIndex)
	if err != nil {
		tflog.Debug(ctx, fmt.Sprintf("Pool index %d not found during delete, treating as gone", poolIndex))
		d.SetId("")

		return nil
	}

	tflog.Debug(ctx, fmt.Sprintf("Cancelling decommission for pool index %d (endpoint: %s)", poolIndex, poolEndpoint))

	if err := admin.CancelDecommissionPool(ctx, poolEndpoint); err != nil {
		if isDecommissionCancelError(err) {
			tflog.Debug(ctx, fmt.Sprintf("Decommission already complete or not in progress for pool %d: %v", poolIndex, err))
		} else {
			return NewResourceError("cancelling decommission", d.Id(), err)
		}
	}

	tflog.Debug(ctx, fmt.Sprintf("Decommission cancelled for pool index %d", poolIndex))

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

func decommissionState(info *madmin.PoolDecommissionInfo) string {
	if info == nil {
		return poolStateDecommissioning
	}
	switch {
	case info.Canceled:
		return poolStateCanceled
	case info.Failed:
		return poolStateFailed
	case info.Complete:
		return poolStateDecommissioned
	default:
		return poolStateDecommissioning
	}
}

func findPoolEndpointByID(pools []madmin.PoolStatus, poolIndex int) (string, error) {
	for _, p := range pools {
		if p.ID == poolIndex {
			return p.CmdLine, nil
		}
	}

	return "", fmt.Errorf("pool with index %d not found", poolIndex)
}

// isDecommissionCancelError reports whether err means there's nothing for
// CancelDecommissionPool to act on (decommission already finished, was already
// cancelled, or the pool was removed). MinIO server returns these as plain
// errors without stable admin codes, so substring matching is the most
// resilient option here.
func isDecommissionCancelError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not in progress") ||
		strings.Contains(msg, "already complete") ||
		strings.Contains(msg, "canceled") ||
		strings.Contains(msg, "cancelled") ||
		strings.Contains(msg, "pool not found")
}
