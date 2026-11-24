package minio

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func dataSourceIAMUsers() *schema.Resource {
	return &schema.Resource{
		Description: "Lists IAM users with optional filtering by name prefix and status.",
		ReadContext: dataSourceIAMUsersRead,
		Schema: map[string]*schema.Schema{
			"name_prefix": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Optional prefix filter matching user names.",
			},
			"status": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "enabled",
				Description:  "Filter users by account status. Valid values are `enabled`, `disabled`, or `all`. Defaults to `enabled`.",
				ValidateFunc: validation.StringInSlice([]string{"enabled", "disabled", "all"}, false),
			},
			"users": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "IAM users matching the filters, sorted by name.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "User name (or distinguished name for LDAP users that have a direct policy mapping).",
						},
						"status": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Account status (`enabled` or `disabled`).",
						},
						"policy_names": {
							Type:        schema.TypeList,
							Computed:    true,
							Description: "Names of canned policies attached directly to the user.",
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
						"member_of_groups": {
							Type:        schema.TypeList,
							Computed:    true,
							Description: "Names of IAM groups the user belongs to.",
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
					},
				},
			},
		},
	}
}

func splitIAMPolicyNames(raw string) []string {
	parts := strings.Split(raw, ",")
	policies := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			policies = append(policies, trimmed)
		}
	}
	return policies
}

func dataSourceIAMUsersRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	m := meta.(*S3MinioClient)
	admin := m.S3Admin

	prefix := strings.TrimSpace(d.Get("name_prefix").(string))
	wantStatus := strings.ToLower(strings.TrimSpace(d.Get("status").(string)))
	if wantStatus == "" {
		wantStatus = "enabled"
	}
	filterID := fmt.Sprintf("iam-users:%s:%s", wantStatus, prefix)

	usersMap, err := admin.ListUsers(ctx)
	if err != nil {
		return NewResourceError("listing IAM users", filterID, err)
	}

	matchingNames := make([]string, 0, len(usersMap))
	for name, ui := range usersMap {
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		status := strings.ToLower(string(ui.Status))

		switch wantStatus {
		case "all":
		case "enabled":
			if status != "enabled" {
				continue
			}
		case "disabled":
			if status != "disabled" {
				continue
			}
		default:
			continue
		}

		matchingNames = append(matchingNames, name)
	}
	slices.Sort(matchingNames)

	out := make([]map[string]interface{}, 0, len(matchingNames))
	for _, name := range matchingNames {
		ui := usersMap[name]
		groups := slices.Clone(ui.MemberOf)
		if groups == nil {
			groups = []string{}
		}
		out = append(out, map[string]interface{}{
			"name":             name,
			"status":           strings.ToLower(string(ui.Status)),
			"policy_names":     splitIAMPolicyNames(ui.PolicyName),
			"member_of_groups": groups,
		})
	}

	d.SetId(filterID)
	if err := d.Set("users", out); err != nil {
		return NewResourceError("setting users", filterID, err)
	}
	return nil
}
