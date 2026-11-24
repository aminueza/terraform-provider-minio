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
	if diags := setResourceField(d, "name", name, name); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "status", strings.ToLower(string(info.Status)), name); diags.HasError() {
		return diags
	}

	var policies []string
	if info.PolicyName != "" {
		policies = strings.Split(info.PolicyName, ",")
	}
	if diags := setResourceField(d, "policy_names", policies, name); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "member_of_groups", info.MemberOf, name); diags.HasError() {
		return diags
	}

	return nil
}
