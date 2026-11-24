package minio

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioDataUsage() *schema.Resource {
	return &schema.Resource{
		Description: "Returns cluster-wide data usage statistics including total objects, size, and per-bucket breakdown.",
		ReadContext: dataSourceMinioDataUsageRead,
		Schema: map[string]*schema.Schema{
			"last_update":   {Type: schema.TypeString, Computed: true, Description: "Timestamp of last usage data update."},
			"total_objects": {Type: schema.TypeString, Computed: true, Description: "Total object count across all buckets."},
			"total_size":    {Type: schema.TypeString, Computed: true, Description: "Total storage used (bytes)."},
			"buckets_count": {Type: schema.TypeInt, Computed: true, Description: "Number of buckets."},
			"buckets_usage": {
				Type:        schema.TypeMap,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Per-bucket storage usage in bytes.",
			},
		},
	}
}

func dataSourceMinioDataUsageRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	info, err := admin.DataUsageInfo(ctx)
	if err != nil {
		return NewResourceError("reading data usage", "data_usage", err)
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	if diags := setResourceField(d, "last_update", info.LastUpdate.Format(time.RFC3339), "data_usage"); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "total_objects", strconv.FormatUint(info.ObjectsTotalCount, 10), "data_usage"); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "total_size", strconv.FormatUint(info.ObjectsTotalSize, 10), "data_usage"); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "buckets_count", len(info.BucketsUsage), "data_usage"); diags.HasError() {
		return diags
	}

	bucketsUsage := make(map[string]string)
	for name, usage := range info.BucketsUsage {
		bucketsUsage[name] = strconv.FormatUint(usage.Size, 10)
	}
	if diags := setResourceField(d, "buckets_usage", bucketsUsage, "data_usage"); diags.HasError() {
		return diags
	}

	return nil
}
