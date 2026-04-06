package minio

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceIAMUser() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieves information about a specific IAM user by name.",
		ReadContext: dataSourceIAMUserRead,
		Schema: map[string]*schema.Schema{
			"name":   {Type: schema.TypeString, Required: true},
			"status": {Type: schema.TypeString, Computed: true},
			"tags":   {Type: schema.TypeMap, Computed: true, Elem: &schema.Schema{Type: schema.TypeString}},
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

func dataSourceIAMUserRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	name := d.Get("name").(string)

	info, err := admin.GetUserInfo(ctx, name)
	if err != nil {
		return NewResourceError("reading IAM user", name, err)
	}

	d.SetId(name)
	_ = d.Set("name", name)
	_ = d.Set("status", strings.ToLower(string(info.Status)))

	var policies []string
	if info.PolicyName != "" {
		policies = strings.Split(info.PolicyName, ",")
	}
	_ = d.Set("policy_names", policies)
	_ = d.Set("member_of_groups", info.MemberOf)

	return nil
}
