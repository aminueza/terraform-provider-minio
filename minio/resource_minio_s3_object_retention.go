package minio

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7"
)

func resourceMinioObjectRetention() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages retention policy for individual S3 objects. The bucket must have object locking enabled.",
		CreateContext: minioCreateObjectRetention,
		ReadContext:   minioReadObjectRetention,
		UpdateContext: minioUpdateObjectRetention,
		DeleteContext: minioDeleteObjectRetention,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the bucket.",
			},
			"key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Object key.",
			},
			"version_id": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Version ID of the object.",
			},
			"mode": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringInSlice([]string{"GOVERNANCE", "COMPLIANCE"}, false),
				Description:  "Retention mode: GOVERNANCE or COMPLIANCE.",
			},
			"retain_until_date": {
				Type:             schema.TypeString,
				Required:         true,
				Description:      "Date until which the object is retained (RFC3339 format).",
				ValidateFunc:     validation.IsRFC3339Time,
				DiffSuppressFunc: suppressTimeDiffs,
			},
			"governance_bypass": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Allow bypassing governance mode retention.",
			},
		},
	}
}

func minioCreateObjectRetention(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client

	bucket := d.Get("bucket").(string)
	key := d.Get("key").(string)
	versionID := d.Get("version_id").(string)
	modeStr := d.Get("mode").(string)
	retainUntil := d.Get("retain_until_date").(string)
	bypass := d.Get("governance_bypass").(bool)

	mode := minio.RetentionMode(modeStr)
	t, err := time.Parse(time.RFC3339, retainUntil)
	if err != nil {
		return NewResourceError("parsing retain_until_date", key, err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Setting retention for %s/%s: mode=%s until=%s", bucket, key, modeStr, retainUntil))

	opts := minio.PutObjectRetentionOptions{
		GovernanceBypass: bypass,
		Mode:             &mode,
		RetainUntilDate:  &t,
		VersionID:        versionID,
	}

	if err := client.PutObjectRetention(ctx, bucket, key, opts); err != nil {
		return NewResourceError("setting object retention", fmt.Sprintf("%s/%s", bucket, key), s3CompatWriteError(meta.(*S3MinioClient), "object retention", err))
	}

	d.SetId(objectVersionID(bucket, key, versionID))
	return minioReadObjectRetention(ctx, d, meta)
}

func minioReadObjectRetention(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)
	key := d.Get("key").(string)
	versionID := d.Get("version_id").(string)

	parsedFromID := bucket == "" || key == ""
	if parsedFromID {
		var err error
		bucket, key, versionID, err = parseObjectVersionID(d.Id())
		if err != nil {
			return NewResourceError("parsing object retention import ID", d.Id(), err)
		}
	}

	client := meta.(*S3MinioClient).S3Client

	mode, retainUntil, err := client.GetObjectRetention(ctx, bucket, key, versionID)
	if err != nil {
		if s3CompatReadUnsupported(ctx, meta.(*S3MinioClient), s3Compat405IsAnAnswer, d, "object retention", err) {
			return nil
		}
		var minioErr minio.ErrorResponse
		// NoSuchObjectLockConfiguration is the server saying this object has no
		// retention, which for this resource means it is gone from state.
		if errors.As(err, &minioErr) && (objectLockReadGone(minioErr) || minioErr.Code == "NoSuchObjectLockConfiguration") {
			tflog.Warn(ctx, fmt.Sprintf("Object %s/%s has no retention, removing from state", bucket, key))
			d.SetId("")
			return nil
		}
		return NewResourceError("reading object retention", fmt.Sprintf("%s/%s", bucket, key), objectVersionImportHint(err, parsedFromID))
	}

	resourceID := d.Id()
	if resourceID == "" {
		resourceID = fmt.Sprintf("%s/%s", bucket, key)
	}
	fields := []struct {
		key string
		val interface{}
	}{
		{"bucket", bucket},
		{"key", key},
	}
	if versionID != "" {
		fields = append(fields, struct {
			key string
			val interface{}
		}{"version_id", versionID})
	}
	if mode != nil {
		fields = append(fields, struct {
			key string
			val interface{}
		}{"mode", string(*mode)})
	}
	if retainUntil != nil {
		fields = append(fields, struct {
			key string
			val interface{}
		}{"retain_until_date", retainUntil.Format(time.RFC3339)})
	}
	for _, f := range fields {
		if diags := setResourceField(d, f.key, f.val, resourceID); diags != nil {
			return diags
		}
	}

	return nil
}

func minioUpdateObjectRetention(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if d.HasChanges("mode", "retain_until_date") {
		return minioCreateObjectRetention(ctx, d, meta)
	}
	return minioReadObjectRetention(ctx, d, meta)
}

func minioDeleteObjectRetention(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	d.SetId("")
	return nil
}
