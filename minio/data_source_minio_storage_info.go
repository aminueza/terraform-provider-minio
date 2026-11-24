package minio

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioStorageInfo() *schema.Resource {
	return &schema.Resource{
		Description: "Returns disk and drive status for all MinIO server nodes. Essential for capacity planning and health monitoring.",
		ReadContext: dataSourceMinioStorageInfoRead,
		Schema: map[string]*schema.Schema{
			"disk_count":      {Type: schema.TypeInt, Computed: true, Description: "Total number of disks."},
			"online_disks":    {Type: schema.TypeInt, Computed: true, Description: "Number of disks online."},
			"offline_disks":   {Type: schema.TypeInt, Computed: true, Description: "Number of disks offline."},
			"total_space":     {Type: schema.TypeString, Computed: true, Description: "Total storage capacity (bytes)."},
			"used_space":      {Type: schema.TypeString, Computed: true, Description: "Used storage (bytes)."},
			"available_space": {Type: schema.TypeString, Computed: true, Description: "Available storage (bytes)."},
			"disks": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"endpoint":        {Type: schema.TypeString, Computed: true},
						"path":            {Type: schema.TypeString, Computed: true},
						"state":           {Type: schema.TypeString, Computed: true},
						"total_space":     {Type: schema.TypeString, Computed: true},
						"used_space":      {Type: schema.TypeString, Computed: true},
						"available_space": {Type: schema.TypeString, Computed: true},
						"healing":         {Type: schema.TypeBool, Computed: true},
						"scanning":        {Type: schema.TypeBool, Computed: true},
					},
				},
			},
		},
	}
}

func dataSourceMinioStorageInfoRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	info, err := admin.StorageInfo(ctx)
	if err != nil {
		return NewResourceError("reading storage info", "storage_info", err)
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

	var totalSpace, usedSpace, availSpace uint64
	var online, offline int
	var disks []map[string]interface{}

	for _, disk := range info.Disks {
		totalSpace += disk.TotalSpace
		usedSpace += disk.UsedSpace
		availSpace += disk.AvailableSpace
		if disk.State == "ok" {
			online++
		} else {
			offline++
		}
		disks = append(disks, map[string]interface{}{
			"endpoint":        disk.Endpoint,
			"path":            disk.DrivePath,
			"state":           disk.State,
			"total_space":     strconv.FormatUint(disk.TotalSpace, 10),
			"used_space":      strconv.FormatUint(disk.UsedSpace, 10),
			"available_space": strconv.FormatUint(disk.AvailableSpace, 10),
			"healing":         disk.Healing,
			"scanning":        disk.Scanning,
		})
	}

	if diags := setResourceField(d, "disk_count", len(info.Disks), "storage_info"); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "online_disks", online, "storage_info"); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "offline_disks", offline, "storage_info"); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "total_space", strconv.FormatUint(totalSpace, 10), "storage_info"); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "used_space", strconv.FormatUint(usedSpace, 10), "storage_info"); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "available_space", strconv.FormatUint(availSpace, 10), "storage_info"); diags.HasError() {
		return diags
	}
	return setResourceField(d, "disks", disks, "storage_info")
}
