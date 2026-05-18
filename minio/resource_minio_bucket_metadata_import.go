package minio

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioBucketMetadataImport() *schema.Resource {
	return &schema.Resource{
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
				DiffSuppressFunc: func(k, oldVal, newVal string, d *schema.ResourceData) bool {
					// After the import has succeeded once (d.Id() is set), suppress any
					// drift in the input bytes — re-exports of the same bucket are not
					// byte-identical (zip ordering / timestamps) but the imported
					// metadata is unchanged on the server.
					return d.Id() != ""
				},
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

	_, err = admin.ImportBucketMetadata(ctx, bucket, reader)
	if err != nil {
		return NewResourceError("importing bucket metadata", bucket, err)
	}

	d.SetId(bucket)

	importedAt := time.Now().UTC().Format(time.RFC3339)

	if err := d.Set("imported_at", importedAt); err != nil {
		return NewResourceError("setting imported_at", bucket, err)
	}

	log.Printf("[DEBUG] Imported metadata for bucket: %s", bucket)

	return minioReadBucketMetadataImport(ctx, d, meta)
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
