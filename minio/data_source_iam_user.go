package minio

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceIAMUser() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieves information about a specific IAM user by name.",
		Read:        dataSourceIAMUserRead,
		Schema: map[string]*schema.Schema{
			"name":   {Type: schema.TypeString, Required: true},
			"status": {Type: schema.TypeString, Computed: true},
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
