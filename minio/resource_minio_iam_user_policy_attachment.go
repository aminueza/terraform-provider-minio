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

var userPolicyAttachmentLock = NewMutexKV()

func resourceMinioIAMUserPolicyAttachment() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateUserPolicyAttachment,
		ReadContext:   minioReadUserPolicyAttachment,
		DeleteContext: minioDeleteUserPolicyAttachment,
		Importer: &schema.ResourceImporter{
			StateContext: minioImportUserPolicyAttachment,
		},
		Schema: map[string]*schema.Schema{
			"policy_name": {
				Type:         schema.TypeString,
				Description:  "Name of policy to attach to user",
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateIAMNamePolicy,
			},
			"user_name": {
				Type:         schema.TypeString,
				Description:  "Name of user",
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateMinioIamUserName,
			},
		},
	}
}

func minioCreateUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	var userName = d.Get("user_name").(string)
	var policyName = d.Get("policy_name").(string)
	minioAdmin := meta.(*S3MinioClient).S3Admin

	userPolicyAttachmentLock.Lock(userName)
	defer userPolicyAttachmentLock.Unlock(userName)

	policies, err := minioReadUserPolicies(ctx, minioAdmin, userName)
	if err != nil {
		return err
	}
	if !Contains(policies, policyName) {
		policies = append(policies, policyName)
		log.Printf("[DEBUG] Attaching policy %s to user: %s (%v)", policyName, userName, policies)
		err := minioAdmin.SetPolicy(ctx, strings.Join(policies, ","), userName, false)
		if err != nil {
			return NewResourceError("unable to Set User policy", userName+" "+policyName, err)
		}
	}

	d.SetId(id.PrefixedUniqueId(fmt.Sprintf("%s-", userName)))

	return doMinioReadUserPolicyAttachment(ctx, d, meta, userName, policyName)
}
func minioReadUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var userName = d.Get("user_name").(string)
	var policyName = d.Get("policy_name").(string)

	userPolicyAttachmentLock.Lock(userName)
	defer userPolicyAttachmentLock.Unlock(userName)

	return doMinioReadUserPolicyAttachment(ctx, d, meta, userName, policyName)
}
func doMinioReadUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}, userName, policyName string) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	policies, errUser := minioReadUserPolicies(ctx, minioAdmin, userName)
	if errUser != nil {
		return errUser
	}

	if !Contains(policies, policyName) {
		log.Printf("[WARN] No such policy by name (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err := d.Set("policy_name", policyName); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func minioDeleteUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	var userName = d.Get("user_name").(string)
	var policyName = d.Get("policy_name").(string)

	userPolicyAttachmentLock.Lock(userName)
	defer userPolicyAttachmentLock.Unlock(userName)

	policies, err := minioReadUserPolicies(ctx, minioAdmin, userName)
	if err != nil {
		return err
	}

	newPolicies, found := Filter(policies, policyName)
	if !found {
		return nil
	}

	log.Printf("[DEBUG] Detaching policy %s from user: %s (%v)", policyName, userName, newPolicies)
	errIam := minioAdmin.SetPolicy(ctx, strings.Join(newPolicies, ","), userName, false)
	if errIam != nil {
		return NewResourceError("unable to delete user policy", userName, errIam)
	}

	return nil
}

func minioImportUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	idParts := strings.SplitN(d.Id(), "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		return nil, fmt.Errorf("unexpected format of ID (%q), expected <user-name>/<policy_name>", d.Id())
	}

	userName := idParts[0]
	policyName := idParts[1]

	err := d.Set("user_name", userName)
	if err != nil {
		return nil, errors.New(NewResourceErrorStr("unable to import user policy", userName, err))
	}
	err = d.Set("policy_name", policyName)
	if err != nil {
		return nil, errors.New(NewResourceErrorStr("unable to import user policy", userName, err))
	}
	d.SetId(id.PrefixedUniqueId(fmt.Sprintf("%s-", userName)))
	return []*schema.ResourceData{d}, nil
}

func minioReadUserPolicies(ctx context.Context, minioAdmin *madmin.AdminClient, userName string) ([]string, diag.Diagnostics) {
	var isLDAPUser = LDAPUserDistinguishedNamePattern.MatchString(userName)

	log.Printf("[DEBUG] UserPolicyAttachment: is user '%s' an LDAP user? %t", userName, isLDAPUser)

	userInfo, errUser := minioAdmin.GetUserInfo(ctx, userName)
	if errUser != nil {
		errUserResponse, errUserIsResponse := errUser.(madmin.ErrorResponse)

		log.Printf("[DEBUG] UserPolicyAttachment: got an error, errUserIsResponse=%t, errUserResponse.Code=%s", errUserIsResponse, errUserResponse.Code)

		if strings.EqualFold(errUserResponse.Code, "XMinioAdminNoSuchUser") {
			return nil, nil
		} else {
			if !isLDAPUser || !errUserIsResponse {
				return nil, NewResourceError("failed to load user Infos", userName, errUser)
			}
		}
	}
	if userInfo.PolicyName == "" {
		return nil, nil
	}
	return strings.Split(userInfo.PolicyName, ","), nil
}
