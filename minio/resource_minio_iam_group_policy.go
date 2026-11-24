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
				ValidateFunc: validateMinioIamStaticGroupName,
			},
		},
	}
}

// validateMinioIamStaticGroupName is validateMinioIamGroupName without the
// LDAP DN alternative: this resource manages built-in MinIO groups, and DN
// values can contain colons, which would break the
// `<group-name>:<policy-name>` import ID.
func validateMinioIamStaticGroupName(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	if !StaticGroupNamePattern.MatchString(value) {
		errors = append(errors, fmt.Errorf(
			"only alphanumeric characters, hyphens, underscores, commas, periods, @ symbols, plus and equals signs allowed in %q: %q (LDAP Distinguished Names are not supported by this resource)",
			k, value))
	}
	return
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

	// Lock the policy name before the group lock everywhere this pair is taken.
	iamPolicyCreateLock.Lock(name)
	defer iamPolicyCreateLock.Unlock(name)

	// AddCannedPolicy overwrites any existing canned policy (including built-ins
	// or policies managed outside this resource), so fail instead of taking over.
	_, preCheckErr := iAMGroupPolicyConfig.MinioAdmin.InfoCannedPolicy(ctx, name)
	if preCheckErr == nil {
		return NewResourceError(
			"unable to create group policy",
			name,
			groupPolicyExistsError(ctx, iAMGroupPolicyConfig.MinioAdmin, groupName, name),
		)
	}
	if !isNoSuchPolicyError(preCheckErr) {
		return NewResourceError("checking for existing IAM policy", name, preCheckErr)
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
				return NewResourceError("unable to attach group policy", groupName+" "+name, fmt.Errorf("%v; and cleaning up the canned policy %q also failed, remove it manually: %v", attachErr, name, rmErr))
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
			if !groupPolicyNameLooksGenerated(policyName) {
				tflog.Warn(ctx, fmt.Sprintf("IAM group policy (%s) is not attached to group (%s), removing from state", policyName, groupName))
				d.SetId("")
				return nil
			}
			// The name was generated by Create, so dropping the resource would
			// leak the canned policy: the next create picks a new name and the
			// old policy is nobody's resource anymore. Keep the resource and
			// tell the user how to reattach.
			return diag.Diagnostics{diag.Diagnostic{
				Severity: diag.Warning,
				Summary:  fmt.Sprintf("canned policy %q exists but is not attached to group %q; recreate this resource (terraform apply -replace=<address>) to attach a fresh policy, or attach %q to %q manually; the policy is kept to avoid deleting a policy Terraform may not own", policyName, groupName, policyName, groupName),
			}}
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

// groupPolicyNameLooksGenerated reports whether the policy name ends with
// Terraform's 26-character UniqueId suffix (18 decimal digits + 8 lowercase
// hex), which is what Create produces for `name_prefix` (id.PrefixedUniqueId)
// and for no name at all (id.UniqueId). The refresh RPC carries no config, so
// GetRawConfig is always null in Read and the suffix is the only durable trace
// of generation. An explicit name that happens to match is treated as
// generated: keeping the resource with a warning instead of dropping it is the
// safe direction, since dropping a generated name leaks the canned policy.
func groupPolicyNameLooksGenerated(name string) bool {
	if len(name) < id.UniqueIDSuffixLength {
		return false
	}
	return iamPolicyUniqueSuffixPattern.MatchString(name[len(name)-id.UniqueIDSuffixLength:])
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

// groupPolicyExistsError explains that a canned policy blocks creating this
// group policy. The import hint only works while the policy is attached to the
// group: Read drops the resource when the policy is not attached, so an
// unattached policy needs attaching first or a different name.
func groupPolicyExistsError(ctx context.Context, minioAdmin *madmin.AdminClient, groupName, name string) error {
	policies, err := minioReadGroupPolicies(ctx, minioAdmin, groupName)
	if err == nil && !Contains(policies, name) {
		return fmt.Errorf("canned policy %q already exists but is not attached to group %q. Attach it to the group first and import it: terraform import minio_iam_group_policy.<name> %s:%s, or choose another name", name, groupName, groupName, name)
	}
	return alreadyExistsError("minio_iam_group_policy", name, fmt.Sprintf("%s:%s", groupName, name))
}

// isNoSuchUserError reports whether a madmin call failed because the user
// does not exist.
func isNoSuchUserError(err error) bool {
	return isMadminErrorCode(err, "XMinioAdminNoSuchUser")
}
