package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
)

func resourceMinioIAMUserGroupMembership() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateUserGroupMembership,
		ReadContext:   minioReadUserGroupMembership,
		UpdateContext: minioUpdateUserGroupMembership,
		DeleteContext: minioDeleteUserGroupMembership,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"user": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The name of the IAM user",
			},
			"groups": {
				Type:        schema.TypeSet,
				Required:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "A list of IAM groups to add the user to",
			},
		},
	}
}

type IAMUserGroupMembershipConfig struct {
	MinioAdmin *madmin.AdminClient
	UserName   string
	Groups     []string
}

func iamUserGroupMembershipConfig(d *schema.ResourceData, meta interface{}) *IAMUserGroupMembershipConfig {
	m := meta.(*S3MinioClient)

	groups := []string{}
	if v, ok := d.GetOk("groups"); ok {
		for _, g := range v.(*schema.Set).List() {
			groups = append(groups, g.(string))
		}
	}

	userName := d.Get("user").(string)
	if userName == "" {
		userName = d.Id()
	}
	return &IAMUserGroupMembershipConfig{
		MinioAdmin: m.S3Admin,
		UserName:   userName,
		Groups:     groups,
	}
}

func minioCreateUserGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := iamUserGroupMembershipConfig(d, meta)

	for _, grp := range cfg.Groups {
		err := cfg.MinioAdmin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
			Group:   grp,
			Members: []string{cfg.UserName},
		})
		if err != nil {
			return NewResourceError("adding user to group", cfg.UserName, err)
		}
	}

	d.SetId(cfg.UserName)

	desired := make(map[string]struct{})
	for _, g := range cfg.Groups {
		desired[g] = struct{}{}
	}
	userInfo, err := cfg.MinioAdmin.GetUserInfo(ctx, cfg.UserName)
	if err != nil {
		return NewResourceError("reading user info for reconciliation", cfg.UserName, err)
	}
	current := make(map[string]struct{})
	for _, g := range userInfo.MemberOf {
		current[g] = struct{}{}
	}
	for grp := range current {
		if _, ok := desired[grp]; !ok {
			if err := cfg.MinioAdmin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
				Group:    grp,
				Members:  []string{cfg.UserName},
				IsRemove: true,
			}); err != nil {
				return NewResourceError("removing user from extra group", cfg.UserName, err)
			}
		}
	}

	return minioReadUserGroupMembership(ctx, d, meta)
}

func minioReadUserGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := iamUserGroupMembershipConfig(d, meta)

	userInfo, err := cfg.MinioAdmin.GetUserInfo(ctx, cfg.UserName)
	if err != nil {
		return NewResourceError("reading user info", cfg.UserName, err)
	}

	// Ensure 'user' attribute is set in state (required for import)
	if _, ok := d.GetOk("user"); !ok {
		if err := d.Set("user", cfg.UserName); err != nil {
			return NewResourceError("setting user attribute", cfg.UserName, err)
		}
	}

	if err := d.Set("groups", schema.NewSet(schema.HashString, toInterfaceSlice(userInfo.MemberOf))); err != nil {
		return NewResourceError("setting groups attribute", cfg.UserName, err)
	}

	return nil
}

func minioUpdateUserGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if !d.HasChange("groups") {
		return nil
	}

	cfg := iamUserGroupMembershipConfig(d, meta)

	desired := make(map[string]struct{})
	for _, g := range cfg.Groups {
		desired[g] = struct{}{}
	}

	userInfo, err := cfg.MinioAdmin.GetUserInfo(ctx, cfg.UserName)
	if err != nil {
		return NewResourceError("fetching current groups for user", cfg.UserName, err)
	}
	current := make(map[string]struct{})
	for _, g := range userInfo.MemberOf {
		current[g] = struct{}{}
	}

	for grp := range desired {
		if _, ok := current[grp]; !ok {
			if err := cfg.MinioAdmin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
				Group:    grp,
				Members:  []string{cfg.UserName},
				IsRemove: false,
			}); err != nil {
				return NewResourceError("adding user to group", cfg.UserName, err)
			}
		}
	}

	for grp := range current {
		if _, ok := desired[grp]; ok {
			continue
		}
		if err := cfg.MinioAdmin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
			Group:    grp,
			Members:  []string{cfg.UserName},
			IsRemove: true,
		}); err != nil {
			return NewResourceError("removing user from group", cfg.UserName, err)
		}
	}

	return minioReadUserGroupMembership(ctx, d, meta)
}

func minioDeleteUserGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := iamUserGroupMembershipConfig(d, meta)

	for _, grp := range cfg.Groups {
		if err := cfg.MinioAdmin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
			Group:    grp,
			Members:  []string{cfg.UserName},
			IsRemove: true,
		}); err != nil {
			return NewResourceError("removing user from group", cfg.UserName, err)
		}
	}

	d.SetId("")
	return nil
}

func toInterfaceSlice(strs []string) []interface{} {
	out := make([]interface{}, len(strs))
	for i, s := range strs {
		out[i] = s
	}
	return out
}
