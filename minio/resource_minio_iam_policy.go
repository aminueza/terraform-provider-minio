package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
	awspolicy "github.com/jen20/awspolicyequivalence"
)

func resourceMinioIAMPolicy() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreatePolicy,
		ReadContext:   minioReadPolicy,
		UpdateContext: minioUpdatePolicy,
		DeleteContext: minioDeletePolicy,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
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

func minioCreatePolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

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

	err := iamPolicyConfig.MinioAdmin.AddCannedPolicy(ctx, name, ParseIamPolicyConfigFromString(iamPolicyConfig.MinioIAMPolicy))
	if err != nil {
		return NewResourceError("Unable to create policy", name, err)
	}

	d.SetId(aws.StringValue(&name))

	return minioReadPolicy(ctx, d, meta)
}

func minioReadPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamPolicyConfig := IAMPolicyConfig(d, meta)

	log.Printf("[DEBUG] Getting IAM Policy: %s", d.Id())

	output, err := iamPolicyConfig.MinioAdmin.InfoCannedPolicy(ctx, string(d.Id()))
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
		return diag.FromErr(err)
	}

	outputAsJSON, err := json.Marshal(&output)
	if err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("policy", string(outputAsJSON)); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func minioUpdatePolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamPolicyConfig := IAMPolicyConfig(d, meta)

	log.Println("[DEBUG] Update IAM Policy:", string(d.Id()))

	err := iamPolicyConfig.MinioAdmin.AddCannedPolicy(ctx, string(d.Id()), ParseIamPolicyConfigFromString(iamPolicyConfig.MinioIAMPolicy))
	if err != nil {
		return NewResourceError("Unable to update policy", string(d.Id()), err)
	}

	return minioReadPolicy(ctx, d, meta)
}

func minioDeletePolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamPolicyConfig := IAMPolicyConfig(d, meta)

	err := iamPolicyConfig.MinioAdmin.RemoveCannedPolicy(ctx, string(d.Id()))
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
