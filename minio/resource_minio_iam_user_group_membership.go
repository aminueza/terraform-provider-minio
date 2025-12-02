package minio

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

// resourceMinioIAMUserGroupMembership defines the Terraform resource for attaching a single IAM user
// to multiple IAM groups.
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

// IAMUserGroupMembershipConfig holds the configuration needed for CRUD operations.
type IAMUserGroupMembershipConfig struct {
	MinioAdmin *madmin.AdminClient
	UserName   string
	Groups     []string
}

// iamUserGroupMembershipConfig extracts the configuration from the resource data.
func iamUserGroupMembershipConfig(d *schema.ResourceData, meta interface{}) *IAMUserGroupMembershipConfig {
	m := meta.(*S3MinioClient)

	// Extract groups from the Set
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

// minioCreateUserGroupMembership creates the membership by adding the user to each specified group.
func minioCreateUserGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := iamUserGroupMembershipConfig(d, meta)

	// Add user to each group
	for _, grp := range cfg.Groups {
		err := cfg.MinioAdmin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
			Group:   grp,
			Members: []string{cfg.UserName},
		})
		if err != nil {
			return diag.FromErr(fmt.Errorf("failed to add user %q to group %q: %w", cfg.UserName, grp, err))
		}
	}

	// Use user name as the resource ID
	d.SetId(cfg.UserName)

	// Reconcile: ensure the user belongs to exactly the groups defined in the resource.
	// Remove any groups the user is a member of that are not in cfg.Groups.
	desired := make(map[string]struct{})
	for _, g := range cfg.Groups {
		desired[g] = struct{}{}
	}
	userInfo, err := cfg.MinioAdmin.GetUserInfo(ctx, cfg.UserName)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to read user %q info for reconciliation: %w", cfg.UserName, err))
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
				return diag.FromErr(fmt.Errorf("failed to remove user %q from extra group %q: %w", cfg.UserName, grp, err))
			}
		}
	}

	return minioReadUserGroupMembership(ctx, d, meta)
}

// minioReadUserGroupMembership reads the current groups for the user.
func minioReadUserGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := iamUserGroupMembershipConfig(d, meta)

	userInfo, err := cfg.MinioAdmin.GetUserInfo(ctx, cfg.UserName)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to read user %q info: %w", cfg.UserName, err))
	}

	// Ensure 'user' attribute is set in state (required for import)
	if _, ok := d.GetOk("user"); !ok {
		if err := d.Set("user", cfg.UserName); err != nil {
			return diag.FromErr(err)
		}
	}

	// Set the groups attribute to the current membership
	if err := d.Set("groups", schema.NewSet(schema.HashString, toInterfaceSlice(userInfo.MemberOf))); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

// minioUpdateUserGroupMembership updates the membership by reconciling desired vs actual groups.
func minioUpdateUserGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if !d.HasChange("groups") {
		return nil
	}

	cfg := iamUserGroupMembershipConfig(d, meta)

	// Desired groups from the resource
	desired := make(map[string]struct{})
	for _, g := range cfg.Groups {
		desired[g] = struct{}{}
	}

	// Current groups from MinIO
	userInfo, err := cfg.MinioAdmin.GetUserInfo(ctx, cfg.UserName)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to fetch current groups for user %q: %w", cfg.UserName, err))
	}
	current := make(map[string]struct{})
	for _, g := range userInfo.MemberOf {
		current[g] = struct{}{}
	}

	// Add missing groups
	for grp := range desired {
		if _, ok := current[grp]; !ok {
			if err := cfg.MinioAdmin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
				Group:    grp,
				Members:  []string{cfg.UserName},
				IsRemove: false,
			}); err != nil {
				return diag.FromErr(fmt.Errorf("failed to add user %q to group %q: %w", cfg.UserName, grp, err))
			}
		}
	}

	// Remove extra groups
	for grp := range current {
		if _, ok := desired[grp]; ok {
			// No action needed
			continue
		}
		if err := cfg.MinioAdmin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
			Group:    grp,
			Members:  []string{cfg.UserName},
			IsRemove: true,
		}); err != nil {
			return diag.FromErr(fmt.Errorf("failed to remove user %q from group %q: %w", cfg.UserName, grp, err))
		}
	}

	return minioReadUserGroupMembership(ctx, d, meta)
}

// minioDeleteUserGroupMembership removes the user from all groups managed by this resource.
func minioDeleteUserGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := iamUserGroupMembershipConfig(d, meta)

	// Remove user from each group listed in state
	for _, grp := range cfg.Groups {
		if err := cfg.MinioAdmin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
			Group:    grp,
			Members:  []string{cfg.UserName},
			IsRemove: true,
		}); err != nil {
			return diag.FromErr(fmt.Errorf("failed to remove user %q from group %q: %w", cfg.UserName, grp, err))
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
