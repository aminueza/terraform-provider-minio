package minio

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioIAMExport() *schema.Resource {
	return &schema.Resource{
		Description: "Exports the entire IAM configuration (users, groups, policies, service accounts) from a MinIO server. " +
			"The payload is the raw zip MinIO returns, base64-encoded. Treat as sensitive: it can include access keys.",
		ReadContext: dataSourceMinioIAMExportRead,
		Schema: map[string]*schema.Schema{
			"iam_data": {
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
				Description: "Base64-encoded zip archive containing the exported IAM data. Pass directly to `minio_iam_import.iam_data` to restore on another cluster.",
			},
			"sha256": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "SHA-256 of the raw (pre-base64) export, useful as a change-detection signal across plans.",
			},
			"size_bytes": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "Size of the raw export in bytes.",
			},
		},
	}
}

func dataSourceMinioIAMExportRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	log.Printf("[DEBUG] Exporting IAM configuration")

	rc, err := admin.ExportIAM(ctx)
	if err != nil {
		return NewResourceError("exporting IAM", "iam", err)
	}

	raw, err := io.ReadAll(rc)
	if closeErr := rc.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return NewResourceError("reading IAM export stream", "iam", err)
	}

	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])

	d.SetId(digest)

	for _, field := range []struct {
		key   string
		value interface{}
	}{
		{"iam_data", base64.StdEncoding.EncodeToString(raw)},
		{"sha256", digest},
		{"size_bytes", len(raw)},
	} {
		if err := d.Set(field.key, field.value); err != nil {
			return NewResourceError("setting "+field.key, "iam", err)
		}
	}

	return nil
}
