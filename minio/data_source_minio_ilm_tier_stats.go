package minio

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioILMTierStats() *schema.Resource {
	return &schema.Resource{
		Description: "Returns transition statistics for all configured ILM storage tiers including object counts and total bytes.",
		Read:        dataSourceMinioILMTierStatsRead,
		Schema: map[string]*schema.Schema{
			"tiers": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name":         {Type: schema.TypeString, Computed: true, Description: "Tier name."},
						"type":         {Type: schema.TypeString, Computed: true, Description: "Tier type (s3, gcs, azure, minio)."},
						"total_size":   {Type: schema.TypeString, Computed: true, Description: "Total bytes transitioned to this tier."},
						"num_objects":  {Type: schema.TypeInt, Computed: true, Description: "Number of objects on this tier."},
						"num_versions": {Type: schema.TypeInt, Computed: true, Description: "Number of object versions on this tier."},
					},
				},
			},
		},
	}
}

func dataSourceMinioILMTierStatsRead(d *schema.ResourceData, meta interface{}) error {
	admin := meta.(*S3MinioClient).S3Admin

	tierInfos, err := admin.TierStats(context.Background())
	if err != nil {
		return err
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

	var tiers []map[string]interface{}
	for _, t := range tierInfos {
		tiers = append(tiers, map[string]interface{}{
			"name":         t.Name,
			"type":         t.Type,
			"total_size":   strconv.FormatUint(t.Stats.TotalSize, 10),
			"num_objects":  t.Stats.NumObjects,
			"num_versions": t.Stats.NumVersions,
		})
	}

	_ = d.Set("tiers", tiers)
	return nil
}
