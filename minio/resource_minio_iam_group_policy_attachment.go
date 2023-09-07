package minio

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

var groupPolicyAttachmentLock = NewMutexKV()

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

	groupPolicyAttachmentLock.Lock(groupName)
	defer groupPolicyAttachmentLock.Unlock(groupName)

	policies, err := minioReadGroupPolicies(ctx, minioAdmin, groupName)
	if err != nil {
		return err
	}
	if !Contains(policies, policyName) {
		log.Printf("[DEBUG] Attaching policy %s to group: %s", policyName, groupName)
		policies = append(policies, policyName)
		err := minioAdmin.SetPolicy(ctx, strings.Join(policies, ","), groupName, true)
		if err != nil {
			return NewResourceError("unable to attach group policy", groupName+" "+policyName, err)
		}
	}

	d.SetId(id.PrefixedUniqueId(fmt.Sprintf("%s-", groupName)))

	return doMinioReadGroupPolicyAttachment(ctx, d, meta, groupName, policyName)
}

func minioReadGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var groupName = d.Get("group_name").(string)
	var policyName = d.Get("policy_name").(string)

	groupPolicyAttachmentLock.Lock(groupName)
	defer groupPolicyAttachmentLock.Unlock(groupName)

	return doMinioReadGroupPolicyAttachment(ctx, d, meta, groupName, policyName)
}
func doMinioReadGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}, groupName, policyName string) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin
	policies, err := minioReadGroupPolicies(ctx, minioAdmin, groupName)
	if err != nil {
		return err
	}
	if !Contains(policies, policyName) {
		log.Printf("[WARN] No such policy by name (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err := d.Set("policy_name", policyName); err != nil {
		return NewResourceError("failed to load group infos", groupName, err)
	}

	return nil
}

func minioDeleteGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var groupName = d.Get("group_name").(string)
	var policyName = d.Get("policy_name").(string)

	groupPolicyAttachmentLock.Lock(groupName)
	defer groupPolicyAttachmentLock.Unlock(groupName)

	policies, err := minioReadGroupPolicies(ctx, minioAdmin, groupName)
	if err != nil {
		return err
	}

	newPolicies, found := Filter(policies, policyName)
	if !found {
		return nil
	}

	errIam := minioAdmin.SetPolicy(ctx, strings.Join(newPolicies, ","), groupName, true)
	if errIam != nil {
		return NewResourceError("unable to delete user policy", groupName, errIam)
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
		return nil, errors.New(NewResourceErrorStr("unable to import group policy", groupName, err))
	}
	err = d.Set("policy_name", policyName)
	if err != nil {
		return nil, errors.New(NewResourceErrorStr("unable to import group policy", groupName, err))
	}
	d.SetId(id.PrefixedUniqueId(fmt.Sprintf("%s-", groupName)))
	return []*schema.ResourceData{d}, nil
}

func minioReadGroupPolicies(ctx context.Context, minioAdmin *madmin.AdminClient, groupName string) ([]string, diag.Diagnostics) {
	groupInfo, errGroup := minioAdmin.GetGroupDescription(ctx, groupName)
	if errGroup != nil {
		return nil, NewResourceError("failed to load group infos", groupName, errGroup)
	}
	if groupInfo.Policy == "" {
		return nil, nil
	}
	return strings.Split(groupInfo.Policy, ","), nil
}
