package minio

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Data source: minio_iam_user — reads one existing user by name.
func dataSourceIAMUser() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceIAMUserRead,
		Schema: map[string]*schema.Schema{
			// Input
			"name": {Type: schema.TypeString, Required: true},

			// Outputs
			"status": {Type: schema.TypeString, Computed: true},

			// Placeholders for future enrichment (policies & groups).
			"policy_names": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"member_of_groups": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func dataSourceIAMUserRead(d *schema.ResourceData, meta interface{}) error {
	m := meta.(*S3MinioClient)
	admin := m.S3Admin

	name := d.Get("name").(string)
	info, err := admin.GetUserInfo(context.Background(), name)
	if err != nil {
		return err
	}

	d.SetId(name)
	_ = d.Set("status", strings.ToLower(string(info.Status)))
	_ = d.Set("policy_names", []string{})
	_ = d.Set("member_of_groups", []string{})

	return nil
}