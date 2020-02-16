package minio

import (
	"fmt"
	"log"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/structure"
	awspolicy "github.com/jen20/awspolicyequivalence"
)

func resourceMinioIAMPolicy() *schema.Resource {
	return &schema.Resource{
		Create: minioCreatePolicy,
		Read:   minioReadPolicy,
		Update: minioUpdatePolicy,
		Delete: minioDeletePolicy,
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
		},
	}
}

func minioCreatePolicy(d *schema.ResourceData, meta interface{}) error {

	var name string

	iamPolicyConfig := IAMPolicyConfig(d, meta)

	if len(iamPolicyConfig.MinioIAMName) > 0 {
		name = iamPolicyConfig.MinioIAMName
	} else if len(iamPolicyConfig.MinioIAMNamePrefix) > 0 {
		name = resource.PrefixedUniqueId(iamPolicyConfig.MinioIAMNamePrefix)
	} else {
		name = resource.UniqueId()
	}

	log.Printf("[DEBUG] Creating IAM Policy %s: %v", name, iamPolicyConfig.MinioIAMPolicy)

	err := iamPolicyConfig.MinioAdmin.AddCannedPolicy(name, iamPolicyConfig.MinioIAMPolicy)
	if err != nil {
		return NewResourceError("Unable to create policy", name, err)
	}

	d.SetId(aws.StringValue(&name))

	return minioReadPolicy(d, meta)
}

func minioReadPolicy(d *schema.ResourceData, meta interface{}) error {

	iamPolicyConfig := IAMPolicyConfig(d, meta)

	log.Printf("[DEBUG] Getting IAM Policy: %s", d.Id())

	output, err := iamPolicyConfig.MinioAdmin.InfoCannedPolicy(string(d.Id()))
	if err != nil {
		return NewResourceError("Unable to read policy", d.Id(), err)
	}

	log.Printf("[WARN] (%v)", output)

	if &output == nil {
		log.Printf("[WARN] No IAM policy by name (%s) found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err := d.Set("name", string(d.Id())); err != nil {
		return err
	}

	if err := d.Set("policy", string(output)); err != nil {
		return err
	}

	return nil
}

func minioUpdatePolicy(d *schema.ResourceData, meta interface{}) error {

	var on interface{}
	var nn interface{}
	var name string

	iamPolicyConfig := IAMPolicyConfig(d, meta)

	if len(iamPolicyConfig.MinioIAMName) > 0 {
		name = iamPolicyConfig.MinioIAMName
	} else if len(iamPolicyConfig.MinioIAMNamePrefix) > 0 {
		name = resource.PrefixedUniqueId(iamPolicyConfig.MinioIAMNamePrefix)
	}

	if d.HasChange(name) {
		on, nn = d.GetChange(name)
	} else if d.HasChange(iamPolicyConfig.MinioIAMPolicy) {
		on, nn = d.GetChange(iamPolicyConfig.MinioIAMPolicy)
	}
	
	if on == nil && nn == nil {
		return minioReadPolicy(d, meta)
	}

	if len(on.(string)) > 0 && len(nn.(string)) > 0 {
		log.Println("[DEBUG] Update IAM Policy:", iamPolicyConfig.MinioIAMName)
		err := iamPolicyConfig.MinioAdmin.RemoveCannedPolicy(on.(string))
		if err != nil {
			return NewResourceError("Unable to update policy", name, err)
		}

		err = iamPolicyConfig.MinioAdmin.AddCannedPolicy(nn.(string), string(iamPolicyConfig.MinioIAMPolicy))
		if err != nil {
			return NewResourceError("Unable to update policy", name, err)
		}

		d.SetId(nn.(string))

	}

	return minioReadPolicy(d, meta)
}

func minioDeletePolicy(d *schema.ResourceData, meta interface{}) error {
	iamPolicyConfig := IAMPolicyConfig(d, meta)

	err := iamPolicyConfig.MinioAdmin.RemoveCannedPolicy(string(d.Id()))
	if err != nil {
		return NewResourceError("Unable to delete policy", d.Id(), err)
	}

	_ = d.Set("policy", "")

	return nil
}

func validateIAMNamePolicy(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	if len(value) > 128 {
		errors = append(errors, fmt.Errorf(
			"%q cannot be longer than 128 characters", k))
	}

	if len(value) > 96 {
		errors = append(errors, fmt.Errorf(
			"%q cannot be longer than 96 characters, name is limited to 128", k))
	}

	if !regexp.MustCompile(`^[\w+=,.@-]*$`).MatchString(value) {
		errors = append(errors, fmt.Errorf(
			"%q must match [\\w+=,.@-]", k))
	}
	return
}

func validateIAMPolicyJSON(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	if len(value) < 1 {
		errors = append(errors, fmt.Errorf("%q contains an invalid JSON policy", k))
		return
	}
	if value[:1] != "{" {
		errors = append(errors, fmt.Errorf("%q contains an invalid JSON policy", k))
		return
	}
	if _, err := structure.NormalizeJsonString(v); err != nil {
		errors = append(errors, fmt.Errorf("%q contains an invalid JSON: %s", k, err))
	}
	return
}

func suppressEquivalentAwsPolicyDiffs(k, old, new string, d *schema.ResourceData) bool {
	equivalent, err := awspolicy.PoliciesAreEquivalent(old, new)
	if err != nil {
		return false
	}

	return equivalent
}
