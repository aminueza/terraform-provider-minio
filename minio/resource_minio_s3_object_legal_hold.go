package minio

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7"
)

func resourceMinioObjectLegalHold() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateObjectLegalHold,
		ReadContext:   minioReadObjectLegalHold,
		UpdateContext: minioUpdateObjectLegalHold,
		DeleteContext: minioDeleteObjectLegalHold,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Description:   "Manages legal hold status for S3 objects in a MinIO bucket. The bucket must have object locking enabled.",
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the bucket",
			},
			"key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Object key",
			},
			"version_id": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Version ID of the object",
			},
			"status": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringInSlice([]string{"ON", "OFF"}, false),
				Description:  "Legal hold status: ON or OFF",
			},
		},
	}
}

func minioCreateObjectLegalHold(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := ObjectLegalHoldConfig(d, meta)

	log.Printf("[DEBUG] Setting legal hold for object %s in bucket %s to %s", cfg.MinioObjectKey, cfg.MinioBucket, cfg.MinioStatus)

	status := minio.LegalHoldStatus(cfg.MinioStatus)
	opts := minio.PutObjectLegalHoldOptions{
		Status:    &status,
		VersionID: cfg.MinioVersionID,
	}

	if err := cfg.MinioClient.PutObjectLegalHold(ctx, cfg.MinioBucket, cfg.MinioObjectKey, opts); err != nil {
		return NewResourceError("creating object legal hold", fmt.Sprintf("%s/%s", cfg.MinioBucket, cfg.MinioObjectKey), err)
	}

	d.SetId(fmt.Sprintf("%s/%s", cfg.MinioBucket, cfg.MinioObjectKey))
	return minioReadObjectLegalHold(ctx, d, meta)
}

func minioReadObjectLegalHold(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)
	objectKey := d.Get("key").(string)

	if bucket == "" || objectKey == "" {
		bucket, objectKey = parseBucketAndKeyFromID(d.Id())
	}

	client := meta.(*S3MinioClient).S3Client
	versionID := d.Get("version_id").(string)

	opts := minio.GetObjectLegalHoldOptions{
		VersionID: versionID,
	}

	status, err := client.GetObjectLegalHold(ctx, bucket, objectKey, opts)
	if err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && (minioErr.Code == "NoSuchKey" || minioErr.Code == "NoSuchVersion") {
			log.Printf("[WARN] Object %s/%s not found, removing from state", bucket, objectKey)
			d.SetId("")
			return nil
		}
		return NewResourceError("reading object legal hold", fmt.Sprintf("%s/%s", bucket, objectKey), err)
	}

	resourceID := fmt.Sprintf("%s/%s", bucket, objectKey)

	if err := d.Set("bucket", bucket); err != nil {
		return NewResourceError("setting bucket", resourceID, err)
	}
	if err := d.Set("key", objectKey); err != nil {
		return NewResourceError("setting key", resourceID, err)
	}

	holdStatus := "OFF"
	if status != nil {
		holdStatus = string(*status)
	}
	if err := d.Set("status", holdStatus); err != nil {
		return NewResourceError("setting status", resourceID, err)
	}

	return nil
}

func minioUpdateObjectLegalHold(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if d.HasChange("status") {
		cfg := ObjectLegalHoldConfig(d, meta)

		log.Printf("[DEBUG] Updating legal hold for object %s in bucket %s to %s", cfg.MinioObjectKey, cfg.MinioBucket, cfg.MinioStatus)

		status := minio.LegalHoldStatus(cfg.MinioStatus)
		opts := minio.PutObjectLegalHoldOptions{
			Status:    &status,
			VersionID: cfg.MinioVersionID,
		}

		if err := cfg.MinioClient.PutObjectLegalHold(ctx, cfg.MinioBucket, cfg.MinioObjectKey, opts); err != nil {
			return NewResourceError("updating object legal hold", fmt.Sprintf("%s/%s", cfg.MinioBucket, cfg.MinioObjectKey), err)
		}
	}

	return minioReadObjectLegalHold(ctx, d, meta)
}

func minioDeleteObjectLegalHold(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)
	objectKey := d.Get("key").(string)

	if bucket == "" || objectKey == "" {
		bucket, objectKey = parseBucketAndKeyFromID(d.Id())
	}

	client := meta.(*S3MinioClient).S3Client
	versionID := d.Get("version_id").(string)

	log.Printf("[DEBUG] Removing legal hold for object %s in bucket %s", objectKey, bucket)

	status := minio.LegalHoldDisabled
	opts := minio.PutObjectLegalHoldOptions{
		Status:    &status,
		VersionID: versionID,
	}

	if err := client.PutObjectLegalHold(ctx, bucket, objectKey, opts); err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && minioErr.Code == "NoSuchKey" {
			return nil
		}
		return NewResourceError("deleting object legal hold", fmt.Sprintf("%s/%s", bucket, objectKey), err)
	}

	d.SetId("")
	return nil
}
