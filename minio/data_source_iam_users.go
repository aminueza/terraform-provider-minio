package minio

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// Data source: minio_iam_users — lists users with optional prefix & status filtering.
//
// status supports: "enabled", "disabled", "all" (default: "enabled").
func dataSourceIAMUsers() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceIAMUsersRead,
		Schema: map[string]*schema.Schema{
			// Inputs
			"name_prefix": {Type: schema.TypeString, Optional: true},
			"status": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "enabled",
				ValidateFunc: validation.StringInSlice([]string{"enabled", "disabled", "all"}, false),
			},

			// Outputs
			"users": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name":   {Type: schema.TypeString, Computed: true},
						"status": {Type: schema.TypeString, Computed: true},

						// Placeholders for future enrichment
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
				},
			},
		},
	}
}

func dataSourceIAMUsersRead(d *schema.ResourceData, meta interface{}) error {
	m := meta.(*S3MinioClient)
	admin := m.S3Admin

	usersMap, err := admin.ListUsers(context.Background())
	if err != nil {
		return err
	}

	prefix := strings.TrimSpace(d.Get("name_prefix").(string))
	wantStatus := strings.ToLower(strings.TrimSpace(d.Get("status").(string))) // enabled|disabled|all

	var out []map[string]interface{}
	for name, ui := range usersMap {
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		status := strings.ToLower(string(ui.Status))

		switch wantStatus {
		case "all":
			// keep
		case "enabled":
			if status != "enabled" {
				continue
			}
		case "disabled":
			if status != "disabled" {
				continue
			}
		default:
			// Should never happen thanks to ValidateFunc.
			continue
		}

		out = append(out, map[string]interface{}{
			"name":             name,
			"status":           status,
			"policy_names":     []string{},
			"member_of_groups": []string{},
		})
	}

	// A volatile ID to force refresh when re-reading during a plan/apply.
	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	_ = d.Set("users", out)
	return nil
}