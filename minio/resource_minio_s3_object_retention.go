package minio

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
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
				Type:         schema.TypeString,
				Required:     true,
				Description:  "Date until which the object is retained (RFC3339 format).",
				ValidateFunc: validation.IsRFC3339Time,
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

func retentionID(bucket, key, versionID string) string {
	id := fmt.Sprintf("%s/%s", bucket, key)
	if versionID != "" {
		id += "#" + versionID
	}
	return id
}

func parseRetentionID(id string) (bucket, key, versionID string) {
	rest := id
	if idx := strings.LastIndex(id, "#"); idx != -1 {
		rest = id[:idx]
		versionID = id[idx+1:]
	}
	bucket, key = parseBucketAndKeyFromID(rest)
	return bucket, key, versionID
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

	log.Printf("[DEBUG] Setting retention for %s/%s: mode=%s until=%s", bucket, key, modeStr, retainUntil)

	opts := minio.PutObjectRetentionOptions{
		GovernanceBypass: bypass,
		Mode:             &mode,
		RetainUntilDate:  &t,
		VersionID:        versionID,
	}

	if err := client.PutObjectRetention(ctx, bucket, key, opts); err != nil {
		return NewResourceError("setting object retention", fmt.Sprintf("%s/%s", bucket, key), err)
	}

	d.SetId(retentionID(bucket, key, versionID))
	return minioReadObjectRetention(ctx, d, meta)
}

func minioReadObjectRetention(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)
	key := d.Get("key").(string)
	versionID := d.Get("version_id").(string)

	if bucket == "" || key == "" {
		bucket, key, versionID = parseRetentionID(d.Id())
	}

	client := meta.(*S3MinioClient).S3Client

	mode, retainUntil, err := client.GetObjectRetention(ctx, bucket, key, versionID)
	if err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && (minioErr.Code == "NoSuchKey" || minioErr.Code == "NoSuchObjectLockConfiguration") {
			d.SetId("")
			return nil
		}
		return NewResourceError("reading object retention", fmt.Sprintf("%s/%s", bucket, key), err)
	}

	_ = d.Set("bucket", bucket)
	_ = d.Set("key", key)
	if versionID != "" {
		_ = d.Set("version_id", versionID)
	}
	if mode != nil {
		_ = d.Set("mode", string(*mode))
	}
	if retainUntil != nil {
		_ = d.Set("retain_until_date", retainUntil.Format(time.RFC3339))
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
