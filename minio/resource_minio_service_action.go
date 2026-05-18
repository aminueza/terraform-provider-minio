package minio

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceMinioServiceAction() *schema.Resource {
	return &schema.Resource{
		Description: "Performs a one-shot MinIO service control operation (restart, stop, freeze, or unfreeze). This resource is not stateful — taking it down does not undo the action.",

		CreateContext: minioCreateServiceAction,
		ReadContext:   minioReadServiceAction,
		DeleteContext: minioDeleteServiceAction,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"action": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice([]string{"freeze", "unfreeze", "restart", "stop"}, false),
				Description:  "The service action to perform. One of: freeze, unfreeze, restart, stop.",
			},
			"triggers": {
				Type:        schema.TypeMap,
				Optional:    true,
				ForceNew:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Arbitrary map of strings to force re-execution when changed (like the terraform_data pattern).",
			},
			"executed_at": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "RFC3339 timestamp of when the action was executed.",
			},
			"result": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Short text summary of what was done.",
			},
		},
	}
}

func minioCreateServiceAction(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	action := d.Get("action").(string)

	admin := meta.(*S3MinioClient).S3Admin

	log.Printf("[DEBUG] Creating service action: %s", action)

	var err error
	var result string

	switch action {
	case "restart":
		err = admin.ServiceRestartV2(ctx)
		result = "MinIO cluster restarted"
	case "stop":
		err = admin.ServiceStopV2(ctx)
		result = "MinIO cluster stopped"
	case "freeze":
		err = admin.ServiceFreezeV2(ctx)
		result = "MinIO cluster frozen (all S3 API calls suspended)"
	case "unfreeze":
		err = admin.ServiceUnfreezeV2(ctx)
		result = "MinIO cluster unfrozen (S3 API calls resumed)"
	}

	if err != nil {
		return NewResourceError("executing service action", action, err)
	}

	executedAt := time.Now().UTC().Format(time.RFC3339)
	id := fmt.Sprintf("%s-%s", action, executedAt)

	d.SetId(id)
	_ = d.Set("executed_at", executedAt)
	_ = d.Set("result", result)
	_ = d.Set("action", action)

	log.Printf("[DEBUG] Service action %s completed at %s", action, executedAt)

	return nil
}

func minioReadServiceAction(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] Reading service action (no-op): %s", d.Id())
	return nil
}

func minioDeleteServiceAction(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] Deleting service action (state-only, no API call): %s", d.Id())
	d.SetId("")
	return nil
}
