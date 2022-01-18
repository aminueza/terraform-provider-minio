package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"log"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio/pkg/madmin"
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
				Required:     true,
				ValidateFunc: validateMinioIamUserName,
			},
			"force_destroy": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Delete user even if it has non-Terraform-managed IAM access keys",
			},
			"disable_user": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Disable user",
			},
			"update_secret": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Rotate Minio User Secret Key",
			},
			"status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"secret": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"tags": tagsSchema(),
		},
	}
}

func minioCreateUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamUserConfig := IAMUserConfig(d, meta)

	accessKey := iamUserConfig.MinioIAMName
	secretKey, _ := generateSecretAccessKey()

	err := iamUserConfig.MinioAdmin.AddUser(ctx, accessKey, string(secretKey))
	if err != nil {
		return NewResourceError("creating user failed", d.Id(), err)
	}

	d.SetId(aws.StringValue(&accessKey))
	_ = d.Set("secret", string(secretKey))
	return minioReadUser(ctx, d, meta)
}

func minioUpdateUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamUserConfig := IAMUserConfig(d, meta)
	secretKey := iamUserConfig.MinioSecret

	if secretKey == "" || iamUserConfig.MinioUpdateKey {
		secretKeyBytes, _ := generateSecretAccessKey()
		secretKey = string(secretKeyBytes)
	}

	if d.HasChange(iamUserConfig.MinioIAMName) {
		on, nn := d.GetChange(iamUserConfig.MinioIAMName)

		log.Println("[DEBUG] Update IAM User:", iamUserConfig.MinioIAMName)
		err := iamUserConfig.MinioAdmin.RemoveUser(ctx, on.(string))
		if err != nil {
			return NewResourceError("Error updating IAM User %s: %s", d.Id(), err)
		}

		err = iamUserConfig.MinioAdmin.AddUser(ctx, nn.(string), secretKey)
		if err != nil {
			return NewResourceError("Error updating IAM User %s: %s", d.Id(), err)
		}

		d.SetId(nn.(string))
	}

	userStatus := UserStatus{
		AccessKey: iamUserConfig.MinioIAMName,
		SecretKey: secretKey,
		Status:    madmin.AccountStatus(statusUser(false)),
	}

	output, _ := iamUserConfig.MinioAdmin.GetUserInfo(ctx, iamUserConfig.MinioIAMName)

	if iamUserConfig.MinioDisableUser || output.Status == madmin.AccountStatus(statusUser(false)) && !iamUserConfig.MinioForceDestroy {
		userStatus.Status = madmin.AccountStatus(statusUser(true))
	} else if output.Status == madmin.AccountStatus(statusUser(true)) && !iamUserConfig.MinioForceDestroy {
		userStatus.Status = madmin.AccountStatus(statusUser(false))
	}

	err := iamUserConfig.MinioAdmin.SetUserStatus(ctx, userStatus.AccessKey, userStatus.Status)
	if err != nil {
		return NewResourceError("Error to disable IAM User %s: %s", d.Id(), err)
	}

	if iamUserConfig.MinioUpdateKey {
		err := iamUserConfig.MinioAdmin.SetUser(ctx, userStatus.AccessKey, userStatus.SecretKey, userStatus.Status)
		if err != nil {
			return NewResourceError("Error updating IAM User Key %s: %s", d.Id(), err)
		}
	}

	if iamUserConfig.MinioForceDestroy {
		_ = minioDeleteUser(ctx, d, meta)
	}

	return minioReadUser(ctx, d, meta)
}

func minioReadUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamUserConfig := IAMUserConfig(d, meta)

	output, err := iamUserConfig.MinioAdmin.GetUserInfo(ctx, d.Id())
	if err != nil {
		return NewResourceError("Error reading IAM User %s: %s", d.Id(), err)
	}

	log.Printf("[WARN] (%v)", output)

	if _, ok := d.GetOk("name"); !ok {
		_ = d.Set("name", d.Id())
	}

	if err := d.Set("status", string(output.Status)); err != nil {
		return NewResourceError("reading IAM user failed", d.Id(), err)
	}

	if &output == nil {
		log.Printf("[WARN] No IAM user by name (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}
	return nil
}

func minioDeleteUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamUserConfig := IAMUserConfig(d, meta)

	// IAM Users must be removed from all groups before they can be deleted
	if err := deleteMinioIamUserGroupMemberships(ctx, iamUserConfig); err != nil {
		return NewResourceError("Error removing IAM User (%s) group memberships: %s", d.Id(), err)
	}

	err := deleteMinioIamUser(ctx, iamUserConfig)
	if err != nil {
		return NewResourceError("Error deleting IAM User %s: %s", d.Id(), err)
	}

	return nil
}

func statusUser(status bool) string {
	if status {
		return "disabled"
	}
	return "enabled"
}

func validateMinioIamUserName(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	if !regexp.MustCompile(`^[0-9A-Za-z=,.@\-_+]+$`).MatchString(value) {
		errors = append(errors, fmt.Errorf(
			"only alphanumeric characters, hyphens, underscores, commas, periods, @ symbols, plus and equals signs allowed in %q: %q",
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
