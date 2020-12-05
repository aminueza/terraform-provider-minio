package minio

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/minio/minio/pkg/madmin"
)

func resourceMinioIAMGroup() *schema.Resource {
	return &schema.Resource{
		Create: minioCreateGroup,
		Read:   minioReadGroup,
		Update: minioUpdateGroup,
		Delete: minioDeleteGroup,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateMinioIamGroupName,
			},
			"force_destroy": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Delete group even if it has non-Terraform-managed members",
			},
			"group_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"disable_group": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Disable group",
			},
		},
	}
}

func minioCreateGroup(d *schema.ResourceData, meta interface{}) error {

	iamGroupConfig := IAMGroupConfig(d, meta)

	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupConfig.MinioIAMName,
		Members:  []string{},
		IsRemove: false,
	}

	err := iamGroupConfig.MinioAdmin.UpdateGroupMembers(context.Background(), groupAddRemove)
	if err != nil {
		return err
	}

	d.SetId(aws.StringValue(&iamGroupConfig.MinioIAMName))

	return minioReadGroup(d, meta)
}

func minioUpdateGroup(d *schema.ResourceData, meta interface{}) error {

	iamGroupConfig := IAMGroupConfig(d, meta)

	if d.HasChange(iamGroupConfig.MinioIAMName) {
		_, nn := d.GetChange(iamGroupConfig.MinioIAMName)

		log.Println("[DEBUG] Update IAM Group:", iamGroupConfig.MinioIAMName)

		groupAddRemove := madmin.GroupAddRemove{
			Group:    iamGroupConfig.MinioIAMName,
			Members:  []string{},
			IsRemove: false,
		}

		err := iamGroupConfig.MinioAdmin.UpdateGroupMembers(context.Background(), groupAddRemove)
		if err != nil {
			return fmt.Errorf("Error updating IAM Group %s: %s", d.Id(), err)
		}

		d.SetId(nn.(string))
	}

	if iamGroupConfig.MinioDisableGroup {
		err := minioDisableGroup(d, meta)
		if err != nil {
			return err
		}
	}

	if iamGroupConfig.MinioForceDestroy {
		err := minioDeleteGroup(d, meta)
		if err != nil {
			return err
		}
	}

	return minioReadGroup(d, meta)
}

func minioReadGroup(d *schema.ResourceData, meta interface{}) error {

	iamGroupConfig := IAMGroupConfig(d, meta)

	output, err := iamGroupConfig.MinioAdmin.GetGroupDescription(context.Background(), iamGroupConfig.MinioIAMName)
	if err != nil {
		if strings.Contains(err.Error(), "group does not exist") {
			log.Printf("[WARN] No IAM group by name (%s) found, removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error reading IAM Group %s: %s", d.Id(), err)
	}

	log.Printf("[WARN] (%v)", output)

	if err := d.Set("group_name", string(output.Name)); err != nil {
		return err
	}

	return nil
}

func minioDeleteGroup(d *schema.ResourceData, meta interface{}) error {

	iamGroupConfig := IAMGroupConfig(d, meta)

	log.Printf("[DEBUG] Checking if IAM Group %s is empty:", d.Id())
	groupDesc, err := iamGroupConfig.MinioAdmin.GetGroupDescription(context.Background(), d.Id())
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			log.Printf("[WARN] No IAM group by name (%s) found, removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error reading IAM Group %s: %s", d.Id(), err)
	}

	if groupDesc.Name == "" {
		return nil
	}

	if len(groupDesc.Policy) == 0 {
		//delete group requires to set policy if it doesn't exist
		_ = iamGroupConfig.MinioAdmin.SetPolicy(context.Background(), "readonly", d.Id(), true)

	}

	//force to delete group even if group isn't empty
	if iamGroupConfig.MinioForceDestroy {
		err := deleteMinioGroup(iamGroupConfig, groupDesc.Members)

		if err != nil {
			return fmt.Errorf("Error deleting IAM Group %s: %s", d.Id(), err)
		}

	}

	//Group must be empty to be deleted
	if len(groupDesc.Members) == 0 {
		err := deleteMinioGroup(iamGroupConfig, []string{})

		if err != nil {
			return fmt.Errorf("Error deleting IAM Group %s: %s", d.Id(), err)
		}

	}

	return nil
}

func minioDisableGroup(d *schema.ResourceData, meta interface{}) error {

	iamGroupConfig := IAMGroupConfig(d, meta)

	log.Println("[DEBUG] Disabling IAM Group request:", iamGroupConfig.MinioIAMName)

	err := iamGroupConfig.MinioAdmin.SetGroupStatus(context.Background(), iamGroupConfig.MinioIAMName, madmin.GroupDisabled)

	if err != nil {
		return fmt.Errorf("Error disabling IAM Group %s: %s", d.Id(), err)
	}

	return nil
}

func deleteMinioGroup(iamGroupConfig *S3MinioIAMGroupConfig, members []string) error {

	log.Println("[DEBUG] deleting IAM Group request:", iamGroupConfig.MinioIAMName)
	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupConfig.MinioIAMName,
		Members:  members,
		IsRemove: true,
	}

	err := iamGroupConfig.MinioAdmin.UpdateGroupMembers(context.Background(), groupAddRemove)
	if err != nil {
		return err
	}

	return nil
}

func validateMinioIamGroupName(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	if !regexp.MustCompile(`^[0-9A-Za-z=,.@\-_+]+$`).MatchString(value) {
		errors = append(errors, fmt.Errorf(
			"only alphanumeric characters, hyphens, underscores, commas, periods, @ symbols, plus and equals signs allowed in %q: %q",
			k, value))
	}
	return
}
