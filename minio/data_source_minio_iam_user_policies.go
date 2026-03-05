package minio

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceIAMUserPolicies() *schema.Resource {
	return &schema.Resource{
		Description: "Returns all IAM policies effective for a user, including policies attached directly and inherited from group membership.",
		Read:        dataSourceIAMUserPoliciesRead,
		Schema: map[string]*schema.Schema{
			"name": {Type: schema.TypeString, Required: true},
			"direct_policies": {
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Policies attached directly to the user.",
			},
			"group_policies": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Policies inherited from group membership.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"group":  {Type: schema.TypeString, Computed: true},
						"policy": {Type: schema.TypeString, Computed: true},
					},
				},
			},
			"all_policies": {
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Deduplicated list of all effective policy names.",
			},
		},
	}
}

func dataSourceIAMUserPoliciesRead(d *schema.ResourceData, meta interface{}) error {
	admin := meta.(*S3MinioClient).S3Admin
	ctx := context.Background()

	name := d.Get("name").(string)
	info, err := admin.GetUserInfo(ctx, name)
	if err != nil {
		return err
	}

	d.SetId(name)

	var directPolicies []string
	if info.PolicyName != "" {
		for _, p := range strings.Split(info.PolicyName, ",") {
			directPolicies = append(directPolicies, strings.TrimSpace(p))
		}
	}
	_ = d.Set("direct_policies", directPolicies)

	seen := map[string]bool{}
	for _, p := range directPolicies {
		seen[p] = true
	}

	var groupPolicies []map[string]interface{}
	for _, groupName := range info.MemberOf {
		desc, err := admin.GetGroupDescription(ctx, groupName)
		if err != nil {
			continue
		}
		if desc.Policy != "" {
			for _, p := range strings.Split(desc.Policy, ",") {
				p = strings.TrimSpace(p)
				groupPolicies = append(groupPolicies, map[string]interface{}{
					"group":  groupName,
					"policy": p,
				})
				seen[p] = true
			}
		}
	}
	_ = d.Set("group_policies", groupPolicies)

	var allPolicies []string
	for p := range seen {
		allPolicies = append(allPolicies, p)
	}
	_ = d.Set("all_policies", allPolicies)

	return nil
}
