package minio

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioIAMGroupPolicyAttachment() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateGroupPolicyAttachment,
		ReadContext:   minioReadGroupPolicyAttachment,
		DeleteContext: minioDeleteGroupPolicyAttachment,
		Importer: &schema.ResourceImporter{
			StateContext: minioImportGroupPolicyAttachment,
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

func minioCreateGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var groupName = d.Get("group_name").(string)
	var policyName = d.Get("policy_name").(string)

	log.Printf("[DEBUG] Attaching policy %s to group: %s", policyName, groupName)
	err := minioAdmin.SetPolicy(ctx, policyName, groupName, true)
	if err != nil {
		return NewResourceError("Unable to attach group policy", groupName+" "+policyName, err)
	}

	d.SetId(resource.PrefixedUniqueId(fmt.Sprintf("%s-", groupName)))

	return minioReadGroupPolicyAttachment(ctx, d, meta)
}

func minioReadGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var groupName = d.Get("group_name").(string)

	groupInfo, errGroup := minioAdmin.GetGroupDescription(ctx, groupName)
	if errGroup != nil {
		return NewResourceError("Fail to load group infos", groupName, errGroup)
	}

	if groupInfo.Policy == "" {
		log.Printf("[WARN] No such policy by name (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err := d.Set("policy_name", string(groupInfo.Policy)); err != nil {
		return NewResourceError("Fail to load group infos", groupName, err)
	}

	return nil
}

func minioDeleteGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var groupName = d.Get("group_name").(string)

	errIam := minioAdmin.SetPolicy(ctx, "", groupName, true)
	if errIam != nil {
		return NewResourceError("Unable to delete user policy", groupName, errIam)
	}

	return nil
}

func minioImportGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	idParts := strings.SplitN(d.Id(), "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		return nil, fmt.Errorf("unexpected format of ID (%q), expected <group-name>/<policy-name>", d.Id())
	}

	groupName := idParts[0]
	policyName := idParts[1]

	err := d.Set("group_name", groupName)
	if err != nil {
		return nil, errors.New(NewResourceErrorStr("Unable to import group policy", groupName, err))
	}
	err = d.Set("policy_name", policyName)
	if err != nil {
		return nil, errors.New(NewResourceErrorStr("Unable to import group policy", groupName, err))
	}
	d.SetId(resource.PrefixedUniqueId(fmt.Sprintf("%s-", groupName)))
	return []*schema.ResourceData{d}, nil
}
