package minio

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioPoolRebalance() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreatePoolRebalance,
		ReadContext:   minioReadPoolRebalance,
		DeleteContext: minioDeletePoolRebalance,
		Schema: map[string]*schema.Schema{
			"triggers": {
				Type:        schema.TypeMap,
				Optional:    true,
				ForceNew:    true,
				Description: "Map of arbitrary strings that, when changed, will force re-creation of the resource.",
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"started_at": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "RFC3339 timestamp when the rebalance operation was started.",
			},
			"status": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "JSON-marshalled rebalance status snapshot, or 'stopped' when no rebalance is in progress.",
			},
		},
	}
}

func minioCreatePoolRebalance(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	log.Printf("[DEBUG] Starting pool rebalance")

	rebalanceID, err := admin.RebalanceStart(ctx)
	if err != nil {
		return NewResourceError("starting pool rebalance", "pool", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if err := d.Set("started_at", now); err != nil {
		return NewResourceError("setting started_at", "pool", err)
	}

	d.SetId(rebalanceID)

	log.Printf("[DEBUG] Pool rebalance started with ID %s", rebalanceID)

	return minioReadPoolRebalance(ctx, d, meta)
}

func minioReadPoolRebalance(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	log.Printf("[DEBUG] Reading pool rebalance status for ID %s", d.Id())

	status, err := admin.RebalanceStatus(ctx)
	if err != nil {
		if isRebalanceNotFoundError(err) {
			if err := d.Set("status", "stopped"); err != nil {
				return NewResourceError("setting status", d.Id(), err)
			}
			return nil
		}
		return NewResourceError("reading pool rebalance status", d.Id(), err)
	}

	statusJSON, err := json.Marshal(status)
	if err != nil {
		return NewResourceError("marshalling rebalance status", d.Id(), err)
	}

	if err := d.Set("status", string(statusJSON)); err != nil {
		return NewResourceError("setting status", d.Id(), err)
	}

	return nil
}

func minioDeletePoolRebalance(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	log.Printf("[DEBUG] Stopping pool rebalance (ID: %s)", d.Id())

	err := admin.RebalanceStop(ctx)
	if err != nil {
		if isRebalanceNotFoundError(err) {
			log.Printf("[DEBUG] Rebalance already stopped (ID: %s)", d.Id())
			d.SetId("")
			return nil
		}
		return NewResourceError("stopping pool rebalance", d.Id(), err)
	}

	log.Printf("[DEBUG] Pool rebalance stopped (ID: %s)", d.Id())

	d.SetId("")
	return nil
}

func isRebalanceNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no rebalance") ||
		strings.Contains(msg, "not running") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "not started")
}
