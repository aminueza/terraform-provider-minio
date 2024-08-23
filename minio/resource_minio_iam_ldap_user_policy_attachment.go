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

var ldapUserPolicyAttachmentLock = NewMutexKV()

func resourceMinioIAMLDAPUserPolicyAttachment() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateLDAPUserPolicyAttachment,
		ReadContext:   minioReadLDAPUserPolicyAttachment,
		DeleteContext: minioDeleteLDAPUserPolicyAttachment,
		Importer: &schema.ResourceImporter{
			StateContext: minioImportLDAPUserPolicyAttachment,
		},
		Schema: map[string]*schema.Schema{
			"policy_name": {
				Type:         schema.TypeString,
				Description:  "Name of policy to attach to user",
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateIAMNamePolicy,
			},
			"user_dn": {
				Type:         schema.TypeString,
				Description:  "The dn of user to attach policy to",
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateMinioIamUserName,
			},
		},
	}
}

func minioCreateLDAPUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var userDN = d.Get("user_dn").(string)
	var policyName = d.Get("policy_name").(string)

	ldapUserPolicyAttachmentLock.Lock(userDN)
	defer ldapUserPolicyAttachmentLock.Unlock(userDN)

	policies, err := minioReadLDAPUserPolicies(ctx, minioAdmin, userDN)
	if err != nil {
		return err
	}

	log.Printf("[DEBUG] '%s' user policies: %v", userDN, policies)

	if !Contains(policies, policyName) {
		log.Printf("[DEBUG] Attaching policy %s to user: %s", policyName, userDN)
		paResp, err := minioAdmin.AttachPolicyLDAP(ctx, madmin.PolicyAssociationReq{
			Policies: []string{policyName},
			User:     userDN,
		})

		log.Printf("[DEBUG] PolicyAssociationResp: %v", paResp)

		if err != nil {
			return NewResourceError(fmt.Sprintf("Unable to attach user to policy '%s'", policyName), userDN, err)
		}
	}

	d.SetId(fmt.Sprintf("%s/%s", policyName, userDN))

	return doMinioReadLDAPUserPolicyAttachment(ctx, d, meta, userDN, policyName)
}

func minioReadLDAPUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var userDN = d.Get("user_dn").(string)
	var policyName = d.Get("policy_name").(string)

	ldapUserPolicyAttachmentLock.Lock(userDN)
	defer ldapUserPolicyAttachmentLock.Unlock(userDN)

	return doMinioReadLDAPUserPolicyAttachment(ctx, d, meta, userDN, policyName)
}

func doMinioReadLDAPUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}, userDN, policyName string) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	per, err := minioAdmin.GetLDAPPolicyEntities(ctx, madmin.PolicyEntitiesQuery{
		Policy: []string{policyName},
		Users:  []string{userDN},
	})

	if err != nil {
		return NewResourceError(fmt.Sprintf("Failed to query for user policy '%s'", policyName), userDN, err)
	}

	log.Printf("[DEBUG] PolicyEntityResponse: %v", per)
	if len(per.PolicyMappings) == 0 {
		log.Printf("[WARN] No such policy association (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err := d.Set("policy_name", policyName); err != nil {
		return NewResourceError("failed to load user infos", userDN, err)
	}

	return nil
}

func minioDeleteLDAPUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var userDN = d.Get("user_dn").(string)
	var policyName = d.Get("policy_name").(string)

	ldapUserPolicyAttachmentLock.Lock(userDN)
	defer ldapUserPolicyAttachmentLock.Unlock(userDN)

	policies, err := minioReadLDAPUserPolicies(ctx, minioAdmin, userDN)
	if err != nil {
		return err
	}

	_, found := Filter(policies, policyName)
	if !found {
		return nil
	}

	paResp, detachErr := minioAdmin.DetachPolicyLDAP(ctx, madmin.PolicyAssociationReq{
		Policies: []string{policyName},
		User:     userDN,
	})

	log.Printf("[DEBUG] PolicyAssociationResp: %v", paResp)

	if detachErr != nil {
		return NewResourceError(fmt.Sprintf("Unable to detach policy '%s'", policyName), userDN, detachErr)
	}

	return nil
}

func minioImportLDAPUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	idParts := strings.SplitN(d.Id(), "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		return nil, fmt.Errorf("unexpected format of ID (%q), expected <user-name>/<policy-name>", d.Id())
	}

	userDN := idParts[0]
	policyName := idParts[1]

	err := d.Set("user_dn", userDN)
	if err != nil {
		return nil, errors.New(NewResourceErrorStr("unable to import user policy", userDN, err))
	}
	err = d.Set("policy_name", policyName)
	if err != nil {
		return nil, errors.New(NewResourceErrorStr("unable to import user policy", userDN, err))
	}

	d.SetId(fmt.Sprintf("%s/%s", policyName, userDN))
	return []*schema.ResourceData{d}, nil
}

func minioReadLDAPUserPolicies(ctx context.Context, minioAdmin *madmin.AdminClient, userDN string) ([]string, diag.Diagnostics) {
	policyEntities, err := minioAdmin.GetLDAPPolicyEntities(ctx, madmin.PolicyEntitiesQuery{
		Users: []string{userDN},
	})

	if err != nil {
		return nil, NewResourceError("failed to load user infos", userDN, err)
	}

	if len(policyEntities.UserMappings) == 0 {
		return nil, nil
	}

	if len(policyEntities.UserMappings) > 1 {
		return nil, NewResourceError("failed to load user infos", userDN, errors.New("More than one user returned when getting LDAP policies for single user"))
	}

	return policyEntities.UserMappings[0].Policies, nil
}
