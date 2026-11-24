package minio

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3Buckets() *schema.Resource {
	return &schema.Resource{
		Description: "Lists all S3 buckets with optional name prefix filtering.",
		ReadContext: dataSourceMinioS3BucketsRead,
		Schema: map[string]*schema.Schema{
			"name_prefix": {Type: schema.TypeString, Optional: true},
			"buckets": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name":          {Type: schema.TypeString, Computed: true},
						"creation_date": {Type: schema.TypeString, Computed: true},
					},
				},
			},
		},
	}
}

func dataSourceMinioS3BucketsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client

	prefix := strings.TrimSpace(d.Get("name_prefix").(string))
	resource := "s3_buckets"
	if prefix != "" {
		resource = prefix
	}
	buckets, err := client.ListBuckets(ctx)
	if err != nil {
		return NewResourceError("listing S3 buckets", resource, err)
	}

	var out []map[string]interface{}
	for _, b := range buckets {
		if prefix != "" && !strings.HasPrefix(b.Name, prefix) {
			continue
		}
		out = append(out, map[string]interface{}{
			"name":          b.Name,
			"creation_date": b.CreationDate.Format(time.RFC3339),
		})
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	return setResourceField(d, "buckets", out, resource)
}
