package minio

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioILMTiers() *schema.Resource {
	return &schema.Resource{
		Description: "Lists all configured ILM remote storage tiers.",
		Read:        dataSourceMinioILMTiersRead,
		Schema: map[string]*schema.Schema{
			"tiers": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name":     {Type: schema.TypeString, Computed: true},
						"type":     {Type: schema.TypeString, Computed: true},
						"bucket":   {Type: schema.TypeString, Computed: true},
						"endpoint": {Type: schema.TypeString, Computed: true},
						"region":   {Type: schema.TypeString, Computed: true},
						"prefix":   {Type: schema.TypeString, Computed: true},
					},
				},
			},
		},
	}
}

func dataSourceMinioILMTiersRead(d *schema.ResourceData, meta interface{}) error {
	admin := meta.(*S3MinioClient).S3Admin

	tiers, err := admin.ListTiers(context.Background())
	if err != nil {
		return err
	}

	var out []map[string]interface{}
	for _, t := range tiers {
		out = append(out, map[string]interface{}{
			"name":     t.Name,
			"type":     t.Type.String(),
			"bucket":   t.Bucket(),
			"endpoint": t.Endpoint(),
			"region":   t.Region(),
			"prefix":   t.Prefix(),
		})
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	_ = d.Set("tiers", out)
	return nil
}
