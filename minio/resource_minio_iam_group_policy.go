package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioIAMGroupPolicy() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateGroupPolicy,
		ReadContext:   minioReadGroupPolicy,
		UpdateContext: minioUpdateGroupPolicy,
		DeleteContext: minioDeleteGroupPolicy,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
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
				ForceNew:      true,
				ConflictsWith: []string{"name"},
				ValidateFunc:  validateIAMNamePolicy,
			},
			"group": {
				Type:        schema.TypeString,
				Description: "Name of group the policy belongs to.",
				Required:    true,
				ForceNew:    true,
			},
		},
	}
}

func minioCreateGroupPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var name string

	iAMGroupPolicyConfig := IAMGroupPolicyConfig(d, meta)

	if policyName := iAMGroupPolicyConfig.MinioIAMName; policyName != "" {
		name = policyName
	} else if policyName := iAMGroupPolicyConfig.MinioIAMNamePrefix; policyName != "" {
		name = id.PrefixedUniqueId(policyName)
	} else {
		name = id.UniqueId()
	}

	log.Printf("[DEBUG] Creating IAM Group Policy %s: %v", name, iAMGroupPolicyConfig.MinioIAMPolicy)

	err := iAMGroupPolicyConfig.MinioAdmin.AddCannedPolicy(ctx, name, []byte(iAMGroupPolicyConfig.MinioIAMPolicy))
	if err != nil {
		return NewResourceError("unable to create group policy", iAMGroupPolicyConfig.MinioIAMPolicy, err)
	}

	d.SetId(fmt.Sprintf("%s:%s", iAMGroupPolicyConfig.MinioIAMGroup, name))

	log.Printf("[DEBUG] Creating IAM Group Policy %s", d.Id())

	return minioReadGroupPolicy(ctx, d, meta)
}

func minioReadGroupPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iAMGroupPolicyConfig := IAMGroupPolicyConfig(d, meta)

	groupName, policyName, err := resourceMinioIamGroupPolicyParseID(d.Id())
	if err != nil {
		return NewResourceError("[FATAL] Reading group policies failed", d.Id(), err)
	}

	log.Printf("[DEBUG] Getting IAM Group Policy: %s", d.Id())

	output, err := iAMGroupPolicyConfig.MinioAdmin.InfoCannedPolicy(ctx, policyName)
	if output == nil {
		log.Printf("[WARN] No IAM group policy by name (%s) found, removing from state: %s", d.Id(), err)
		d.SetId("")
		return nil
	}

	if err := d.Set("name", policyName); err != nil {
		return NewResourceError("[FATAL] Reading group policies failed", d.Id(), err)
	}

	if err := d.Set("policy", strings.TrimSpace(string(output))); err != nil {
		return NewResourceError("[FATAL] Reading group policies failed", d.Id(), err)
	}

	if err := d.Set("group", groupName); err != nil {
		return NewResourceError("[FATAL] Reading group policies failed", d.Id(), err)
	}

	return nil
}

func minioUpdateGroupPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	var on interface{}
	var nn interface{}
	var name string

	iAMGroupPolicyConfig := IAMGroupPolicyConfig(d, meta)

	groupName, policyName, err := resourceMinioIamGroupPolicyParseID(d.Id())
	if err != nil {
		return NewResourceError("[FATAL] Updating group policies failed", d.Id(), err)
	}

	if d.HasChange(policyName) {
		on, nn = d.GetChange(policyName)
	} else if d.HasChange(iAMGroupPolicyConfig.MinioIAMPolicy) {
		on, nn = d.GetChange(iAMGroupPolicyConfig.MinioIAMPolicy)
	}

	if on == nil && nn == nil {
		return minioReadGroupPolicy(ctx, d, meta)
	}

	if len(on.(string)) > 0 && len(nn.(string)) > 0 {
		log.Println("[DEBUG] Update IAM Group Policy:", policyName)
		err := iAMGroupPolicyConfig.MinioAdmin.RemoveCannedPolicy(ctx, on.(string))
		if err != nil {
			return NewResourceError("unable to update group policy", name, err)
		}

		err = iAMGroupPolicyConfig.MinioAdmin.AddCannedPolicy(ctx, nn.(string), []byte(iAMGroupPolicyConfig.MinioIAMPolicy))
		if err != nil {
			return NewResourceError("unable to update group policy", name, err)
		}

		d.SetId(fmt.Sprintf("%s:%s", groupName, policyName))

	}

	return minioReadGroupPolicy(ctx, d, meta)
}

func minioDeleteGroupPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamPolicyConfig := IAMPolicyConfig(d, meta)

	_, policyName, err := resourceMinioIamGroupPolicyParseID(d.Id())
	if err != nil {
		return NewResourceError("[FATAL] Reading group policies failed", d.Id(), err)
	}

	policy, _ := iamPolicyConfig.MinioAdmin.InfoCannedPolicy(ctx, policyName)
	if policy == nil {
		return nil
	}

	err = iamPolicyConfig.MinioAdmin.RemoveCannedPolicy(ctx, policyName)
	if err != nil {
		return NewResourceError("unable to delete group policy", d.Id(), err)
	}

	return nil
}

func resourceMinioIamGroupPolicyParseID(id string) (groupName, policyName string, err error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		err = fmt.Errorf("group_policy id must be of the form <group-name>:<policy-name> got %s:%s", parts[0], parts[1])
		return
	}

	groupName = parts[0]
	policyName = parts[1]
	return
}
