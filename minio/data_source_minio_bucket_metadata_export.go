package minio

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"io"
)

func dataSourceMinioBucketMetadataExport() *schema.Resource {
	return &schema.Resource{
		Description: "Exports a base64-encoded zip stream containing the metadata (policies, tagging, notification, ILM, etc.) for a single bucket. Use together with `minio_bucket_metadata_import` to copy metadata between buckets. Note: the full metadata zip is persisted in Terraform state; use a secure state backend.",
		ReadContext: dataSourceMinioBucketMetadataExportRead,
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringLenBetween(3, 63),
				Description:  "Name of the bucket to export metadata from.",
			},
			"metadata": {
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
				Description: "Base64-encoded zip stream of the bucket metadata export.",
			},
		},
	}
}

func dataSourceMinioBucketMetadataExportRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	bucket := d.Get("bucket").(string)

	tflog.Debug(ctx, fmt.Sprintf("Exporting metadata for bucket: %s", bucket))

	reader, err := admin.ExportBucketMetadata(ctx, bucket)
	if err != nil {
		return NewResourceError("exporting bucket metadata", bucket, err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		return NewResourceError("reading bucket metadata export", bucket, err)
	}

	encoded := base64.StdEncoding.EncodeToString(data)

	d.SetId(bucket)

	if err := d.Set("metadata", encoded); err != nil {
		return NewResourceError("setting metadata", bucket, err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Exported metadata for bucket: %s", bucket))

	return nil
}
