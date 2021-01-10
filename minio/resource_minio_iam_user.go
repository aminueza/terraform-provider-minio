package minio

import (
	"context"
	"fmt"
	"log"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/minio/minio/pkg/madmin"
)

func resourceMinioIAMUser() *schema.Resource {
	return &schema.Resource{
		Create: minioCreateUser,
		Read:   minioReadUser,
		Update: minioUpdateUser,
		Delete: minioDeleteUser,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
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

func minioCreateUser(d *schema.ResourceData, meta interface{}) error {

	iamUserConfig := IAMUserConfig(d, meta)

	accessKey := iamUserConfig.MinioIAMName
	secretKey, _ := generateSecretAccessKey()

	err := iamUserConfig.MinioAdmin.AddUser(context.Background(), string(accessKey), string(secretKey))
	if err != nil {
		return err
	}

	d.SetId(aws.StringValue(&accessKey))
	_ = d.Set("secret", string(secretKey))
	return minioReadUser(d, meta)
}

func minioUpdateUser(d *schema.ResourceData, meta interface{}) error {

	iamUserConfig := IAMUserConfig(d, meta)

	secretKey, _ := generateSecretAccessKey()

	if d.HasChange(iamUserConfig.MinioIAMName) {
		on, nn := d.GetChange(iamUserConfig.MinioIAMName)

		log.Println("[DEBUG] Update IAM User:", iamUserConfig.MinioIAMName)
		err := iamUserConfig.MinioAdmin.RemoveUser(context.Background(), on.(string))
		if err != nil {
			return fmt.Errorf("Error updating IAM User %s: %s", d.Id(), err)
		}

		err = iamUserConfig.MinioAdmin.AddUser(context.Background(), nn.(string), string(secretKey))
		if err != nil {
			return fmt.Errorf("Error updating IAM User %s: %s", d.Id(), err)
		}

		d.SetId(nn.(string))
	}

	userStatus := UserStatus{
		AccessKey: iamUserConfig.MinioIAMName,
		SecretKey: string(secretKey),
		Status:    madmin.AccountStatus(statusUser(false)),
	}

	output, _ := iamUserConfig.MinioAdmin.GetUserInfo(context.Background(), iamUserConfig.MinioIAMName)

	if iamUserConfig.MinioDisableUser || output.Status == madmin.AccountStatus(statusUser(false)) && !iamUserConfig.MinioForceDestroy {
		userStatus.Status = madmin.AccountStatus(statusUser(true))
	} else if output.Status == madmin.AccountStatus(statusUser(true)) && !iamUserConfig.MinioForceDestroy {
		userStatus.Status = madmin.AccountStatus(statusUser(false))
	}

	err := iamUserConfig.MinioAdmin.SetUserStatus(context.Background(), userStatus.AccessKey, userStatus.Status)
	if err != nil {
		return fmt.Errorf("Error to disable IAM User %s: %s", d.Id(), err)
	}

	if iamUserConfig.MinioUpdateKey {
		err := iamUserConfig.MinioAdmin.SetUser(context.Background(), userStatus.AccessKey, userStatus.SecretKey, userStatus.Status)
		if err != nil {
			return fmt.Errorf("Error updating IAM User Key %s: %s", d.Id(), err)
		}
	}

	if iamUserConfig.MinioForceDestroy {
		_ = minioDeleteUser(d, meta)
	}

	return minioReadUser(d, meta)
}

func minioReadUser(d *schema.ResourceData, meta interface{}) error {

	iamUserConfig := IAMUserConfig(d, meta)

	output, err := iamUserConfig.MinioAdmin.GetUserInfo(context.Background(), d.Id())
	if err != nil {
		return fmt.Errorf("Error reading IAM User %s: %s", d.Id(), err)
	}

	log.Printf("[WARN] (%v)", output)

	if _, ok := d.GetOk("name"); !ok {
		_ = d.Set("name", d.Id())
	}

	if err := d.Set("status", string(output.Status)); err != nil {
		return err
	}

	if &output == nil {
		log.Printf("[WARN] No IAM user by name (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}
	return nil
}

func minioDeleteUser(d *schema.ResourceData, meta interface{}) error {

	iamUserConfig := IAMUserConfig(d, meta)

	// IAM Users must be removed from all groups before they can be deleted
	if err := deleteMinioIamUserGroupMemberships(iamUserConfig); err != nil {
		return fmt.Errorf("Error removing IAM User (%s) group memberships: %s", d.Id(), err)
	}

	err := deleteMinioIamUser(iamUserConfig)
	if err != nil {
		return fmt.Errorf("Error deleting IAM User %s: %s", d.Id(), err)
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

func deleteMinioIamUser(iamUserConfig *S3MinioIAMUserConfig) error {
	log.Println("[DEBUG] Deleting IAM User request:", iamUserConfig.MinioIAMName)
	err := iamUserConfig.MinioAdmin.RemoveUser(context.Background(), iamUserConfig.MinioIAMName)
	if err != nil {
		return err
	}
	return nil
}

func deleteMinioIamUserGroupMemberships(iamUserConfig *S3MinioIAMUserConfig) error {

	userInfo, _ := iamUserConfig.MinioAdmin.GetUserInfo(context.Background(), iamUserConfig.MinioIAMName)

	groupsMemberOf := userInfo.MemberOf

	for _, groupMemberOf := range groupsMemberOf {

		log.Printf("[DEBUG] Removing IAM User %s from IAM Group %s", iamUserConfig.MinioIAMName, groupMemberOf)
		groupAddRemove := madmin.GroupAddRemove{
			Group:    groupMemberOf,
			Members:  []string{iamUserConfig.MinioIAMName},
			IsRemove: true,
		}

		err := iamUserConfig.MinioAdmin.UpdateGroupMembers(context.Background(), groupAddRemove)
		if err != nil {
			return err
		}

	}

	return nil

}
