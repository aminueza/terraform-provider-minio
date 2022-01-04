package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio/pkg/madmin"
)

func resourceMinioIAMGroupMembership() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateGroupMembership,
		ReadContext:   minioReadGroupMembership,
		UpdateContext: minioUpdateGroupMembership,
		DeleteContext: minioDeleteGroupMembership,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of group membership",
			},
			"users": {
				Type:        schema.TypeSet,
				Required:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Set:         schema.HashString,
				Description: "Add user or list of users such as a group membership",
			},
			"group": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Group name to add users",
			},
		},
	}
}

func minioCreateGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamGroupMembershipConfig := IAMGroupMembersipConfig(d, meta)

	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupMembershipConfig.MinioIAMGroup,
		Members:  aws.StringValueSlice(iamGroupMembershipConfig.MinioIAMUsers),
		IsRemove: false,
	}

	err := iamGroupMembershipConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		return NewResourceError("Error adding user(s) to group", iamGroupMembershipConfig.MinioIAMGroup, err)
	}

	d.SetId(iamGroupMembershipConfig.MinioIAMName)

	return minioReadGroupMembership(ctx, d, meta)
}

func minioUpdateGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamGroupMembershipConfig := IAMGroupMembersipConfig(d, meta)

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
			_ = userToADD(ctx, iamGroupMembershipConfig, usersToAdd)
		} else {
			_ = userToRemove(ctx, iamGroupMembershipConfig, usersToRemove)
		}

	}

	return minioReadGroupMembership(ctx, d, meta)
}

func minioReadGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamGroupMembershipConfig := IAMGroupMembersipConfig(d, meta)

	groupDesc, err := iamGroupMembershipConfig.MinioAdmin.GetGroupDescription(ctx, iamGroupMembershipConfig.MinioIAMGroup)
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			log.Printf("[WARN] No IAM group by name (%s) found, removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return NewResourceError("Error reading IAM Group", d.Id(), err)
	}

	if groupDesc.Name == "" {
		return nil
	}

	if err := d.Set("users", groupDesc.Members); err != nil {
		return NewResourceError("Error reading IAM Group", d.Id(), err)
	}

	d.SetId(iamGroupMembershipConfig.MinioIAMName)

	return nil

}

func minioDeleteGroupMembership(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamGroupMembershipConfig := IAMGroupMembersipConfig(d, meta)

	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupMembershipConfig.MinioIAMGroup,
		Members:  aws.StringValueSlice(iamGroupMembershipConfig.MinioIAMUsers),
		IsRemove: true,
	}

	err := iamGroupMembershipConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		return NewResourceError("Error deleting user(s) to group", iamGroupMembershipConfig.MinioIAMGroup, err)
	}

	return nil
}

func isMinioIamUser(ctx context.Context, iamGroupMembershipConfig *S3MinioIAMGroupMembershipConfig, usersToRemove []*string) []string {

	var users []string

	for _, user := range usersToRemove {
		if userInfo, _ := iamGroupMembershipConfig.MinioAdmin.GetUserInfo(ctx, aws.StringValue(user)); userInfo.SecretKey != "" {
			users = append(users, aws.StringValue(user))
		}
	}

	return users
}

func userToADD(ctx context.Context, iamGroupMembershipConfig *S3MinioIAMGroupMembershipConfig, usersToAdd []*string) error {
	var users []string

	groupDesc, _ := iamGroupMembershipConfig.MinioAdmin.GetGroupDescription(ctx, iamGroupMembershipConfig.MinioIAMGroup)

	log.Printf("[WARN] Users to add before: %v and after: %v", groupDesc.Members, aws.StringValueSlice(usersToAdd))

	users = append(groupDesc.Members, aws.StringValueSlice(usersToAdd)...)

	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupMembershipConfig.MinioIAMGroup,
		Members:  users,
		IsRemove: false,
	}

	err := iamGroupMembershipConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		return fmt.Errorf("Error updating user(s) to group %s: %s", iamGroupMembershipConfig.MinioIAMGroup, err)
	}

	return nil
}

func userToRemove(ctx context.Context, iamGroupMembershipConfig *S3MinioIAMGroupMembershipConfig, usersToRemove []*string) error {

	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupMembershipConfig.MinioIAMGroup,
		Members:  aws.StringValueSlice(usersToRemove),
		IsRemove: true,
	}

	groupDesc, _ := iamGroupMembershipConfig.MinioAdmin.GetGroupDescription(ctx, iamGroupMembershipConfig.MinioIAMGroup)

	log.Printf("[WARN] Users to remove before: %v and after: %v", groupDesc.Members, groupAddRemove.Members)

	err := iamGroupMembershipConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		return fmt.Errorf("Error updating user(s) to group %s: %s", iamGroupMembershipConfig.MinioIAMGroup, err)
	}

	return nil
}
