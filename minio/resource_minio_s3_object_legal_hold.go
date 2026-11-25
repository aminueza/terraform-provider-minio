package minio

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"

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

	tflog.Debug(ctx, fmt.Sprintf("Setting legal hold for object %s in bucket %s to %s", cfg.MinioObjectKey, cfg.MinioBucket, cfg.MinioStatus))

	status := minio.LegalHoldStatus(cfg.MinioStatus)
	opts := minio.PutObjectLegalHoldOptions{
		Status:    &status,
		VersionID: cfg.MinioVersionID,
	}

	if err := cfg.MinioClient.PutObjectLegalHold(ctx, cfg.MinioBucket, cfg.MinioObjectKey, opts); err != nil {
		return NewResourceError("creating object legal hold", fmt.Sprintf("%s/%s", cfg.MinioBucket, cfg.MinioObjectKey), s3CompatWriteError(meta.(*S3MinioClient), "object legal hold", err))
	}

	d.SetId(objectVersionID(cfg.MinioBucket, cfg.MinioObjectKey, cfg.MinioVersionID))
	return minioReadObjectLegalHold(ctx, d, meta)
}

func minioReadObjectLegalHold(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)
	objectKey := d.Get("key").(string)
	versionID := d.Get("version_id").(string)

	// On import, attributes are empty — parse them from the ID.
	parsedFromID := bucket == "" || objectKey == ""
	if parsedFromID {
		var err error
		bucket, objectKey, versionID, err = parseObjectVersionID(d.Id())
		if err != nil {
			return NewResourceError("parsing object legal hold import ID", d.Id(), err)
		}
	}

	client := meta.(*S3MinioClient).S3Client

	opts := minio.GetObjectLegalHoldOptions{
		VersionID: versionID,
	}

	status, err := client.GetObjectLegalHold(ctx, bucket, objectKey, opts)
	if err != nil {
		if s3CompatReadUnsupported(ctx, meta.(*S3MinioClient), s3Compat405IsAnAnswer, d, "object legal hold", err) {
			return nil
		}
		var minioErr minio.ErrorResponse
		if !errors.As(err, &minioErr) {
			return NewResourceError("reading object legal hold", fmt.Sprintf("%s/%s", bucket, objectKey), objectVersionImportHint(err, parsedFromID))
		}
		if objectLockReadGone(minioErr) {
			tflog.Warn(ctx, fmt.Sprintf("Object legal hold %s/%s can no longer be read (%s: %s), removing from state", bucket, objectKey, minioErr.Code, minioErr.Message))
			d.SetId("")
			return nil
		}
		if minioErr.Code != "NoSuchObjectLockConfiguration" {
			return NewResourceError("reading object legal hold", fmt.Sprintf("%s/%s", bucket, objectKey), objectVersionImportHint(err, parsedFromID))
		}
		// MinIO answers NoSuchObjectLockConfiguration ("The specified object
		// does not have a ObjectLock configuration") for an object in a locked
		// bucket that never had a legal hold. The effective status here is OFF,
		// not a failed read, so an import of that object records the status
		// instead of erroring. The bucket-without-object-lock case arrives above
		// and drops the resource; see objectLockReadGone and issue #1413.
		status = nil
	}

	if err := d.Set("bucket", bucket); err != nil {
		return NewResourceError("setting bucket", d.Id(), err)
	}
	if err := d.Set("key", objectKey); err != nil {
		return NewResourceError("setting key", d.Id(), err)
	}
	if versionID != "" {
		if err := d.Set("version_id", versionID); err != nil {
			return NewResourceError("setting version_id", d.Id(), err)
		}
	}

	holdStatus := "OFF"
	if status != nil {
		holdStatus = string(*status)
	}
	if err := d.Set("status", holdStatus); err != nil {
		return NewResourceError("setting status", d.Id(), err)
	}

	return nil
}

func minioUpdateObjectLegalHold(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if d.HasChange("status") {
		cfg := ObjectLegalHoldConfig(d, meta)

		tflog.Debug(ctx, fmt.Sprintf("Updating legal hold for object %s in bucket %s to %s", cfg.MinioObjectKey, cfg.MinioBucket, cfg.MinioStatus))

		status := minio.LegalHoldStatus(cfg.MinioStatus)
		opts := minio.PutObjectLegalHoldOptions{
			Status:    &status,
			VersionID: cfg.MinioVersionID,
		}

		if err := cfg.MinioClient.PutObjectLegalHold(ctx, cfg.MinioBucket, cfg.MinioObjectKey, opts); err != nil {
			return NewResourceError("updating object legal hold", fmt.Sprintf("%s/%s", cfg.MinioBucket, cfg.MinioObjectKey), s3CompatWriteError(meta.(*S3MinioClient), "object legal hold", err))
		}
	}

	return minioReadObjectLegalHold(ctx, d, meta)
}

func minioDeleteObjectLegalHold(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)
	objectKey := d.Get("key").(string)
	versionID := d.Get("version_id").(string)

	if bucket == "" || objectKey == "" {
		var err error
		bucket, objectKey, versionID, err = parseObjectVersionID(d.Id())
		if err != nil {
			return NewResourceError("parsing object legal hold import ID", d.Id(), err)
		}
	}

	client := meta.(*S3MinioClient).S3Client

	tflog.Debug(ctx, fmt.Sprintf("Removing legal hold for object %s in bucket %s", objectKey, bucket))

	status := minio.LegalHoldDisabled
	opts := minio.PutObjectLegalHoldOptions{
		Status:    &status,
		VersionID: versionID,
	}

	if err := client.PutObjectLegalHold(ctx, bucket, objectKey, opts); err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && objectLockReadGone(minioErr) {
			return nil
		}
		return NewResourceError("deleting object legal hold", fmt.Sprintf("%s/%s", bucket, objectKey), err)
	}

	d.SetId("")
	return nil
}
