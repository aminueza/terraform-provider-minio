package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
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
				Description:      "Policy JSON string",
				Required:         true,
				ValidateFunc:     validateIAMPolicyJSON,
				DiffSuppressFunc: suppressEquivalentAwsPolicyDiffs,
			},
			"name": {
				Type:          schema.TypeString,
				Description:   "Name of the policy. Conflicts with `name_prefix`.",
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
		},
	}
}

var iamPolicyCreateLock = NewMutexKV()

func minioCreatePolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	var name string

	iamPolicyConfig := IAMPolicyConfig(d, meta)

	if len(iamPolicyConfig.MinioIAMName) > 0 {
		name = iamPolicyConfig.MinioIAMName
	} else if len(iamPolicyConfig.MinioIAMNamePrefix) > 0 {
		name = id.PrefixedUniqueId(iamPolicyConfig.MinioIAMNamePrefix)
	} else {
		name = id.UniqueId()
	}

	iamPolicyCreateLock.Lock(name)
	defer iamPolicyCreateLock.Unlock(name)

	// AddCannedPolicy on MinIO overwrites an existing policy document,
	// including built-in policies.
	_, err := iamPolicyConfig.MinioAdmin.InfoCannedPolicy(ctx, name)
	if err == nil {
		return NewResourceError("unable to create policy", name, alreadyExistsError("minio_iam_policy", name, name))
	}
	if !isNoSuchPolicyError(err) {
		return NewResourceError("checking for existing IAM policy", name, err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Creating IAM Policy %s: %v", name, iamPolicyConfig.MinioIAMPolicy))

	err = iamPolicyConfig.MinioAdmin.AddCannedPolicy(ctx, name, []byte(iamPolicyConfig.MinioIAMPolicy))
	if err != nil {
		return NewResourceError("unable to create policy", name, err)
	}

	d.SetId(name)

	return minioReadPolicy(ctx, d, meta)
}

func minioReadPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	iamPolicyConfig := IAMPolicyConfig(d, meta)

	tflog.Debug(ctx, fmt.Sprintf("Getting IAM Policy: %s", d.Id()))

	info, err := iamPolicyConfig.MinioAdmin.InfoCannedPolicy(ctx, d.Id())
	if err != nil {
		if isNoSuchPolicyError(err) {
			tflog.Debug(ctx, fmt.Sprintf("IAM Policy does not exist: [%s]", d.Id()))
			d.SetId("")
			return nil
		}
		return NewResourceError("unable to read policy", d.Id(), err)
	}

	if err := d.Set("name", d.Id()); err != nil {
		return NewResourceError("setting policy name", d.Id(), err)
	}

	if err := d.Set("name_prefix", iamPolicyNamePrefixFromName(d.Id())); err != nil {
		return NewResourceError("setting policy name_prefix", d.Id(), err)
	}

	actualPolicyText := strings.TrimSpace(string(info.Policy))
	existingPolicy := ""
	if v, ok := d.GetOk("policy"); ok {
		existingPolicy = v.(string)
	}

	policy, err := NormalizeAndCompareJSONPolicies(existingPolicy, actualPolicyText)
	if err != nil {
		return NewResourceError("error while comparing policies", d.Id(), err)
	}

	if err := d.Set("policy", policy); err != nil {
		return NewResourceError("setting policy", d.Id(), err)
	}

	return nil
}

func minioUpdatePolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamPolicyConfig := IAMPolicyConfig(d, meta)

	tflog.Debug(ctx, "Updating IAM policy", map[string]interface{}{"id": d.Id()})

	err := iamPolicyConfig.MinioAdmin.AddCannedPolicy(ctx, d.Id(), []byte(iamPolicyConfig.MinioIAMPolicy))
	if err != nil {
		return NewResourceError("unable to update policy", d.Id(), err)
	}

	return minioReadPolicy(ctx, d, meta)
}

func minioDeletePolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	iamPolicyConfig := IAMPolicyConfig(d, meta)

	err := iamPolicyConfig.MinioAdmin.RemoveCannedPolicy(ctx, d.Id())
	if err != nil {
		return NewResourceError("unable to delete policy", d.Id(), err)
	}

	_ = d.Set("policy", "")

	return nil
}

const (
	maxIAMPolicyNameLength       = 255
	maxIAMPolicyNamePrefixLength = maxIAMPolicyNameLength - id.UniqueIDSuffixLength
)

var iamPolicyNamePattern = regexp.MustCompile(`^[\w+=.@:/-]*$`)

var iamPolicyUniqueSuffixPattern = regexp.MustCompile(`^\d{18}[0-9a-f]{8}$`)

// iamPolicyNamePrefixFromName recovers name_prefix when a policy name ends with
// Terraform's 26-character UniqueId suffix (18 decimal digits + 8 lowercase hex),
// returning "" otherwise so plain names import with an empty prefix.
func iamPolicyNamePrefixFromName(name string) string {
	if len(name) < id.UniqueIDSuffixLength {
		return ""
	}
	suffixStart := len(name) - id.UniqueIDSuffixLength
	if !iamPolicyUniqueSuffixPattern.MatchString(name[suffixStart:]) {
		return ""
	}
	return name[:suffixStart]
}

func validateIAMPolicyName(v interface{}, k string, maxLength int, limitDetail string) (ws []string, errors []error) {
	value := v.(string)
	if len(value) > maxLength {
		if limitDetail != "" {
			errors = append(errors, fmt.Errorf("%q cannot be longer than %d characters, %s", k, maxLength, limitDetail))
		} else {
			errors = append(errors, fmt.Errorf("%q cannot be longer than %d characters", k, maxLength))
		}
	}

	if !iamPolicyNamePattern.MatchString(value) {
		errors = append(errors, fmt.Errorf("%q must match [\\w+=.@:/-]", k))
	}
	return
}

func validateIAMNamePolicy(v interface{}, k string) (ws []string, errors []error) {
	return validateIAMPolicyName(v, k, maxIAMPolicyNameLength, "")
}

func validateIAMNamePrefixPolicy(v interface{}, k string) (ws []string, errors []error) {
	return validateIAMPolicyName(
		v,
		k,
		maxIAMPolicyNamePrefixLength,
		fmt.Sprintf("name is limited to %d", maxIAMPolicyNameLength),
	)
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
