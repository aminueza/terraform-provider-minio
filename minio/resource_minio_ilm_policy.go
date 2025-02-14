package minio

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

func resourceMinioILMPolicy() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateILMPolicy,
		ReadContext:   minioReadILMPolicy,
		DeleteContext: minioDeleteILMPolicy,
		UpdateContext: minioUpdateILMPolicy,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Description: "`minio_ilm_policy` handles lifecycle settings for a given `minio_s3_bucket`.",
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(0, 63),
			},
			"rule": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Required: true,
						},
						"status": {
							Type:         schema.TypeString,
							Optional:     true,
							Default:      "Enabled",
							ValidateFunc: validation.StringInSlice([]string{"Enabled", "Disabled"}, false),
							Description:  "Status of the rule. Can be either 'Enabled' or 'Disabled'. Defaults to 'Enabled'.",
						},
						"expiration": {
							Type:             schema.TypeString,
							Optional:         true,
							Description:      "Value may be duration (5d), date (1970-01-01), or \"DeleteMarker\" to expire delete markers if `noncurrent_version_expiration_days` is used",
							ValidateDiagFunc: validateILMExpiration,
						},
						"transition": {
							Type:     schema.TypeList,
							MaxItems: 1,
							Optional: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"days": {
										Type:             schema.TypeString,
										Optional:         true,
										ValidateDiagFunc: validateILMDays,
									},
									"date": {
										Type:             schema.TypeString,
										Optional:         true,
										ValidateDiagFunc: validateILMDate,
									},
									"storage_class": {
										Type:     schema.TypeString,
										Required: true,
									},
								},
							},
						},
						"noncurrent_transition": {
							Type:     schema.TypeList,
							MaxItems: 1,
							Optional: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"storage_class": {
										Type:     schema.TypeString,
										Required: true,
									},
									"days": {
										Type:             schema.TypeString,
										Required:         true,
										ValidateDiagFunc: validateILMDays,
									},
									"newer_versions": {
										Type:             schema.TypeInt,
										Optional:         true,
										ValidateDiagFunc: validateILMVersions,
									},
								},
							},
						},
						"noncurrent_expiration": {
							Type:     schema.TypeList,
							MaxItems: 1,
							Optional: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"days": {
										Type:             schema.TypeString,
										Required:         true,
										ValidateDiagFunc: validateILMDays,
									},
									"newer_versions": {
										Type:             schema.TypeInt,
										Optional:         true,
										ValidateDiagFunc: validateILMVersions,
									},
								},
							},
						},
						"filter": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"tags": {
							Type:     schema.TypeMap,
							Optional: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
		},
	}
}

func validateILMExpiration(v interface{}, p cty.Path) (errors diag.Diagnostics) {
	value := v.(string)
	exp := parseILMExpiration(value)

	if (lifecycle.Expiration{}) == exp {
		return diag.Errorf("expiration must be a duration (5d), date (1970-01-01), or \"DeleteMarker\"")
	}

	return
}

func validateILMDays(v interface{}, p cty.Path) diag.Diagnostics {
	value := v.(string)
	var days int
	if _, err := fmt.Sscanf(value, "%dd", &days); err != nil {
		return diag.Errorf("days must be in format '(number)d', got: %s", value)
	}
	if days < 1 {
		return diag.Errorf("days must be greater than 0, got: %d", days)
	}
	return nil
}

func validateILMDate(v interface{}, p cty.Path) diag.Diagnostics {
	value := v.(string)
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return diag.Errorf("date must be in format 'YYYY-MM-DD', got: %s", value)
	}
	return nil
}

func validateILMVersions(v interface{}, p cty.Path) diag.Diagnostics {
	value := v.(int)
	if value < 0 {
		return diag.Errorf("newer_versions must be non-negative, got: %d", value)
	}
	return nil
}

func minioCreateILMPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	_, err := c.BucketExists(ctx, bucket)
	if err != nil {
		return diag.FromErr(fmt.Errorf("bucket validation failed: %v", err))
	}

	oldConfig, err := c.GetBucketLifecycle(ctx, bucket)
	if err != nil && !isNotFoundError(err) {
		return diag.FromErr(fmt.Errorf("failed to get existing lifecycle: %v", err))
	}

	config := lifecycle.NewConfiguration()
	rules := d.Get("rule").([]interface{})

	for _, ruleI := range rules {
		rule, ok := ruleI.(map[string]interface{})
		if !ok {
			return diag.Errorf("invalid rule format")
		}

		lifecycleRule, err := createLifecycleRule(rule)
		if err != nil {
			return diag.FromErr(err)
		}

		config.Rules = append(config.Rules, lifecycleRule)
	}

	if err := c.SetBucketLifecycle(ctx, bucket, config); err != nil {
		if oldConfig != nil {
			if rbErr := c.SetBucketLifecycle(ctx, bucket, oldConfig); rbErr != nil {
				return diag.FromErr(fmt.Errorf("policy update failed and rollback failed: %v, rollback error: %v", err, rbErr))
			}
		}
		return diag.FromErr(fmt.Errorf("failed to set lifecycle: %v", err))
	}

	d.SetId(bucket)
	return minioReadILMPolicy(ctx, d, meta)
}

func createLifecycleRule(ruleData map[string]interface{}) (lifecycle.Rule, error) {
	id, ok := getStringValue(ruleData, "id")
	if !ok {
		return lifecycle.Rule{}, fmt.Errorf("rule id is required")
	}

	status, ok := getStringValue(ruleData, "status")
	if !ok {
		status = "Enabled"
	}

	if transition, exists := ruleData["transition"].([]interface{}); exists && len(transition) > 0 {
		t := transition[0].(map[string]interface{})
		if _, ok := t["storage_class"].(string); !ok {
			return lifecycle.Rule{}, fmt.Errorf("storage_class is required for transition")
		}
	}

	if nt, exists := ruleData["noncurrent_transition"].([]interface{}); exists && len(nt) > 0 {
		t := nt[0].(map[string]interface{})
		days, ok := getStringValue(t, "days")
		if !ok {
			return lifecycle.Rule{}, fmt.Errorf("days is required for noncurrent_transition")
		}
		if err := validateILMDays(days, nil); err != nil {
			return lifecycle.Rule{}, fmt.Errorf("invalid days format: %v", err)
		}
	}

	var filter lifecycle.Filter
	tags := convertToStringMap(ruleData["tags"])

	if len(tags) > 0 {
		prefix, _ := getStringValue(ruleData, "filter")
		filter.And.Prefix = prefix
		for k, v := range tags {
			filter.And.Tags = append(filter.And.Tags, lifecycle.Tag{Key: k, Value: v})
		}
	} else {
		prefix, _ := getStringValue(ruleData, "filter")
		filter.Prefix = prefix
	}

	expiration, _ := getStringValue(ruleData, "expiration")

	noncurrentTransition, err := parseILMNoncurrentTransition(ruleData["noncurrent_transition"])
	if err != nil {
		return lifecycle.Rule{}, err
	}

	return lifecycle.Rule{
		ID:                          id,
		Expiration:                  parseILMExpiration(expiration),
		Transition:                  parseILMTransition(ruleData["transition"]),
		NoncurrentVersionExpiration: parseILMNoncurrentExpiration(ruleData["noncurrent_expiration"]),
		NoncurrentVersionTransition: noncurrentTransition,
		Status:                      status,
		RuleFilter:                  filter,
	}, nil
}

func minioReadILMPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Client

	rules := make([]map[string]interface{}, 0)
	config, err := c.GetBucketLifecycle(ctx, d.Id())
	if err != nil {
		// TODO: distinguish between error and 404 not found
		log.Println(NewResourceErrorStr("reading lifecycle configuration failed", d.Id(), err))
		d.SetId("")
		return nil
	}

	if err = d.Set("bucket", d.Id()); err != nil {
		return NewResourceError("setting bucket failed", d.Id(), err)
	}

	for _, r := range config.Rules {
		var expiration string

		if r.Expiration.DeleteMarker {
			expiration = "DeleteMarker"
		} else if r.Expiration.Days != 0 {
			expiration = fmt.Sprintf("%dd", r.Expiration.Days)
		} else if !r.Expiration.IsNull() {
			expiration = r.Expiration.Date.Format("2006-01-02")
		}

		transitions := make([]map[string]interface{}, 0)
		if !r.Transition.IsNull() {
			transition := map[string]interface{}{}
			if !r.Transition.IsDaysNull() {
				transition["days"] = fmt.Sprintf("%dd", r.Transition.Days)
			} else if !r.Transition.IsDateNull() {
				transition["date"] = r.Transition.Date.Format("2006-01-02")
			}
			transition["storage_class"] = r.Transition.StorageClass
			transitions = append(transitions, transition)
		}

		noncurrentExpiration := make([]map[string]interface{}, 0)
		if r.NoncurrentVersionExpiration.NoncurrentDays != 0 {
			noncurrentExpiration = append(noncurrentExpiration, map[string]interface{}{
				"days":           fmt.Sprintf("%dd", r.NoncurrentVersionExpiration.NoncurrentDays),
				"newer_versions": r.NoncurrentVersionExpiration.NewerNoncurrentVersions,
			})
		}

		noncurrentTransition := make([]map[string]interface{}, 0)
		if r.NoncurrentVersionTransition.NoncurrentDays != 0 {
			noncurrentTransition = append(noncurrentTransition, map[string]interface{}{
				"days":           fmt.Sprintf("%dd", r.NoncurrentVersionTransition.NoncurrentDays),
				"storage_class":  r.NoncurrentVersionTransition.StorageClass,
				"newer_versions": r.NoncurrentVersionTransition.NewerNoncurrentVersions,
			})
		}

		var prefix string
		tags := map[string]string{}
		if len(r.RuleFilter.And.Tags) > 0 {
			prefix = r.RuleFilter.And.Prefix
			for _, tag := range r.RuleFilter.And.Tags {
				tags[tag.Key] = tag.Value
			}
		} else {
			prefix = r.RuleFilter.Prefix
		}

		rule := map[string]interface{}{
			"id":                    r.ID,
			"expiration":            expiration,
			"transition":            transitions,
			"noncurrent_expiration": noncurrentExpiration,
			"noncurrent_transition": noncurrentTransition,
			"status":                r.Status,
			"filter":                prefix,
			"tags":                  tags,
		}

		rules = append(rules, rule)
	}

	if err := d.Set("rule", rules); err != nil {
		return NewResourceError("reading lifecycle configuration failed", d.Id(), err)
	}

	return nil
}

func minioUpdateILMPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if d.HasChange("rule") {
		minioCreateILMPolicy(ctx, d, meta)
	}

	return minioReadILMPolicy(ctx, d, meta)
}

func minioDeleteILMPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Client

	config := lifecycle.NewConfiguration()

	if err := c.SetBucketLifecycle(ctx, d.Id(), config); err != nil {
		return NewResourceError("deleting lifecycle configuration failed", d.Id(), err)
	}

	d.SetId("")

	return nil
}

func parseILMExpiration(s string) lifecycle.Expiration {
	var days int
	if s == "DeleteMarker" {
		return lifecycle.Expiration{DeleteMarker: true}
	}
	if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
		return lifecycle.Expiration{Days: lifecycle.ExpirationDays(days)}
	}
	if date, err := time.Parse("2006-01-02", s); err == nil {
		return lifecycle.Expiration{Date: lifecycle.ExpirationDate{Time: date}}
	}

	return lifecycle.Expiration{}
}

func parseILMTransition(transition interface{}) lifecycle.Transition {
	transitions := transition.([]interface{})
	if len(transitions) == 0 {
		return lifecycle.Transition{}
	}

	t := transitions[0].(map[string]interface{})
	if t == nil {
		return lifecycle.Transition{}
	}

	days, ok := t["days"].(string)
	if ok && days != "" {
		var daysInt int
		if _, err := fmt.Sscanf(days, "%dd", &daysInt); err == nil {
			return lifecycle.Transition{
				Days:         lifecycle.ExpirationDays(daysInt),
				StorageClass: t["storage_class"].(string),
			}
		}
	}

	date, ok := t["date"].(string)
	if ok && date != "" {
		if parsedDate, err := time.Parse("2006-01-02", date); err == nil {
			return lifecycle.Transition{
				Date:         lifecycle.ExpirationDate{Time: parsedDate},
				StorageClass: t["storage_class"].(string),
			}
		}
	}

	return lifecycle.Transition{}
}

func parseILMNoncurrentTransition(noncurrentTransition interface{}) (lifecycle.NoncurrentVersionTransition, error) {
	if noncurrentTransition == nil {
		return lifecycle.NoncurrentVersionTransition{}, nil
	}

	transitions, ok := noncurrentTransition.([]interface{})
	if !ok || len(transitions) == 0 {
		return lifecycle.NoncurrentVersionTransition{}, nil
	}

	t, ok := transitions[0].(map[string]interface{})
	if !ok || t == nil {
		return lifecycle.NoncurrentVersionTransition{}, fmt.Errorf("invalid noncurrent_transition format")
	}

	days, ok := getStringValue(t, "days")
	if !ok {
		return lifecycle.NoncurrentVersionTransition{}, fmt.Errorf("days is required")
	}

	var daysInt int
	if _, err := fmt.Sscanf(days, "%dd", &daysInt); err != nil {
		return lifecycle.NoncurrentVersionTransition{}, fmt.Errorf("invalid days format: %s", days)
	}

	return lifecycle.NoncurrentVersionTransition{
		NoncurrentDays:          lifecycle.ExpirationDays(daysInt),
		StorageClass:            t["storage_class"].(string),
		NewerNoncurrentVersions: t["newer_versions"].(int),
	}, nil
}

func parseILMNoncurrentExpiration(noncurrentExpiration interface{}) lifecycle.NoncurrentVersionExpiration {
	noncurrentExpirations := noncurrentExpiration.([]interface{})
	if len(noncurrentExpirations) == 0 {
		return lifecycle.NoncurrentVersionExpiration{}
	}

	t := noncurrentExpirations[0].(map[string]interface{})
	if t == nil {
		return lifecycle.NoncurrentVersionExpiration{}
	}

	days, ok := t["days"].(string)
	if !ok || days == "" {
		return lifecycle.NoncurrentVersionExpiration{}
	}

	var daysInt int
	if _, err := fmt.Sscanf(days, "%dd", &daysInt); err == nil {
		newerVersions, _ := t["newer_versions"].(int) // Optional field
		return lifecycle.NoncurrentVersionExpiration{
			NoncurrentDays:          lifecycle.ExpirationDays(daysInt),
			NewerNoncurrentVersions: newerVersions,
		}
	}

	return lifecycle.NoncurrentVersionExpiration{}
}

func getStringValue(m map[string]interface{}, key string) (string, bool) {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

// Use this helper for tags conversion
func convertToStringMap(v interface{}) map[string]string {
	result := make(map[string]string)
	if m, ok := v.(map[string]interface{}); ok {
		for k, v := range m {
			if s, ok := v.(string); ok {
				result[k] = s
			}
		}
	}
	return result
}

func isNotFoundError(err error) bool {
	return strings.Contains(err.Error(), "The lifecycle configuration does not exist")
}
