package minio

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioKMSKeys() *schema.Resource {
	return &schema.Resource{
		Description: "Lists the keys held by the KMS the MinIO server is configured with. " +
			"Use it to discover which key IDs exist before referring to one in " +
			"`minio_s3_bucket_server_side_encryption`, or to assert that a key a configuration " +
			"depends on is present.",
		ReadContext: dataSourceMinioKMSKeysRead,
		Schema: map[string]*schema.Schema{
			"pattern": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "*",
				Description: "Glob pattern a key name must match to be listed. Defaults to `*`, which lists every key the caller may see.",
			},
			"keys": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Keys the KMS reports, in the order the server returns them.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Key name, which is the `key_id` a `minio_kms_key` resource uses.",
						},
						"created_at": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Time the key was created, in RFC 3339 format. Empty when the KMS backend does not report it.",
						},
						"created_by": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Identity that created the key. Empty when the KMS backend does not report it.",
						},
					},
				},
			},
		},
	}
}

func dataSourceMinioKMSKeysRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	pattern := d.Get("pattern").(string)
	if pattern == "" {
		pattern = "*"
	}

	tflog.Debug(ctx, fmt.Sprintf("Listing KMS keys matching [%s]", pattern))

	keys, err := admin.ListKeys(ctx, pattern)
	if err != nil {
		return NewResourceError("listing KMS keys", pattern, err)
	}

	listed := make([]map[string]interface{}, 0, len(keys))
	for _, key := range keys {
		createdAt := ""
		if !key.CreatedAt.IsZero() {
			createdAt = key.CreatedAt.UTC().Format(time.RFC3339)
		}
		listed = append(listed, map[string]interface{}{
			"name":       key.Name,
			"created_at": createdAt,
			"created_by": key.CreatedBy,
		})
	}

	tflog.Debug(ctx, fmt.Sprintf("KMS reported %d keys matching [%s]", len(listed), pattern))

	d.SetId(pattern)

	if err := d.Set("keys", listed); err != nil {
		return NewResourceError("setting keys", pattern, err)
	}

	return nil
}
