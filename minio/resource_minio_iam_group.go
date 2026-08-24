package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
)

var (
	LDAPGroupDistinguishedNamePattern = regexp.MustCompile(`^(?:((?:(?:CN|cn|OU|ou)=[^,]+,?)+),)+((?:(?:DC|dc)=[^,]+,?)+)$`)
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
				Description:  "Name of the group",
				Required:     true,
				ValidateFunc: validateMinioIamGroupName,
			},
			"force_destroy": {
				Type:        schema.TypeBool,
				Description: "Delete group even if it has non-Terraform-managed members",
				Optional:    true,
				Default:     false,
			},
			"group_name": {
				Type:        schema.TypeString,
				Description: "The name of the group.",
				Computed:    true,
			},
			"disable_group": {
				Type:        schema.TypeBool,
				Description: "Disable group",
				Optional:    true,
				Default:     false,
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

	d.SetId(iamGroupConfig.MinioIAMName)

	return minioReadGroup(ctx, d, meta)
}

func minioUpdateGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamGroupConfig := IAMGroupConfig(d, meta)

	if d.HasChange(iamGroupConfig.MinioIAMName) {
		_, nn := d.GetChange(iamGroupConfig.MinioIAMName)

		tflog.Debug(ctx, "Updating IAM group", map[string]interface{}{"name": iamGroupConfig.MinioIAMName})

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

	return minioReadGroup(ctx, d, meta)
}

func minioReadGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamGroupConfig := IAMGroupConfig(d, meta)

	output, err := iamGroupConfig.MinioAdmin.GetGroupDescription(ctx, d.Id())
	if err != nil {
		if strings.Contains(err.Error(), "group does not exist") {
			tflog.Warn(ctx, fmt.Sprintf("No IAM group by name (%s) found, removing from state", d.Id()))
			d.SetId("")
			return nil
		}
		return NewResourceError("error reading IAM Group %s: %s", d.Id(), err)
	}

	tflog.Warn(ctx, fmt.Sprintf("(%v)", output))

	if err := d.Set("group_name", output.Name); err != nil {
		return NewResourceError("error reading IAM Group %s: %s", d.Id(), err)
	}

	if err := d.Set("disable_group", output.Status == string(madmin.GroupDisabled)); err != nil {
		return NewResourceError("error reading IAM Group %s: %s", d.Id(), err)
	}

	return nil
}

func minioDeleteGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamGroupConfig := IAMGroupConfig(d, meta)

	tflog.Debug(ctx, fmt.Sprintf("Checking if IAM Group %s is empty:", d.Id()))
	groupDesc, err := iamGroupConfig.MinioAdmin.GetGroupDescription(ctx, d.Id())
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			tflog.Warn(ctx, fmt.Sprintf("No IAM group by name (%s) found, removing from state", d.Id()))
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
		_, _ = iamGroupConfig.MinioAdmin.AttachPolicy(ctx, madmin.PolicyAssociationReq{
			Policies: []string{"readonly"},
			Group:    d.Id(),
		})

	}

	//Group must be empty to be deleted
	if len(groupDesc.Members) != 0 {
		if iamGroupConfig.MinioForceDestroy {
			//force to delete group even if group isn't empty
			if err := deleteMinioGroup(ctx, iamGroupConfig, groupDesc.Members); err != nil {
				return NewResourceError("removing IAM group members", d.Id(), err)
			}
		} else {
			members, err := waitForGroupMembersToClear(ctx, func(ctx context.Context) ([]string, error) {
				desc, err := iamGroupConfig.MinioAdmin.GetGroupDescription(ctx, d.Id())
				if err != nil {
					return nil, err
				}
				return desc.Members, nil
			}, groupDrainAttempts, groupDrainDelay)
			if err != nil {
				return NewResourceError("reading IAM group members", d.Id(), err)
			}
			if len(members) != 0 {
				return NewResourceError("deleting IAM group", d.Id(),
					fmt.Errorf("group still has %d member(s); set force_destroy = true to delete a group with members", len(members)))
			}
		}
	}

	if err := deleteMinioGroup(ctx, iamGroupConfig, []string{}); err != nil {
		return NewResourceError("deleting IAM group", d.Id(), err)
	}

	return nil
}

// MinIO keeps serving a group's old member list for a short while after a
// minio_iam_group_membership removes them, and a group can only be deleted
// while it is empty. Re-read the membership for a few seconds so the delete
// doesn't give up on a list that is merely stale.
const (
	groupDrainAttempts = 6
	groupDrainDelay    = time.Second
)

// waitForGroupMembersToClear re-reads a group's members until the list comes
// back empty, and returns the last list it saw.
func waitForGroupMembersToClear(ctx context.Context, members func(context.Context) ([]string, error), attempts int, delay time.Duration) ([]string, error) {
	var last []string

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return last, ctx.Err()
			case <-time.After(delay):
			}
		}

		current, err := members(ctx)
		if err != nil {
			return last, err
		}
		if len(current) == 0 {
			return nil, nil
		}
		last = current
	}

	return last, nil
}

func minioStatusGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) error {

	var minioGroupStatus madmin.GroupStatus

	iamGroupConfig := IAMGroupConfig(d, meta)

	tflog.Debug(ctx, "Disabling IAM group", map[string]interface{}{"name": iamGroupConfig.MinioIAMName})

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

	tflog.Debug(ctx, "Deleting IAM group", map[string]interface{}{"name": iamGroupConfig.MinioIAMName})
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
