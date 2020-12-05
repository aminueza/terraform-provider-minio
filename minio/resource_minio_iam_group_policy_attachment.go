package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceMinioIAMGroupPolicyAttachment() *schema.Resource {
	return &schema.Resource{
		Create: minioCreateGroupPolicyAttachment,
		Read:   minioReadGroupPolicyAttachment,
		Delete: minioDeleteGroupPolicyAttachment,
		Importer: &schema.ResourceImporter{
			State: minioImportGroupPolicyAttachment,
		},
		Schema: map[string]*schema.Schema{
			"policy_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateIAMNamePolicy,
			},
			"group_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateMinioIamGroupName,
			},
		},
	}
}

func minioCreateGroupPolicyAttachment(d *schema.ResourceData, meta interface{}) error {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var groupName = d.Get("group_name").(string)
	var policyName = d.Get("policy_name").(string)

	log.Printf("[DEBUG] Attaching policy %s to group: %s", policyName, groupName)
	err := minioAdmin.SetPolicy(context.Background(), policyName, groupName, true)
	if err != nil {
		return NewResourceError("Unable to attach group policy", groupName+" "+policyName, err)
	}

	d.SetId(resource.PrefixedUniqueId(fmt.Sprintf("%s-", groupName)))

	return minioReadGroupPolicyAttachment(d, meta)
}

func minioReadGroupPolicyAttachment(d *schema.ResourceData, meta interface{}) error {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var groupName = d.Get("group_name").(string)

	groupInfo, errGroup := minioAdmin.GetGroupDescription(context.Background(), groupName)
	if errGroup != nil {
		return NewResourceError("Fail to load group infos", groupName, errGroup)
	}

	if groupInfo.Policy == "" {
		log.Printf("[WARN] No such policy by name (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err := d.Set("policy_name", string(groupInfo.Policy)); err != nil {
		return err
	}

	return nil
}

func minioDeleteGroupPolicyAttachment(d *schema.ResourceData, meta interface{}) error {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var groupName = d.Get("group_name").(string)

	errIam := minioAdmin.SetPolicy(context.Background(), "", groupName, true)
	if errIam != nil {
		return NewResourceError("Unable to delete user policy", groupName, errIam)
	}

	return nil
}

func minioImportGroupPolicyAttachment(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	idParts := strings.SplitN(d.Id(), "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		return nil, fmt.Errorf("unexpected format of ID (%q), expected <group-name>/<policy-name>", d.Id())
	}

	groupName := idParts[0]
	policyName := idParts[1]

	err := d.Set("group_name", groupName)
	if err != nil {
		return nil, NewResourceError("Unable to import group policy", groupName, err)
	}
	err = d.Set("policy_name", policyName)
	if err != nil {
		return nil, NewResourceError("Unable to import group policy", groupName, err)
	}
	d.SetId(resource.PrefixedUniqueId(fmt.Sprintf("%s-", groupName)))
	return []*schema.ResourceData{d}, nil
}
