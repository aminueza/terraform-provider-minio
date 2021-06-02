package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go"
)

func resourceMinioIAMGroupUserAttachment() *schema.Resource {
	return &schema.Resource{
		Create: minioCreateGroupUserAttachment,
		Read:   minioReadGroupUserAttachment,
		Delete: minioDeleteGroupUserAttachment,
		Importer: &schema.ResourceImporter{
			State: minioImportGroupUserAttachment,
		},
		Schema: map[string]*schema.Schema{
			"group_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateIAMNamePolicy,
			},
			"user_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateMinioIamUserName,
			},
		},
	}
}

func minioCreateGroupUserAttachment(d *schema.ResourceData, meta interface{}) error {

	iamGroupMembershipConfig := IAMGroupAttachmentConfig(d, meta)

	var groupAddRemove madmin.GroupAddRemove

	groupAddRemove = madmin.GroupAddRemove{
		Group:    iamGroupMembershipConfig.MinioIAMGroup,
		Members:  []string{iamGroupMembershipConfig.MinioIAMUser},
		IsRemove: false,
	}

	err := iamGroupMembershipConfig.MinioAdmin.UpdateGroupMembers(context.Background(), groupAddRemove)
	if err != nil {
		return fmt.Errorf("Error updating user %s to group %s: %s", iamGroupMembershipConfig.MinioIAMUser, iamGroupMembershipConfig.MinioIAMGroup, err)
	}

	d.SetId(resource.PrefixedUniqueId(fmt.Sprintf("%s/%s", iamGroupMembershipConfig.MinioIAMGroup, iamGroupMembershipConfig.MinioIAMUser)))

	return minioReadGroupUserAttachment(d, meta)
}

func minioReadGroupUserAttachment(d *schema.ResourceData, meta interface{}) error {
	iamGroupMembershipConfig := IAMGroupAttachmentConfig(d, meta)

	groupDesc, err := iamGroupMembershipConfig.MinioAdmin.GetGroupDescription(context.Background(), iamGroupMembershipConfig.MinioIAMGroup)

	if err != nil {
		return NewResourceError("Fail to load group infos", iamGroupMembershipConfig.MinioIAMGroup, err)
	}
	if !Contains(groupDesc.Members, iamGroupMembershipConfig.MinioIAMUser) {
		log.Printf(
			"[WARN] No such User by name (%s) in Group (%s) found, removing from state",
			iamGroupMembershipConfig.MinioIAMUser,
			iamGroupMembershipConfig.MinioIAMGroup,
		)
		d.SetId("")
	}
	return nil
}

func minioDeleteGroupUserAttachment(d *schema.ResourceData, meta interface{}) error {

	iamGroupMembershipConfig := IAMGroupAttachmentConfig(d, meta)
	var groupAddRemove madmin.GroupAddRemove

	groupAddRemove = madmin.GroupAddRemove{
		Group:    iamGroupMembershipConfig.MinioIAMGroup,
		Members:  []string{iamGroupMembershipConfig.MinioIAMUser},
		IsRemove: true,
	}

	err := iamGroupMembershipConfig.MinioAdmin.UpdateGroupMembers(context.Background(), groupAddRemove)
	if err != nil {
		return fmt.Errorf("Error updating user(s) to group %s: %s", iamGroupMembershipConfig.MinioIAMGroup, err)
	}

	return nil
}

func minioImportGroupUserAttachment(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	idParts := strings.SplitN(d.Id(), "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		return nil, fmt.Errorf("unexpected format of ID (%q), expected <group-name>/<user-name>", d.Id())
	}

	groupName := idParts[0]
	userName := idParts[1]

	err := d.Set("user_name", userName)
	if err != nil {
		return nil, NewResourceError("Unable to import user policy", userName, err)
	}
	err = d.Set("group_name", groupName)
	if err != nil {
		return nil, NewResourceError("Unable to import user policy", userName, err)
	}
	d.SetId(resource.PrefixedUniqueId(fmt.Sprintf("%s/%s", groupName, userName)))
	return []*schema.ResourceData{d}, nil
}
