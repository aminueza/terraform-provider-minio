package minio

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"io"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioBucketMetadataImport() *schema.Resource {
	return &schema.Resource{
		Description:   "Imports a base64-encoded zip stream of bucket metadata produced by `minio_bucket_metadata_export`. Destroying this resource only removes Terraform state; the imported metadata remains on the bucket. Use the `triggers` map to force a re-import.",
		CreateContext: minioCreateBucketMetadataImport,
		ReadContext:   minioReadBucketMetadataImport,
		UpdateContext: minioReadBucketMetadataImport,
		DeleteContext: minioDeleteBucketMetadataImport,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(3, 63),
				Description:  "Name of the bucket to import metadata into.",
			},
			"metadata": {
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
				Description: "Base64-encoded zip stream of bucket metadata (from `minio_bucket_metadata_export`). Bytes are not compared after the initial import because re-exports of the same bucket are not byte-identical (zip timestamps); use `triggers` to force a re-import.",
				DiffSuppressFunc: func(k, oldVal, newVal string, d *schema.ResourceData) bool {
					return d.Id() != ""
				},
			},
			"triggers": {
				Type:        schema.TypeMap,
				Optional:    true,
				ForceNew:    true,
				Description: "Map of arbitrary strings that, when changed, force re-import of the metadata.",
				Elem:        &schema.Schema{Type: schema.TypeString},
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

	tflog.Debug(ctx, fmt.Sprintf("Importing metadata for bucket: %s", bucket))

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

	tflog.Debug(ctx, fmt.Sprintf("Imported metadata for bucket: %s", bucket))

	return minioReadBucketMetadataImport(ctx, d, meta)
}

func checkBucketMetaImportErrs(bucket string, result madmin.BucketMetaImportErrs) diag.Diagnostics {
	bucketStatus, ok := result.Buckets[bucket]
	if !ok {
		// Server may key the response by the bucket name inside the zip
		// rather than the request target. If there's exactly one entry,
		// use it; otherwise aggregate.
		switch len(result.Buckets) {
		case 0:
			return nil
		case 1:
			for _, v := range result.Buckets {
				bucketStatus = v
			}
		default:
			var diags diag.Diagnostics
			for k, v := range result.Buckets {
				diags = append(diags, statusDiagnostics(k, v)...)
			}
			return diags
		}
	}

	return statusDiagnostics(bucket, bucketStatus)
}

func statusDiagnostics(bucket string, bucketStatus madmin.BucketStatus) diag.Diagnostics {
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

	tflog.Debug(ctx, fmt.Sprintf("Reading bucket metadata import for bucket: %s", bucket))

	// Read intentionally does not refresh metadata content: the source zip
	// is non-deterministic and madmin exposes no per-feature read that
	// reconstructs the same bytes. Drift on the bucket is detected by the
	// dedicated resources (policy, tagging, lifecycle, ...) instead.
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return NewResourceError("checking bucket existence", bucket, err)
	}

	if !exists {
		tflog.Debug(ctx, fmt.Sprintf("Bucket %s no longer exists, removing from state", bucket))
		d.SetId("")
		return nil
	}

	return nil
}

func minioDeleteBucketMetadataImport(_ context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)

	log.Printf("[DEBUG] Removing bucket metadata import from state for bucket: %s", bucket)

	d.SetId("")

	return nil
}
