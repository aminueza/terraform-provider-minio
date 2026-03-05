package minio

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceIAMGroups() *schema.Resource {
	return &schema.Resource{
		Description: "Lists all IAM groups with optional name prefix filtering.",
		Read:        dataSourceIAMGroupsRead,
		Schema: map[string]*schema.Schema{
			"name_prefix": {Type: schema.TypeString, Optional: true},
			"groups": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name":   {Type: schema.TypeString, Computed: true},
						"status": {Type: schema.TypeString, Computed: true},
						"policy": {Type: schema.TypeString, Computed: true},
						"members": {
							Type:     schema.TypeList,
							Computed: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
			},
		},
	}
}

func dataSourceIAMGroupsRead(d *schema.ResourceData, meta interface{}) error {
	admin := meta.(*S3MinioClient).S3Admin

	groupNames, err := admin.ListGroups(context.Background())
	if err != nil {
		return err
	}

	prefix := strings.TrimSpace(d.Get("name_prefix").(string))

	var out []map[string]interface{}
	for _, name := range groupNames {
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}

		desc, err := admin.GetGroupDescription(context.Background(), name)
		if err != nil {
			continue
		}

		out = append(out, map[string]interface{}{
			"name":    name,
			"status":  strings.ToLower(desc.Status),
			"policy":  desc.Policy,
			"members": desc.Members,
		})
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	_ = d.Set("groups", out)
	return nil
}
