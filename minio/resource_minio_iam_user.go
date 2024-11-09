package minio

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

var (
	LDAPUserDistinguishedNamePattern = regexp.MustCompile(`^(?:((?:CN|cn)=([^,]*)),)+(?:((?:(?:CN|cn|OU|ou)=[^,]+,?)+),)+((?:(?:DC|dc)=[^,]+,?)+)$`)
	StaticUserNamePattern            = regexp.MustCompile(`^[0-9A-Za-z=,.@\-_+]+$`)
)

func resourceMinioIAMUser() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateUser,
		ReadContext:   minioReadUser,
		UpdateContext: minioUpdateUser,
		DeleteContext: minioDeleteUser,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Description:  "Name of the user",
				Required:     true,
				ValidateFunc: validateMinioIamUserName,
				ForceNew:     true,
			},
			"force_destroy": {
				Type:        schema.TypeBool,
				Description: "Delete user even if it has non-Terraform-managed IAM access keys",
				Optional:    true,
				Default:     false,
			},
			"disable_user": {
				Type:        schema.TypeBool,
				Description: "Disable user",
				Optional:    true,
				Default:     false,
			},
			"update_secret": {
				Type:        schema.TypeBool,
				Description: "Rotate Minio User Secret Key",
				Optional:    true,
				Default:     false,
			},
			"status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"secret": {
				Type:      schema.TypeString,
				Computed:  true,
				Optional:  true,
				Sensitive: true,
			},
			"tags": tagsSchema(),
		},
	}
}

func minioCreateUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamUserConfig := IAMUserConfig(d, meta)

	var err error
	accessKey := iamUserConfig.MinioIAMName
	secretKey := iamUserConfig.MinioSecret

	if secretKey == "" {
		if secretKey, err = generateSecretAccessKey(); err != nil {
			return NewResourceError("error creating user", accessKey, err)
		}
	}

	err = iamUserConfig.MinioAdmin.AddUser(ctx, accessKey, secretKey)
	if err != nil {
		return NewResourceError("error creating user", accessKey, err)
	}

	d.SetId(aws.StringValue(&accessKey))
	_ = d.Set("secret", secretKey)

	if iamUserConfig.MinioDisableUser {
		err = iamUserConfig.MinioAdmin.SetUserStatus(ctx, accessKey, madmin.AccountDisabled)
		if err != nil {
			return NewResourceError("error disabling IAM User %s: %s", d.Id(), err)
		}
	}

	return minioReadUser(ctx, d, meta)
}

func minioUpdateUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamUserConfig := IAMUserConfig(d, meta)

	wantedStatus := madmin.AccountEnabled
	if iamUserConfig.MinioDisableUser {
		wantedStatus = madmin.AccountDisabled
	}

	userServerInfo, _ := iamUserConfig.MinioAdmin.GetUserInfo(ctx, iamUserConfig.MinioIAMName)
	if userServerInfo.Status != wantedStatus {
		err := iamUserConfig.MinioAdmin.SetUserStatus(ctx, iamUserConfig.MinioIAMName, wantedStatus)
		if err != nil {
			return NewResourceError("error to disable IAM User %s: %s", d.Id(), err)
		}
	}

	wantedSecret := iamUserConfig.MinioSecret
	if iamUserConfig.MinioUpdateKey {
		if secretKey, err := generateSecretAccessKey(); err != nil {
			return NewResourceError("error creating user", d.Id(), err)
		} else {
			wantedSecret = secretKey
		}
	}

	if d.HasChange("secret") || iamUserConfig.MinioSecret != wantedSecret {
		err := iamUserConfig.MinioAdmin.SetUser(ctx, iamUserConfig.MinioIAMName, wantedSecret, wantedStatus)
		if err != nil {
			return NewResourceError("error updating IAM User Key %s: %s", d.Id(), err)
		}
		_ = d.Set("secret", wantedSecret)
	}

	return minioReadUser(ctx, d, meta)
}

func minioReadUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamUserConfig := IAMUserConfig(d, meta)

	output, err := iamUserConfig.MinioAdmin.GetUserInfo(ctx, d.Id())

	errResp := madmin.ErrorResponse{}

	if errors.As(err, &errResp) {
		if errResp.Code == "XMinioAdminNoSuchUser" {
			log.Printf("%s", NewResourceErrorStr("unable to find user", d.Id(), err))
			d.SetId("")
			return nil
		}
	}

	if err != nil {
		return NewResourceError("error reading IAM User", d.Id(), err)
	}

	log.Printf("[WARN] (%v)", output)

	if _, ok := d.GetOk("name"); !ok {
		_ = d.Set("name", d.Id())
	}

	if err := d.Set("status", string(output.Status)); err != nil {
		return NewResourceError("reading IAM user failed", d.Id(), err)
	}

	return nil
}

func minioDeleteUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamUserConfig := IAMUserConfig(d, meta)

	// IAM Users must be removed from all groups before they can be deleted
	if err := deleteMinioIamUserGroupMemberships(ctx, iamUserConfig); err != nil {
		if iamUserConfig.MinioForceDestroy {
			// Ignore errors when deleting group memberships, continue deleting user
		} else {
			return NewResourceError("error removing IAM User (%s) group memberships: %s", d.Id(), err)
		}
	}

	err := deleteMinioIamUser(ctx, iamUserConfig)
	if err != nil {
		return NewResourceError("error deleting IAM User", d.Id(), err)
	}

	// Actively set resource as deleted as the update path might force a deletion via MinioForceDestroy
	d.SetId("")

	return nil
}

func validateMinioIamUserName(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	if !StaticUserNamePattern.MatchString(value) && !LDAPUserDistinguishedNamePattern.MatchString(value) {
		errors = append(errors, fmt.Errorf(
			"only alphanumeric characters, hyphens, underscores, commas, periods, @ symbols, plus and equals signs allowed or a valid LDAP Distinguished Name (DN) in %q: %q",
			k, value))
	}
	return
}

func deleteMinioIamUser(ctx context.Context, iamUserConfig *S3MinioIAMUserConfig) error {
	log.Println("[DEBUG] Deleting IAM User request:", iamUserConfig.MinioIAMName)
	err := iamUserConfig.MinioAdmin.RemoveUser(ctx, iamUserConfig.MinioIAMName)
	if err != nil {
		return err
	}
	return nil
}

func deleteMinioIamUserGroupMemberships(ctx context.Context, iamUserConfig *S3MinioIAMUserConfig) error {

	userInfo, _ := iamUserConfig.MinioAdmin.GetUserInfo(ctx, iamUserConfig.MinioIAMName)

	groupsMemberOf := userInfo.MemberOf

	for _, groupMemberOf := range groupsMemberOf {

		log.Printf("[DEBUG] Removing IAM User %s from IAM Group %s", iamUserConfig.MinioIAMName, groupMemberOf)
		groupAddRemove := madmin.GroupAddRemove{
			Group:    groupMemberOf,
			Members:  []string{iamUserConfig.MinioIAMName},
			IsRemove: true,
		}

		err := iamUserConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove)
		if err != nil {
			return err
		}

	}

	return nil

}
