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

var ldapGroupPolicyAttachmentLock = NewMutexKV()

func resourceMinioIAMLDAPGroupPolicyAttachment() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateLDAPGroupPolicyAttachment,
		ReadContext:   minioReadLDAPGroupPolicyAttachment,
		DeleteContext: minioDeleteLDAPGroupPolicyAttachment,
		Importer: &schema.ResourceImporter{
			StateContext: minioImportLDAPGroupPolicyAttachment,
		},
		Schema: map[string]*schema.Schema{
			"policy_name": {
				Type:         schema.TypeString,
				Description:  "Name of policy to attach to group",
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateIAMNamePolicy,
			},
			"group_name": {
				Type:         schema.TypeString,
				Description:  "Name of group to attach policy to",
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateMinioIamGroupName,
			},
		},
	}
}

func minioCreateLDAPGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var groupName = d.Get("group_name").(string)
	var policyName = d.Get("policy_name").(string)

	ldapGroupPolicyAttachmentLock.Lock(groupName)
	defer ldapGroupPolicyAttachmentLock.Unlock(groupName)

	policies, err := minioReadLDAPGroupPolicies(ctx, minioAdmin, groupName)
	if err != nil {
		return err
	}

	log.Printf("[DEBUG] '%s' group policies: %v", groupName, policies)

	if !Contains(policies, policyName) {
		log.Printf("[DEBUG] Attaching policy %s to group: %s", policyName, groupName)
		paResp, err := minioAdmin.AttachPolicyLDAP(ctx, madmin.PolicyAssociationReq{
			Policies: []string{policyName},
			Group:    groupName,
		})

		log.Printf("[DEBUG] PolicyAssociationResp: %v", paResp)

		if err != nil {
			return NewResourceError(fmt.Sprintf("Unable to attach group to policy '%s'", policyName), groupName, err)
		}
	}

	d.SetId(fmt.Sprintf("%s/%s", policyName, groupName))

	return doMinioReadLDAPGroupPolicyAttachment(ctx, d, meta, groupName, policyName)
}

func minioReadLDAPGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var groupName = d.Get("group_name").(string)
	var policyName = d.Get("policy_name").(string)

	ldapGroupPolicyAttachmentLock.Lock(groupName)
	defer ldapGroupPolicyAttachmentLock.Unlock(groupName)

	return doMinioReadLDAPGroupPolicyAttachment(ctx, d, meta, groupName, policyName)
}

func doMinioReadLDAPGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}, groupName, policyName string) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	per, err := minioAdmin.GetLDAPPolicyEntities(ctx, madmin.PolicyEntitiesQuery{
		Policy: []string{policyName},
		Groups: []string{groupName},
	})

	if err != nil {
		return NewResourceError(fmt.Sprintf("Failed to query for group policy '%s'", policyName), groupName, err)
	}

	log.Printf("[DEBUG] PolicyEntityResponse: %v", per)
	if len(per.PolicyMappings) == 0 {
		log.Printf("[WARN] No such policy association (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err := d.Set("policy_name", policyName); err != nil {
		return NewResourceError("failed to load group infos", groupName, err)
	}

	return nil
}

func minioDeleteLDAPGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var groupName = d.Get("group_name").(string)
	var policyName = d.Get("policy_name").(string)

	ldapGroupPolicyAttachmentLock.Lock(groupName)
	defer ldapGroupPolicyAttachmentLock.Unlock(groupName)

	policies, err := minioReadLDAPGroupPolicies(ctx, minioAdmin, groupName)
	if err != nil {
		return err
	}

	_, found := Filter(policies, policyName)
	if !found {
		return nil
	}

	paResp, detachErr := minioAdmin.DetachPolicyLDAP(ctx, madmin.PolicyAssociationReq{
		Policies: []string{policyName},
		Group:    groupName,
	})

	log.Printf("[DEBUG] PolicyAssociationResp: %v", paResp)

	if detachErr != nil {
		return NewResourceError(fmt.Sprintf("Unable to detach policy '%s'", policyName), groupName, detachErr)
	}

	return nil
}

func minioImportLDAPGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
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

func minioReadLDAPGroupPolicies(ctx context.Context, minioAdmin *madmin.AdminClient, groupName string) ([]string, diag.Diagnostics) {
	policyEntities, err := minioAdmin.GetLDAPPolicyEntities(ctx, madmin.PolicyEntitiesQuery{
		Groups: []string{groupName},
	})

	if err != nil {
		return nil, NewResourceError("failed to load group infos", groupName, err)
	}

	return policyEntities.GroupMappings[0].Policies, nil
}
