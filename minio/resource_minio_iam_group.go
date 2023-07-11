package minio

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go"
)

var (
	LDAPGroupDistinguishedNamePattern = regexp.MustCompile(`^(?:((?:(?:CN|OU)=[^,]+,?)+),)+((?:DC=[^,]+,?)+)$`)
	StaticGroupNamePattern            = regexp.MustCompile(`^[0-9A-Za-z=,.@\-_+]+$`)
)

func resourceMinioIAMGroup() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateGroup,
		ReadContext:   minioReadGroup,
		UpdateContext: minioUpdateGroup,
		DeleteContext: minioDeleteGroup,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
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

func minioCreateGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamGroupConfig := IAMGroupConfig(d, meta)

	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupConfig.MinioIAMName,
		Members:  []string{},
		IsRemove: false,
	}

	err := iamGroupConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		return NewResourceError("creating group failed", d.Id(), err)
	}

	err = minioStatusGroup(ctx, d, meta)
	if err != nil {
		return NewResourceError("error updating IAM Group %s: %s", d.Id(), err)
	}

	d.SetId(aws.StringValue(&iamGroupConfig.MinioIAMName))

	return minioReadGroup(ctx, d, meta)
}

func minioUpdateGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamGroupConfig := IAMGroupConfig(d, meta)

	if d.HasChange(iamGroupConfig.MinioIAMName) {
		_, nn := d.GetChange(iamGroupConfig.MinioIAMName)

		log.Println("[DEBUG] Update IAM Group:", iamGroupConfig.MinioIAMName)

		groupAddRemove := madmin.GroupAddRemove{
			Group:    iamGroupConfig.MinioIAMName,
			Members:  []string{},
			IsRemove: false,
		}

		err := iamGroupConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove)
		if err != nil {
			return NewResourceError("error updating IAM Group %s: %s", d.Id(), err)
		}

		d.SetId(nn.(string))
	}

	err := minioStatusGroup(ctx, d, meta)
	if err != nil {
		return NewResourceError("error updating IAM Group %s: %s", d.Id(), err)
	}

	if iamGroupConfig.MinioForceDestroy {
		err := minioDeleteGroup(ctx, d, meta)
		if err != nil {
			return err
		}
	}

	return minioReadGroup(ctx, d, meta)
}

func minioReadGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamGroupConfig := IAMGroupConfig(d, meta)

	output, err := iamGroupConfig.MinioAdmin.GetGroupDescription(ctx, d.Id())
	if err != nil {
		if strings.Contains(err.Error(), "group does not exist") {
			log.Printf("[WARN] No IAM group by name (%s) found, removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return NewResourceError("error reading IAM Group %s: %s", d.Id(), err)
	}

	log.Printf("[WARN] (%v)", output)

	if err := d.Set("group_name", string(output.Name)); err != nil {
		return NewResourceError("error reading IAM Group %s: %s", d.Id(), err)
	}

	if err := d.Set("disable_group", output.Status == string(madmin.GroupDisabled)); err != nil {
		return NewResourceError("error reading IAM Group %s: %s", d.Id(), err)
	}

	return nil
}

func minioDeleteGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamGroupConfig := IAMGroupConfig(d, meta)

	log.Printf("[DEBUG] Checking if IAM Group %s is empty:", d.Id())
	groupDesc, err := iamGroupConfig.MinioAdmin.GetGroupDescription(ctx, d.Id())
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			log.Printf("[WARN] No IAM group by name (%s) found, removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return NewResourceError("error reading IAM Group %s: %s", d.Id(), err)
	}

	if groupDesc.Name == "" {
		return nil
	}

	if len(groupDesc.Policy) == 0 {
		//delete group requires to set policy if it doesn't exist
		_ = iamGroupConfig.MinioAdmin.SetPolicy(ctx, "readonly", d.Id(), true)

	}

	//force to delete group even if group isn't empty
	if iamGroupConfig.MinioForceDestroy {
		err := deleteMinioGroup(ctx, iamGroupConfig, groupDesc.Members)

		if err != nil {
			return NewResourceError("error deleting IAM Group %s: %s", d.Id(), err)
		}

	}

	//Group must be empty to be deleted
	if len(groupDesc.Members) == 0 {
		err := deleteMinioGroup(ctx, iamGroupConfig, []string{})

		if err != nil {
			return NewResourceError("error deleting IAM Group %s: %s", d.Id(), err)
		}

	}

	return nil
}

func minioStatusGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) error {

	var minioGroupStatus madmin.GroupStatus

	iamGroupConfig := IAMGroupConfig(d, meta)

	log.Println("[DEBUG] Disabling IAM Group request:", iamGroupConfig.MinioIAMName)

	if iamGroupConfig.MinioDisableGroup {
		minioGroupStatus = madmin.GroupDisabled
	} else {
		minioGroupStatus = madmin.GroupEnabled
	}

	err := iamGroupConfig.MinioAdmin.SetGroupStatus(ctx, iamGroupConfig.MinioIAMName, minioGroupStatus)

	if err != nil {
		return fmt.Errorf("error while enabling or disabling IAM Group %s: %s", d.Id(), err)
	}

	return nil
}

func deleteMinioGroup(ctx context.Context, iamGroupConfig *S3MinioIAMGroupConfig, members []string) error {

	log.Println("[DEBUG] deleting IAM Group request:", iamGroupConfig.MinioIAMName)
	groupAddRemove := madmin.GroupAddRemove{
		Group:    iamGroupConfig.MinioIAMName,
		Members:  members,
		IsRemove: true,
	}

	err := iamGroupConfig.MinioAdmin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		return err
	}

	return nil
}

func validateMinioIamGroupName(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	if !StaticGroupNamePattern.MatchString(value) && !LDAPGroupDistinguishedNamePattern.MatchString(value) {
		errors = append(errors, fmt.Errorf(
			"only alphanumeric characters, hyphens, underscores, commas, periods, @ symbols, plus and equals signs allowed or a valid LDAP Distinguished Name (DN) in %q: %q",
			k, value))
	}
	return
}
