package minio

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

func dataSourceMinioAccountInfo() *schema.Resource {
	return &schema.Resource{
		Description: "Returns storage usage and bucket information for the authenticated account.",
		Read:        dataSourceMinioAccountInfoRead,
		Schema: map[string]*schema.Schema{
			"account_name":  {Type: schema.TypeString, Computed: true, Description: "Name of the authenticated account."},
			"bucket_count":  {Type: schema.TypeInt, Computed: true, Description: "Total number of buckets accessible to this account."},
			"total_size":    {Type: schema.TypeString, Computed: true, Description: "Total storage used across all buckets (bytes)."},
			"total_objects": {Type: schema.TypeString, Computed: true, Description: "Total object count across all buckets."},
			"buckets": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name":    {Type: schema.TypeString, Computed: true},
						"size":    {Type: schema.TypeString, Computed: true},
						"objects": {Type: schema.TypeString, Computed: true},
					},
				},
			},
		},
	}
}

func dataSourceMinioAccountInfoRead(d *schema.ResourceData, meta interface{}) error {
	admin := meta.(*S3MinioClient).S3Admin

	info, err := admin.AccountInfo(context.Background(), madmin.AccountOpts{})
	if err != nil {
		return err
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	_ = d.Set("account_name", info.AccountName)
	_ = d.Set("bucket_count", len(info.Buckets))

	var totalSize uint64
	var totalObjects uint64
	var buckets []map[string]interface{}
	for _, b := range info.Buckets {
		totalSize += b.Size
		totalObjects += b.Objects
		buckets = append(buckets, map[string]interface{}{
			"name":    b.Name,
			"size":    strconv.FormatUint(b.Size, 10),
			"objects": strconv.FormatUint(b.Objects, 10),
		})
	}

	_ = d.Set("total_size", strconv.FormatUint(totalSize, 10))
	_ = d.Set("total_objects", strconv.FormatUint(totalObjects, 10))
	_ = d.Set("buckets", buckets)

	return nil
}
