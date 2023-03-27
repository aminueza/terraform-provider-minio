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

func minioCreateUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	var userName = d.Get("user_name").(string)
	var policyName = d.Get("policy_name").(string)
	minioAdmin := meta.(*S3MinioClient).S3Admin
	err := minioAdmin.SetPolicy(ctx, policyName, userName, false)
	if err != nil {
		return NewResourceError("unable to Set User policy", userName+" "+policyName, err)
	}

	d.SetId(resource.PrefixedUniqueId(fmt.Sprintf("%s-", userName)))

	return minioReadUserPolicyAttachment(ctx, d, meta)
}

func minioReadUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin
	var userName = d.Get("user_name").(string)
	var isLDAPUser = LDAPUserDistinguishedNamePattern.MatchString(userName)

	userInfo, errUser := minioAdmin.GetUserInfo(ctx, userName)
	if errUser != nil && !isLDAPUser {
		return NewResourceError("failed to load user Infos", userName, errUser)
	}

	if userInfo.PolicyName == "" {
		log.Printf("[WARN] No such policy by name (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err := d.Set("policy_name", string(userInfo.PolicyName)); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func minioDeleteUserPolicyAttachment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin
	var userName = d.Get("user_name").(string)

	errIam := minioAdmin.SetPolicy(ctx, "", userName, false)
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
	d.SetId(resource.PrefixedUniqueId(fmt.Sprintf("%s-", userName)))
	return []*schema.ResourceData{d}, nil
}
