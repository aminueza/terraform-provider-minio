package minio

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
)

func resourceMinioIAMGroupMembership() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateGroupMembership,
		ReadContext:   minioReadGroupMembership,
		UpdateContext: minioUpdateGroupMembership,
		DeleteContext: minioDeleteGroupMembership,
		Importer: &schema.ResourceImporter{
			StateContext: minioImportGroupMembership,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Description: "Name of group membership",
				Required:    true,
			},
			"users": {
				Type:        schema.TypeSet,
				Required:    true,
				Description: "Add user or list of users such as a group membership",
				Elem:        &schema.Schema{Type: schema.TypeString},
				Set:         schema.HashString,
			},
			"group": {
				Type:        schema.TypeString,
				Description: "Group name to add users",
				Required:    true,
				ForceNew:    true,
			},
		},
	}
}

func minioCreateGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamGroupMembershipConfig := IAMGroupMembershipConfig(d, meta)

	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupMembershipConfig.MinioIAMGroup,
		Members:  iamGroupMembershipConfig.MinioIAMUsers,
		IsRemove: false,
	}

	err := iamGroupMembershipConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		return NewResourceError("error adding user(s) to group", iamGroupMembershipConfig.MinioIAMGroup, err)
	}

	d.SetId(iamGroupMembershipConfig.MinioIAMGroup)

	return minioReadGroupMembership(ctx, d, meta)
}

func minioUpdateGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamGroupMembershipConfig := IAMGroupMembershipConfig(d, meta)

	if d.HasChange("users") {
		on, nn := d.GetChange("users")

		if on == nil && nn == nil {
			return minioReadGroupMembership(ctx, d, meta)
		}

		if on == nil {
			on = new(schema.Set)
		}
		if nn == nil {
			nn = new(schema.Set)
		}

		os := on.(*schema.Set)
		ns := nn.(*schema.Set)
		usersToRemove := getStringList(os.Difference(ns).List())
		usersToAdd := getStringList(ns.Difference(os).List())

		if len(usersToAdd) > 0 {
			if err := userToADD(ctx, iamGroupMembershipConfig, usersToAdd); err != nil {
				return NewResourceError("adding users to group", iamGroupMembershipConfig.MinioIAMGroup, err)
			}
		}
		if len(usersToRemove) > 0 {
			if err := userToRemove(ctx, iamGroupMembershipConfig, usersToRemove); err != nil {
				// Ignore "user does not exist" errors - the user may have been deleted
				// before the membership update, which effectively removes them from the group
				if !isGroupMembershipUserMissing(err) {
					return NewResourceError("removing users from group", iamGroupMembershipConfig.MinioIAMGroup, err)
				}
			}
		}

	}

	return minioReadGroupMembership(ctx, d, meta)
}

func minioReadGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamGroupMembershipConfig := IAMGroupMembershipConfig(d, meta)
	groupName := iamGroupMembershipConfig.MinioIAMGroup
	if groupName == "" {
		groupName = d.Id()
	}
	if groupName == "" {
		d.SetId("")
		return nil
	}

	groupDesc, err := iamGroupMembershipConfig.MinioAdmin.GetGroupDescription(ctx, groupName)
	if err != nil {
		if isGroupMembershipGroupMissing(err) || strings.Contains(err.Error(), "group does not exist") {
			tflog.Warn(ctx, fmt.Sprintf("No IAM group by name (%s) found, removing from state", groupName))
			d.SetId("")
			return nil
		}
		return NewResourceError("error reading IAM Group", groupName, err)
	}
	if groupDesc == nil {
		return NewResourceError("error reading IAM Group", groupName, errors.New("empty group description"))
	}

	if err := d.Set("group", groupName); err != nil {
		return NewResourceError("error reading IAM Group", groupName, err)
	}
	if err := d.Set("users", groupDesc.Members); err != nil {
		return NewResourceError("error reading IAM Group", groupName, err)
	}

	d.SetId(groupName)

	return nil

}

func minioImportGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	groupName := d.Id()
	if groupName == "" {
		return nil, fmt.Errorf("cannot import IAM group membership: group name is empty")
	}
	if err := d.Set("group", groupName); err != nil {
		return nil, errors.New(NewResourceErrorStr("importing IAM group membership", groupName, err))
	}
	return []*schema.ResourceData{d}, nil
}

func minioDeleteGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamGroupMembershipConfig := IAMGroupMembershipConfig(d, meta)

	// MinIO deletes the entire group (and its policy mapping) when UpdateGroupMembers
	// receives IsRemove=true with an empty member slice. Skip the API call when state
	// already has no members so removing the membership leaves the group intact.
	if len(iamGroupMembershipConfig.MinioIAMUsers) == 0 {
		tflog.Debug(ctx, fmt.Sprintf("No members to remove from IAM group (%s), skipping UpdateGroupMembers", iamGroupMembershipConfig.MinioIAMGroup))
		d.SetId("")
		return nil
	}

	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupMembershipConfig.MinioIAMGroup,
		Members:  iamGroupMembershipConfig.MinioIAMUsers,
		IsRemove: true,
	}

	err := iamGroupMembershipConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		if isGroupMembershipGroupMissing(err) || isGroupMembershipUserMissing(err) {
			d.SetId("")
			return nil
		}
		return NewResourceError("error deleting user(s) to group", iamGroupMembershipConfig.MinioIAMGroup, err)
	}

	d.SetId("")
	return nil
}

func isGroupMembershipGroupMissing(err error) bool {
	var errResp madmin.ErrorResponse
	if errors.As(err, &errResp) {
		return errResp.Code == "XMinioAdminNoSuchGroup"
	}
	return strings.Contains(err.Error(), "group does not exist")
}

func isGroupMembershipUserMissing(err error) bool {
	var errResp madmin.ErrorResponse
	if errors.As(err, &errResp) {
		return errResp.Code == "XMinioAdminNoSuchUser"
	}
	return strings.Contains(err.Error(), "user does not exist")
}

func userToADD(ctx context.Context, iamGroupMembershipConfig *S3MinioIAMGroupMembershipConfig, usersToAdd []string) error {
	groupDesc, err := iamGroupMembershipConfig.MinioAdmin.GetGroupDescription(ctx, iamGroupMembershipConfig.MinioIAMGroup)
	if err != nil {
		return fmt.Errorf("error reading group %s: %w", iamGroupMembershipConfig.MinioIAMGroup, err)
	}
	if groupDesc == nil {
		return fmt.Errorf("error reading group %s: empty group description", iamGroupMembershipConfig.MinioIAMGroup)
	}

	tflog.Warn(ctx, fmt.Sprintf("Users to add before: %v and after: %v", groupDesc.Members, usersToAdd))

	users := make([]string, 0, len(groupDesc.Members)+len(usersToAdd))
	users = append(users, groupDesc.Members...)
	users = append(users, usersToAdd...)

	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupMembershipConfig.MinioIAMGroup,
		Members:  users,
		IsRemove: false,
	}

	if err := iamGroupMembershipConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove); err != nil {
		return fmt.Errorf("error updating user(s) to group %s: %s", iamGroupMembershipConfig.MinioIAMGroup, err)
	}

	return nil
}

func userToRemove(ctx context.Context, iamGroupMembershipConfig *S3MinioIAMGroupMembershipConfig, usersToRemove []string) error {
	if len(usersToRemove) == 0 {
		return nil
	}

	groupDesc, err := iamGroupMembershipConfig.MinioAdmin.GetGroupDescription(ctx, iamGroupMembershipConfig.MinioIAMGroup)
	if err != nil {
		return fmt.Errorf("error reading group %s: %w", iamGroupMembershipConfig.MinioIAMGroup, err)
	}
	if groupDesc == nil {
		return fmt.Errorf("error reading group %s: empty group description", iamGroupMembershipConfig.MinioIAMGroup)
	}

	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupMembershipConfig.MinioIAMGroup,
		Members:  usersToRemove,
		IsRemove: true,
	}

	tflog.Warn(ctx, fmt.Sprintf("Users to remove before: %v and after: %v", groupDesc.Members, groupAddRemove.Members))

	if err := iamGroupMembershipConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove); err != nil {
		return fmt.Errorf("error updating user(s) to group %s: %s", iamGroupMembershipConfig.MinioIAMGroup, err)
	}

	return nil
}
