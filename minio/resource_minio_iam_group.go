package minio

import (
	"fmt"
	"log"
	"regexp"

	madmin "github.com/aminueza/terraform-minio-provider/madmin"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform/helper/schema"
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
				Description: "Delete group even if it has non-Terraform-managed IAM access keys",
			},
		},
	}
}

func minioCreateGroup(d *schema.ResourceData, meta interface{}) error {

	iamGroupConfig := IAMGroupConfig(d, meta)

	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupConfig.MinioIAMName,
		Members:  []string{""},
		IsRemove: false,
	}

	err := iamGroupConfig.MinioAdmin.UpdateGroupMembers(groupAddRemove)
	if err != nil {
		return err
	}

	d.SetId(aws.StringValue(&iamGroupConfig.MinioIAMName))

	return minioReadGroup(d, meta)
}

func minioUpdateGroup(d *schema.ResourceData, meta interface{}) error {

	iamGroupConfig := IAMGroupConfig(d, meta)

	if d.HasChange(iamGroupConfig.MinioIAMName) {
		on, nn := d.GetChange(iamGroupConfig.MinioIAMName)

		log.Println("[DEBUG] Update IAM Group:", iamGroupConfig.MinioIAMName)

		groupAddRemove := madmin.GroupAddRemove{
			Group:    iamGroupConfig.MinioIAMName,
			Members:  []string{on.(string)},
			IsRemove: false,
		}

		err := iamGroupConfig.MinioAdmin.UpdateGroupMembers(groupAddRemove)
		if err != nil {
			return fmt.Errorf("Error updating IAM Group %s: %s", d.Id(), err)
		}

		d.SetId(nn.(string))
	}

	if iamGroupConfig.MinioForceDestroy {
		minioDeleteGroup(d, meta)
	}

	return minioReadGroup(d, meta)
}

func minioReadGroup(d *schema.ResourceData, meta interface{}) error {

	iamGroupConfig := IAMGroupConfig(d, meta)

	output, err := iamGroupConfig.MinioAdmin.GetGroupDescription(iamGroupConfig.MinioIAMName)
	if err != nil {
		return fmt.Errorf("Error reading IAM Group %s: %s", d.Id(), err)
	}

	log.Printf("[WARN] (%v)", output)

	if err := d.Set("status", string(output.Status)); err != nil {
		return err
	}

	if &output == nil {
		log.Printf("[WARN] No IAM group by name (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}
	return nil
}

func minioDeleteGroup(d *schema.ResourceData, meta interface{}) error {

	iamGroupConfig := IAMGroupConfig(d, meta)

	if iamGroupConfig.MinioForceDestroy {
		log.Println("[DEBUG] deleting IAM Group request:", iamGroupConfig.MinioIAMName)

		groupAddRemove := madmin.GroupAddRemove{
			Group:    iamGroupConfig.MinioIAMName,
			IsRemove: true,
		}

		err := iamGroupConfig.MinioAdmin.UpdateGroupMembers(groupAddRemove)

		if err != nil {
			return fmt.Errorf("Error deleting IAM Group %s: %s", d.Id(), err)
		}
	}

	return nil
}

func minioDisableGroup(d *schema.ResourceData, meta interface{}) error {

	iamGroupConfig := IAMGroupConfig(d, meta)

	if iamGroupConfig.MinioForceDestroy {
		log.Println("[DEBUG] Disabling IAM Group request:", iamGroupConfig.MinioIAMName)
		err := iamGroupConfig.MinioAdmin.SetGroupStatus(iamGroupConfig.MinioIAMName, madmin.GroupDisabled)

		if err != nil {
			return fmt.Errorf("Error disabling IAM Group %s: %s", d.Id(), err)
		}
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
