package minio

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioAccessKey() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateAccessKey,
		ReadContext:   minioReadAccessKey,
		UpdateContext: minioUpdateAccessKey,
		DeleteContext: minioDeleteAccessKey,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"user": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The user for whom the access key is managed.",
			},
			"access_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "The access key.",
			},
			"secret_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				Description: "The secret key.",
			},
			"status": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "enabled",
				Description: "The status of the access key (enabled/disabled).",
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					status := val.(string)
					if status != "enabled" && status != "disabled" {
						errs = append(errs, fmt.Errorf("%q must be either 'enabled' or 'disabled', got: %s", key, status))
					}
					return
				},
			},
		},
	}
}

func minioCreateAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	user := d.Get("user").(string)
	accessKey := d.Get("access_key").(string)
	secretKey := d.Get("secret_key").(string)
	status := d.Get("status").(string)

	log.Printf("[INFO] Creating accesskey for user %s", user)

	req := madmin.AddServiceAccountReq{
		SecretKey:  secretKey,
		AccessKey:  accessKey,
		TargetUser: user,
	}

	creds, err := client.S3Admin.AddServiceAccount(ctx, req)
	if err != nil {
		return diag.Errorf("failed to create accesskey: %s", err)
	}

	d.SetId(aws.StringValue(&creds.AccessKey))
	_ = d.Set("access_key", creds.AccessKey)
	_ = d.Set("secret_key", creds.SecretKey)

	if status == "disabled" {
		return minioUpdateAccessKey(ctx, d, meta)
	}

	return minioReadAccessKey(ctx, d, meta)
}

func minioReadAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	accessKeyID := d.Id()

	log.Printf("[INFO] Reading accesskey %s", accessKeyID)

	info, err := client.S3Admin.InfoServiceAccount(ctx, accessKeyID)
	if err != nil {
		log.Printf("[WARN] Failed to read accesskey %s: %s", accessKeyID, err)
		d.SetId("")
		return diag.Errorf("failed to read accesskey: %s", err)
	}

	parentUser := info.ParentUser
	_ = d.Set("user", parentUser)

	var status string
	if info.AccountStatus == "on" {
		status = "enabled"
	} else {
		status = "disabled"
	}
	_ = d.Set("status", status)

	// Set access_key to the ID for importing
	// Secret key can't be retrieved from the API, so it remains unset during import
	_ = d.Set("access_key", accessKeyID)

	return nil
}

func minioUpdateAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	accessKeyID := d.Id()
	status := d.Get("status").(string)

	log.Printf("[INFO] Updating accesskey %s with status %s", accessKeyID, status)

	newStatus := "on"
	if status == "disabled" {
		newStatus = "off"
	}

	if err := client.S3Admin.UpdateServiceAccount(ctx, accessKeyID, madmin.UpdateServiceAccountReq{NewStatus: newStatus}); err != nil {
		return diag.Errorf("failed to update accesskey status: %s", err)
	}

	return minioReadAccessKey(ctx, d, meta)
}

func minioDeleteAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	accessKeyID := d.Id()

	log.Printf("[INFO] Deleting accesskey %s", accessKeyID)

	if err := client.S3Admin.DeleteServiceAccount(ctx, accessKeyID); err != nil {
		return diag.Errorf("failed to delete accesskey: %s", err)
	}

	d.SetId("")
	return nil
}
