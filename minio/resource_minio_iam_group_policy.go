package minio

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
)

func resourceMinioIAMGroupPolicy() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateGroupPolicy,
		ReadContext:   minioReadGroupPolicy,
		UpdateContext: minioUpdateGroupPolicy,
		DeleteContext: minioDeleteGroupPolicy,
		Importer: &schema.ResourceImporter{
			StateContext: minioImportGroupPolicy,
		},

		Schema: map[string]*schema.Schema{
			"policy": {
				Type:             schema.TypeString,
				Description:      "Policy JSON string",
				Required:         true,
				ValidateFunc:     validateIAMPolicyJSON,
				DiffSuppressFunc: suppressEquivalentAwsPolicyDiffs,
			},
			"name": {
				Type:          schema.TypeString,
				Description:   "Name of the policy. If omitted, Terraform will assign a random, unique name.",
				Optional:      true,
				Computed:      true,
				ForceNew:      true,
				ConflictsWith: []string{"name_prefix"},
				ValidateFunc:  validateIAMNamePolicy,
			},
			"name_prefix": {
				Type:          schema.TypeString,
				Description:   "Prefix to the generated policy name. Do not use with `name`.",
				Optional:      true,
				Computed:      true,
				ForceNew:      true,
				ConflictsWith: []string{"name"},
				ValidateFunc:  validateIAMNamePrefixPolicy,
			},
			"group": {
				Type:         schema.TypeString,
				Description:  "Name of group the policy belongs to.",
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateMinioIamGroupName,
			},
		},
	}
}

func minioCreateGroupPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var name string

	iAMGroupPolicyConfig := IAMGroupPolicyConfig(d, meta)
	groupName := iAMGroupPolicyConfig.MinioIAMGroup

	if policyName := iAMGroupPolicyConfig.MinioIAMName; policyName != "" {
		name = policyName
	} else if policyName := iAMGroupPolicyConfig.MinioIAMNamePrefix; policyName != "" {
		name = id.PrefixedUniqueId(policyName)
	} else {
		name = id.UniqueId()
	}

	groupPolicyAttachmentLock.Lock(groupName)
	defer groupPolicyAttachmentLock.Unlock(groupName)

	policies, err := minioReadGroupPolicies(ctx, iAMGroupPolicyConfig.MinioAdmin, groupName)
	if err != nil {
		return NewResourceError("failed to load group infos", groupName, err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Creating IAM Group Policy %s: %v", name, iAMGroupPolicyConfig.MinioIAMPolicy))

	err = iAMGroupPolicyConfig.MinioAdmin.AddCannedPolicy(ctx, name, []byte(iAMGroupPolicyConfig.MinioIAMPolicy))
	if err != nil {
		return NewResourceError("unable to create group policy", name, err)
	}

	if !Contains(policies, name) {
		tflog.Debug(ctx, fmt.Sprintf("Attaching policy %s to group %s", name, groupName))
		policies = append(policies, name)
		if _, attachErr := iAMGroupPolicyConfig.MinioAdmin.AttachPolicy(ctx, madmin.PolicyAssociationReq{
			Policies: policies,
			Group:    groupName,
		}); attachErr != nil {
			if rmErr := iAMGroupPolicyConfig.MinioAdmin.RemoveCannedPolicy(ctx, name); rmErr != nil && !isNoSuchPolicyError(rmErr) {
				tflog.Warn(ctx, fmt.Sprintf("Failed cleaning up canned policy %s after attach failure: %v", name, rmErr))
			}
			return NewResourceError("unable to attach group policy", groupName+" "+name, attachErr)
		}
	}

	d.SetId(fmt.Sprintf("%s:%s", groupName, name))

	tflog.Debug(ctx, fmt.Sprintf("Creating IAM Group Policy %s", d.Id()))

	return doMinioReadGroupPolicy(ctx, d, meta, groupName, name)
}

func minioReadGroupPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	groupName, policyName, err := resourceMinioIamGroupPolicyParseID(d.Id())
	if err != nil {
		return NewResourceError("[FATAL] Reading group policies failed", d.Id(), err)
	}

	groupPolicyAttachmentLock.Lock(groupName)
	defer groupPolicyAttachmentLock.Unlock(groupName)

	return doMinioReadGroupPolicy(ctx, d, meta, groupName, policyName)
}

func doMinioReadGroupPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}, groupName, policyName string) diag.Diagnostics {
	iAMGroupPolicyConfig := IAMGroupPolicyConfig(d, meta)

	tflog.Debug(ctx, fmt.Sprintf("Getting IAM Group Policy: %s", d.Id()))

	info, err := iAMGroupPolicyConfig.MinioAdmin.InfoCannedPolicy(ctx, policyName)
	if err != nil {
		if isNoSuchPolicyError(err) && !d.IsNewResource() {
			tflog.Warn(ctx, fmt.Sprintf("No IAM group policy by name (%s) found, removing from state", d.Id()))
			d.SetId("")
			return nil
		}
		return NewResourceError("unable to read group policy", d.Id(), err)
	}

	policies, err := minioReadGroupPolicies(ctx, iAMGroupPolicyConfig.MinioAdmin, groupName)
	if err != nil {
		if isNoSuchGroupError(err) && !d.IsNewResource() {
			tflog.Warn(ctx, fmt.Sprintf("No such Group (%s) found, removing group policy (%s) from state", groupName, d.Id()))
			d.SetId("")
			return nil
		}
		return NewResourceError("failed to load group infos", groupName, err)
	}
	if !Contains(policies, policyName) {
		if !d.IsNewResource() {
			tflog.Warn(ctx, fmt.Sprintf("IAM group policy (%s) is not attached to group (%s), removing from state", policyName, groupName))
			d.SetId("")
			return nil
		}
		return NewResourceError("group policy not attached after create", d.Id(), fmt.Errorf("policy %q is not attached to group %q", policyName, groupName))
	}

	if err := d.Set("name", policyName); err != nil {
		return NewResourceError("[FATAL] Reading group policies failed", d.Id(), err)
	}

	if err := d.Set("name_prefix", iamPolicyNamePrefixFromName(policyName)); err != nil {
		return NewResourceError("[FATAL] Reading group policies failed", d.Id(), err)
	}

	if err := d.Set("policy", strings.TrimSpace(string(info.Policy))); err != nil {
		return NewResourceError("[FATAL] Reading group policies failed", d.Id(), err)
	}

	if err := d.Set("group", groupName); err != nil {
		return NewResourceError("[FATAL] Reading group policies failed", d.Id(), err)
	}

	return nil
}

func minioUpdateGroupPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iAMGroupPolicyConfig := IAMGroupPolicyConfig(d, meta)

	_, policyName, err := resourceMinioIamGroupPolicyParseID(d.Id())
	if err != nil {
		return NewResourceError("[FATAL] Updating group policies failed", d.Id(), err)
	}

	if d.HasChange("policy") {
		tflog.Debug(ctx, fmt.Sprintf("Updating IAM Group Policy %s content", policyName))

		err := iAMGroupPolicyConfig.MinioAdmin.AddCannedPolicy(ctx, policyName, []byte(iAMGroupPolicyConfig.MinioIAMPolicy))
		if err != nil {
			return NewResourceError("unable to update group policy", policyName, err)
		}
	}

	return minioReadGroupPolicy(ctx, d, meta)
}

func minioDeleteGroupPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamGroupPolicyConfig := IAMGroupPolicyConfig(d, meta)

	groupName, policyName, err := resourceMinioIamGroupPolicyParseID(d.Id())
	if err != nil {
		return NewResourceError("[FATAL] Reading group policies failed", d.Id(), err)
	}

	groupPolicyAttachmentLock.Lock(groupName)
	defer groupPolicyAttachmentLock.Unlock(groupName)

	policies, err := minioReadGroupPolicies(ctx, iamGroupPolicyConfig.MinioAdmin, groupName)
	if err != nil {
		if !isNoSuchGroupError(err) {
			return NewResourceError("failed to load group infos", groupName, err)
		}
	} else if Contains(policies, policyName) {
		tflog.Debug(ctx, fmt.Sprintf("Detaching policy %s from group: %s", policyName, groupName))
		if _, detachErr := iamGroupPolicyConfig.MinioAdmin.DetachPolicy(ctx, madmin.PolicyAssociationReq{
			Policies: []string{policyName},
			Group:    groupName,
		}); detachErr != nil && !isNoSuchGroupError(detachErr) {
			return NewResourceError("unable to detach group policy", d.Id(), detachErr)
		}
	}

	if _, err := iamGroupPolicyConfig.MinioAdmin.InfoCannedPolicy(ctx, policyName); err != nil {
		if isNoSuchPolicyError(err) {
			return nil
		}
		return NewResourceError("unable to read group policy before deleting it", d.Id(), err)
	}

	err = iamGroupPolicyConfig.MinioAdmin.RemoveCannedPolicy(ctx, policyName)
	if err != nil && !isNoSuchPolicyError(err) {
		return NewResourceError("unable to delete group policy", d.Id(), err)
	}

	return nil
}

func minioImportGroupPolicy(_ context.Context, d *schema.ResourceData, _ interface{}) ([]*schema.ResourceData, error) {
	if _, _, err := resourceMinioIamGroupPolicyParseID(d.Id()); err != nil {
		return nil, err
	}
	return []*schema.ResourceData{d}, nil
}

func resourceMinioIamGroupPolicyParseID(id string) (groupName, policyName string, err error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		err = fmt.Errorf("unexpected format of ID (%q), expected <group-name>:<policy-name>", id)
		return
	}

	groupName = parts[0]
	policyName = parts[1]
	return
}

func isMadminErrorCode(err error, code string) bool {
	var errResp madmin.ErrorResponse
	return errors.As(err, &errResp) && errResp.Code == code
}

// isNoSuchPolicyError reports whether a madmin call failed because the canned
// policy does not exist.
func isNoSuchPolicyError(err error) bool {
	return isMadminErrorCode(err, "XMinioAdminNoSuchPolicy")
}

// isNoSuchGroupError reports whether a madmin call failed because the group
// does not exist.
func isNoSuchGroupError(err error) bool {
	return isMadminErrorCode(err, "XMinioAdminNoSuchGroup")
}

// isNoSuchUserError reports whether a madmin call failed because the user
// does not exist.
func isNoSuchUserError(err error) bool {
	return isMadminErrorCode(err, "XMinioAdminNoSuchUser")
}
