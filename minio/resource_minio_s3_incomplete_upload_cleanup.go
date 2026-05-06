package minio

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceMinioS3IncompleteUploadCleanup() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateIncompleteUploadCleanup,
		ReadContext:   minioReadIncompleteUploadCleanup,
		UpdateContext: minioUpdateIncompleteUploadCleanup,
		DeleteContext: minioDeleteIncompleteUploadCleanup,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Description: `Cleans up incomplete/stuck multipart uploads in a MinIO bucket. ` +
			`Uses ListIncompleteUploads and RemoveIncompleteUpload to find and abort ` +
			`multipart uploads that were never completed.`,

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(1, 63),
				Description:  "Name of the bucket to clean up incomplete uploads from.",
			},
			"prefix": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Object prefix to filter incomplete uploads (default: all objects).",
			},
			"last_cleanup": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Timestamp of the last cleanup operation.",
			},
		},
	}
}

func minioCreateIncompleteUploadCleanup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := IncompleteUploadCleanupConfig(d, meta)

	log.Printf("[DEBUG] Creating incomplete upload cleanup for bucket: %s", config.MinioBucket)

	if err := cleanupIncompleteUploads(ctx, config); err != nil {
		return NewResourceError("cleaning up incomplete uploads", config.MinioBucket, err)
	}

	id := config.MinioBucket
	if config.MinioPrefix != "" {
		id = config.MinioBucket + "/" + config.MinioPrefix
	}
	d.SetId(id)

	if err := d.Set("last_cleanup", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return NewResourceError("setting last_cleanup", d.Id(), err)
	}

	log.Printf("[DEBUG] Created incomplete upload cleanup for: %s", d.Id())
	return minioReadIncompleteUploadCleanup(ctx, d, meta)
}

func minioReadIncompleteUploadCleanup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := IncompleteUploadCleanupConfig(d, meta)

	log.Printf("[DEBUG] Reading incomplete upload cleanup for: %s", d.Id())

	exists, err := config.MinioClient.BucketExists(ctx, config.MinioBucket)
	if err != nil {
		return NewResourceError("checking bucket existence", config.MinioBucket, err)
	}
	if !exists {
		log.Printf("[WARN] Bucket %s not found, removing from state", config.MinioBucket)
		d.SetId("")
		return nil
	}

	if err := d.Set("bucket", config.MinioBucket); err != nil {
		return NewResourceError("setting bucket", d.Id(), err)
	}
	if err := d.Set("prefix", config.MinioPrefix); err != nil {
		return NewResourceError("setting prefix", d.Id(), err)
	}

	return nil
}

func minioUpdateIncompleteUploadCleanup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := IncompleteUploadCleanupConfig(d, meta)

	log.Printf("[DEBUG] Updating incomplete upload cleanup for: %s", config.MinioBucket)

	if err := cleanupIncompleteUploads(ctx, config); err != nil {
		return NewResourceError("cleaning up incomplete uploads", config.MinioBucket, err)
	}

	if err := d.Set("last_cleanup", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return NewResourceError("setting last_cleanup", d.Id(), err)
	}

	log.Printf("[DEBUG] Updated incomplete upload cleanup for: %s", config.MinioBucket)
	return minioReadIncompleteUploadCleanup(ctx, d, meta)
}

func minioDeleteIncompleteUploadCleanup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] Deleting incomplete upload cleanup for: %s", d.Id())
	d.SetId("")
	return nil
}

func cleanupIncompleteUploads(ctx context.Context, config *S3MinioIncompleteUploadCleanup) error {
	log.Printf("[DEBUG] Listing incomplete uploads for bucket: %s, prefix: %s", config.MinioBucket, config.MinioPrefix)

	incompleteCh := config.MinioClient.ListIncompleteUploads(ctx, config.MinioBucket, config.MinioPrefix, true)

	var cleanupErrors []string
	cleanedCount := 0

	for obj := range incompleteCh {
		if obj.Err != nil {
			return obj.Err
		}

		log.Printf("[DEBUG] Removing incomplete upload: %s (UploadID: %s)", obj.Key, obj.UploadID)

		err := config.MinioClient.RemoveIncompleteUpload(ctx, config.MinioBucket, obj.Key)
		if err != nil {
			cleanupErrors = append(cleanupErrors, obj.Key+": "+err.Error())
			continue
		}
		cleanedCount++
	}

	if len(cleanupErrors) > 0 {
		return fmt.Errorf("errors during cleanup: %s", strings.Join(cleanupErrors, "; "))
	}

	log.Printf("[DEBUG] Cleaned up %d incomplete uploads in bucket %s", cleanedCount, config.MinioBucket)
	return nil
}
