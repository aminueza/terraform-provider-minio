package minio

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioBucketMetadataImport() *schema.Resource {
	return &schema.Resource{
		Description:   "Imports a base64-encoded zip stream of bucket metadata produced by `minio_bucket_metadata_export`. Note: destroying this resource only removes Terraform state; the imported metadata remains on the bucket.",
		CreateContext: minioCreateBucketMetadataImport,
		ReadContext:   minioReadBucketMetadataImport,
		DeleteContext: minioDeleteBucketMetadataImport,
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the bucket to import metadata into.",
			},
			"metadata": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "Base64-encoded zip stream of bucket metadata (from minio_bucket_metadata_export).",
			},
			"imported_at": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "RFC3339 timestamp of the successful import.",
			},
		},
	}
}

func minioCreateBucketMetadataImport(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	bucket := d.Get("bucket").(string)
	metadataRaw := d.Get("metadata").(string)

	log.Printf("[DEBUG] Importing metadata for bucket: %s", bucket)

	decoded, err := base64.StdEncoding.DecodeString(metadataRaw)
	if err != nil {
		return NewResourceError("decoding metadata", bucket, err)
	}

	reader := io.NopCloser(bytes.NewReader(decoded))

	result, err := admin.ImportBucketMetadata(ctx, bucket, reader)
	if err != nil {
		return NewResourceError("importing bucket metadata", bucket, err)
	}

	if diags := checkBucketMetaImportErrs(bucket, result); diags != nil {
		return diags
	}

	d.SetId(bucket)

	importedAt := time.Now().UTC().Format(time.RFC3339)

	if err := d.Set("imported_at", importedAt); err != nil {
		return NewResourceError("setting imported_at", bucket, err)
	}

	log.Printf("[DEBUG] Imported metadata for bucket: %s", bucket)

	return minioReadBucketMetadataImport(ctx, d, meta)
}

func checkBucketMetaImportErrs(bucket string, result madmin.BucketMetaImportErrs) diag.Diagnostics {
	bucketStatus, ok := result.Buckets[bucket]
	if !ok {
		return nil
	}

	var diags diag.Diagnostics

	if bucketStatus.Err != "" {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("importing bucket metadata for %q", bucket),
			Detail:   bucketStatus.Err,
		})
	}

	type metaField struct {
		name string
		ms   madmin.MetaStatus
	}
	fields := []metaField{
		{"object lock", bucketStatus.ObjectLock},
		{"versioning", bucketStatus.Versioning},
		{"policy", bucketStatus.Policy},
		{"tagging", bucketStatus.Tagging},
		{"SSE config", bucketStatus.SSEConfig},
		{"lifecycle", bucketStatus.Lifecycle},
		{"notification", bucketStatus.Notification},
		{"quota", bucketStatus.Quota},
		{"CORS", bucketStatus.Cors},
	}

	var warnings []string
	for _, f := range fields {
		if f.ms.IsSet && f.ms.Err != "" {
			warnings = append(warnings, fmt.Sprintf("%s: %s", f.name, f.ms.Err))
		}
	}

	if len(warnings) > 0 {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  fmt.Sprintf("partial import for bucket %q; some features failed", bucket),
			Detail:   "Failed features:\n" + strings.Join(warnings, "\n"),
		})
	}

	return diags
}

func minioReadBucketMetadataImport(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	log.Printf("[DEBUG] Reading bucket metadata import for bucket: %s", bucket)

	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return NewResourceError("checking bucket existence", bucket, err)
	}

	if !exists {
		log.Printf("[DEBUG] Bucket %s no longer exists, removing from state", bucket)
		d.SetId("")
		return nil
	}

	return nil
}

func minioDeleteBucketMetadataImport(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)

	log.Printf("[DEBUG] Removing bucket metadata import from state for bucket: %s", bucket)

	d.SetId("")

	return nil
}
