package minio

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioDataUsage() *schema.Resource {
	return &schema.Resource{
		Description: "Returns cluster-wide data usage statistics including total objects, size, and per-bucket breakdown.",
		Read:        dataSourceMinioDataUsageRead,
		Schema: map[string]*schema.Schema{
			"last_update":    {Type: schema.TypeString, Computed: true, Description: "Timestamp of last usage data update."},
			"total_objects":  {Type: schema.TypeString, Computed: true, Description: "Total object count across all buckets."},
			"total_size":     {Type: schema.TypeString, Computed: true, Description: "Total storage used (bytes)."},
			"buckets_count":  {Type: schema.TypeInt, Computed: true, Description: "Number of buckets."},
			"buckets_usage": {
				Type:     schema.TypeMap,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Description: "Per-bucket storage usage in bytes.",
			},
		},
	}
}

func dataSourceMinioDataUsageRead(d *schema.ResourceData, meta interface{}) error {
	admin := meta.(*S3MinioClient).S3Admin

	info, err := admin.DataUsageInfo(context.Background())
	if err != nil {
		return err
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	_ = d.Set("last_update", info.LastUpdate.Format(time.RFC3339))
	_ = d.Set("total_objects", strconv.FormatUint(info.ObjectsTotalCount, 10))
	_ = d.Set("total_size", strconv.FormatUint(info.ObjectsTotalSize, 10))
	_ = d.Set("buckets_count", len(info.BucketsUsage))

	bucketsUsage := make(map[string]string)
	for name, usage := range info.BucketsUsage {
		bucketsUsage[name] = strconv.FormatUint(usage.Size, 10)
	}
	_ = d.Set("buckets_usage", bucketsUsage)

	return nil
}
