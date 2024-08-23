package minio

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
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
			"group_dn": {
				Type:         schema.TypeString,
				Description:  "The dn of group to attach policy to",
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateMinioIamGroupName,
			},
		},
	}
}

func minioCreateLDAPGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var groupDN = d.Get("group_dn").(string)
	var policyName = d.Get("policy_name").(string)

	ldapGroupPolicyAttachmentLock.Lock(groupDN)
	defer ldapGroupPolicyAttachmentLock.Unlock(groupDN)

	policies, err := minioReadLDAPGroupPolicies(ctx, minioAdmin, groupDN)
	if err != nil {
		return err
	}

	log.Printf("[DEBUG] '%s' group policies: %v", groupDN, policies)

	if !Contains(policies, policyName) {
		log.Printf("[DEBUG] Attaching policy %s to group: %s", policyName, groupDN)
		paResp, err := minioAdmin.AttachPolicyLDAP(ctx, madmin.PolicyAssociationReq{
			Policies: []string{policyName},
			Group:    groupDN,
		})

		log.Printf("[DEBUG] PolicyAssociationResp: %v", paResp)

		if err != nil {
			return NewResourceError(fmt.Sprintf("Unable to attach group to policy '%s'", policyName), groupDN, err)
		}
	}

	d.SetId(fmt.Sprintf("%s/%s", policyName, groupDN))

	return doMinioReadLDAPGroupPolicyAttachment(ctx, d, meta, groupDN, policyName)
}

func minioReadLDAPGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var groupDN = d.Get("group_dn").(string)
	var policyName = d.Get("policy_name").(string)

	ldapGroupPolicyAttachmentLock.Lock(groupDN)
	defer ldapGroupPolicyAttachmentLock.Unlock(groupDN)

	return doMinioReadLDAPGroupPolicyAttachment(ctx, d, meta, groupDN, policyName)
}

func doMinioReadLDAPGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}, groupDN, policyName string) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	per, err := minioAdmin.GetLDAPPolicyEntities(ctx, madmin.PolicyEntitiesQuery{
		Policy: []string{policyName},
		Groups: []string{groupDN},
	})

	if err != nil {
		return NewResourceError(fmt.Sprintf("Failed to query for group policy '%s'", policyName), groupDN, err)
	}

	log.Printf("[DEBUG] PolicyEntityResponse: %v", per)
	if len(per.PolicyMappings) == 0 {
		log.Printf("[WARN] No such policy association (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err := d.Set("policy_name", policyName); err != nil {
		return NewResourceError("failed to load group infos", groupDN, err)
	}

	return nil
}

func minioDeleteLDAPGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var groupDN = d.Get("group_dn").(string)
	var policyName = d.Get("policy_name").(string)

	ldapGroupPolicyAttachmentLock.Lock(groupDN)
	defer ldapGroupPolicyAttachmentLock.Unlock(groupDN)

	policies, err := minioReadLDAPGroupPolicies(ctx, minioAdmin, groupDN)
	if err != nil {
		return err
	}

	_, found := Filter(policies, policyName)
	if !found {
		return nil
	}

	paResp, detachErr := minioAdmin.DetachPolicyLDAP(ctx, madmin.PolicyAssociationReq{
		Policies: []string{policyName},
		Group:    groupDN,
	})

	log.Printf("[DEBUG] PolicyAssociationResp: %v", paResp)

	if detachErr != nil {
		return NewResourceError(fmt.Sprintf("Unable to detach policy '%s'", policyName), groupDN, detachErr)
	}

	return nil
}

func minioImportLDAPGroupPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	idParts := strings.SplitN(d.Id(), "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		return nil, fmt.Errorf("unexpected format of ID (%q), expected <group-name>/<policy-name>", d.Id())
	}

	groupDN := idParts[0]
	policyName := idParts[1]

	err := d.Set("group_dn", groupDN)
	if err != nil {
		return nil, errors.New(NewResourceErrorStr("unable to import group policy", groupDN, err))
	}
	err = d.Set("policy_name", policyName)
	if err != nil {
		return nil, errors.New(NewResourceErrorStr("unable to import group policy", groupDN, err))
	}

	d.SetId(fmt.Sprintf("%s/%s", policyName, groupDN))
	return []*schema.ResourceData{d}, nil
}

func minioReadLDAPGroupPolicies(ctx context.Context, minioAdmin *madmin.AdminClient, groupDN string) ([]string, diag.Diagnostics) {
	policyEntities, err := minioAdmin.GetLDAPPolicyEntities(ctx, madmin.PolicyEntitiesQuery{
		Groups: []string{groupDN},
	})

	if err != nil {
		return nil, NewResourceError("failed to load group infos", groupDN, err)
	}

	if len(policyEntities.GroupMappings) == 0 {
		return nil, nil
	}

	if len(policyEntities.GroupMappings) > 1 {
		return nil, NewResourceError("failed to load user infos", groupDN, errors.New("More than one group returned when getting LDAP policies for single group"))
	}

	return policyEntities.GroupMappings[0].Policies, nil
}
