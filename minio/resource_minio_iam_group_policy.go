package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioIAMGroupPolicy() *schema.Resource {
	return &schema.Resource{
		Create: minioCreateGroupPolicy,
		Read:   minioReadGroupPolicy,
		Update: minioUpdateGroupPolicy,
		Delete: minioDeleteGroupPolicy,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"policy": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validateIAMPolicyJSON,
				DiffSuppressFunc: suppressEquivalentAwsPolicyDiffs,
			},
			"name": {
				Type:          schema.TypeString,
				Optional:      true,
				Computed:      true,
				ForceNew:      true,
				ConflictsWith: []string{"name_prefix"},
				ValidateFunc:  validateIAMNamePolicy,
			},
			"name_prefix": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"name"},
				ValidateFunc:  validateIAMNamePolicy,
			},
			"group": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func minioCreateGroupPolicy(d *schema.ResourceData, meta interface{}) error {
	var name string

	iAMGroupPolicyConfig := IAMGroupPolicyConfig(d, meta)

	if policyName := iAMGroupPolicyConfig.MinioIAMName; policyName != "" {
		name = policyName
	} else if policyName := iAMGroupPolicyConfig.MinioIAMNamePrefix; policyName != "" {
		name = resource.PrefixedUniqueId(policyName)
	} else {
		name = resource.UniqueId()
	}

	log.Printf("[DEBUG] Creating IAM Group Policy %s: %v", name, iAMGroupPolicyConfig.MinioIAMPolicy)

	err := iAMGroupPolicyConfig.MinioAdmin.AddCannedPolicy(context.Background(), name, []byte(iAMGroupPolicyConfig.MinioIAMPolicy))
	if err != nil {
		return NewResourceError("Unable to create group policy", iAMGroupPolicyConfig.MinioIAMPolicy, err)
	}

	d.SetId(fmt.Sprintf("%s:%s", iAMGroupPolicyConfig.MinioIAMGroup, name))

	log.Printf("[DEBUG] Creating IAM Group Policy %s", d.Id())

	return minioReadGroupPolicy(d, meta)
}

func minioReadGroupPolicy(d *schema.ResourceData, meta interface{}) error {

	iAMGroupPolicyConfig := IAMGroupPolicyConfig(d, meta)

	groupName, policyName, err := resourceMinioIamGroupPolicyParseID(d.Id())
	if err != nil {
		return err
	}

	log.Printf("[DEBUG] Getting IAM Group Policy: %s", d.Id())

	output, err := iAMGroupPolicyConfig.MinioAdmin.InfoCannedPolicy(context.Background(), policyName)
	if output == nil {
		log.Printf("[WARN] No IAM group policy by name (%s) found, removing from state: %s", d.Id(), err)
		d.SetId("")
		return nil
	}

	outputAsJSON, err := json.Marshal(&output)
	if err != nil {
		return err
	}

	log.Printf("[WARN] (%v)", outputAsJSON)

	if err := d.Set("name", policyName); err != nil {
		return err
	}

	if err := d.Set("policy", string(outputAsJSON)); err != nil {
		return err
	}

	if err := d.Set("group", groupName); err != nil {
		return err
	}

	return nil
}

func minioUpdateGroupPolicy(d *schema.ResourceData, meta interface{}) error {

	var on interface{}
	var nn interface{}
	var name string

	iAMGroupPolicyConfig := IAMGroupPolicyConfig(d, meta)

	groupName, policyName, err := resourceMinioIamGroupPolicyParseID(d.Id())
	if err != nil {
		return err
	}

	if d.HasChange(policyName) {
		on, nn = d.GetChange(policyName)
	} else if d.HasChange(iAMGroupPolicyConfig.MinioIAMPolicy) {
		on, nn = d.GetChange(iAMGroupPolicyConfig.MinioIAMPolicy)
	}

	if on == nil && nn == nil {
		return minioReadGroupPolicy(d, meta)
	}

	if len(on.(string)) > 0 && len(nn.(string)) > 0 {
		log.Println("[DEBUG] Update IAM Group Policy:", policyName)
		err := iAMGroupPolicyConfig.MinioAdmin.RemoveCannedPolicy(context.Background(), on.(string))
		if err != nil {
			return NewResourceError("Unable to update group policy", name, err)
		}

		err = iAMGroupPolicyConfig.MinioAdmin.AddCannedPolicy(context.Background(), nn.(string), []byte(iAMGroupPolicyConfig.MinioIAMPolicy))
		if err != nil {
			return NewResourceError("Unable to update group policy", name, err)
		}

		d.SetId(fmt.Sprintf("%s:%s", groupName, policyName))

	}

	return minioReadGroupPolicy(d, meta)
}

func minioDeleteGroupPolicy(d *schema.ResourceData, meta interface{}) error {
	iamPolicyConfig := IAMPolicyConfig(d, meta)

	_, policyName, err := resourceMinioIamGroupPolicyParseID(d.Id())
	if err != nil {
		return err
	}

	policy, _ := iamPolicyConfig.MinioAdmin.InfoCannedPolicy(context.Background(), policyName)
	if policy == nil {
		return nil
	}

	err = iamPolicyConfig.MinioAdmin.RemoveCannedPolicy(context.Background(), policyName)
	if err != nil {
		return NewResourceError("Unable to delete group policy", d.Id(), err)
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
