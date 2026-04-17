package minio

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

func resourceMinioS3BucketLifecycle() *schema.Resource {
	return &schema.Resource{
		Description: "Manages S3 bucket lifecycle configuration (expiration, transitions, noncurrent version handling, and multipart upload cleanup). " +
			"Provides AWS S3 parity with `aws_s3_bucket_lifecycle_configuration`. " +
			"Do not configure both `minio_s3_bucket_lifecycle` and `minio_ilm_policy` for the same bucket.",
		CreateContext: minioCreateS3BucketLifecycle,
		ReadContext:   minioReadS3BucketLifecycle,
		UpdateContext: minioUpdateS3BucketLifecycle,
		DeleteContext: minioDeleteS3BucketLifecycle,
		CustomizeDiff: customizeDiffS3BucketLifecycle,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(1, 63),
				Description:  "Name of the bucket to apply the lifecycle configuration to.",
			},
			"rule": {
				Type:        schema.TypeList,
				Required:    true,
				MinItems:    1,
				Description: "Lifecycle rules. Evaluated in order; if multiple rules match an object, MinIO applies the most restrictive expiration/transition.",
				Elem: &schema.Resource{
					Schema: bucketLifecycleRuleSchema(),
				},
			},
		},
	}
}

func bucketLifecycleRuleSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"id": {
			Type:         schema.TypeString,
			Required:     true,
			ValidateFunc: validation.StringLenBetween(1, 255),
			Description:  "Unique identifier for the rule (max 255 chars).",
		},
		"status": {
			Type:         schema.TypeString,
			Optional:     true,
			Default:      "Enabled",
			ValidateFunc: validation.StringInSlice([]string{"Enabled", "Disabled"}, false),
			Description:  "Whether the rule is currently applied. One of `Enabled` or `Disabled`. Defaults to `Enabled`.",
		},
		"filter": {
			Type:        schema.TypeList,
			Optional:    true,
			MaxItems:    1,
			Description: "Filter identifying one or more objects to which the rule applies. Omit to match all objects in the bucket.",
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"prefix": {
						Type:        schema.TypeString,
						Optional:    true,
						Description: "Prefix identifying one or more objects to which the rule applies. Cannot be combined with `tag` at the top level; use `and` for composite filters.",
					},
					"tag": {
						Type:        schema.TypeList,
						Optional:    true,
						MaxItems:    1,
						Description: "Single tag to match. Use `and` for multi-tag or prefix+tag composite filters.",
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"key": {
									Type:         schema.TypeString,
									Required:     true,
									ValidateFunc: validation.StringLenBetween(1, 128),
								},
								"value": {
									Type:         schema.TypeString,
									Required:     true,
									ValidateFunc: validation.StringLenBetween(0, 256),
								},
							},
						},
					},
					"object_size_greater_than": {
						Type:         schema.TypeInt,
						Optional:     true,
						ValidateFunc: validation.IntAtLeast(0),
						Description:  "Minimum object size in bytes for the rule to apply.",
					},
					"object_size_less_than": {
						Type:         schema.TypeInt,
						Optional:     true,
						ValidateFunc: validation.IntAtLeast(0),
						Description:  "Maximum object size in bytes for the rule to apply.",
					},
					"and": {
						Type:        schema.TypeList,
						Optional:    true,
						MaxItems:    1,
						Description: "Composite filter (AND of prefix, tags, and object size bounds). Use when combining more than one condition.",
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"prefix": {
									Type:     schema.TypeString,
									Optional: true,
								},
								"tags": {
									Type:     schema.TypeMap,
									Optional: true,
									Elem:     &schema.Schema{Type: schema.TypeString},
								},
								"object_size_greater_than": {
									Type:         schema.TypeInt,
									Optional:     true,
									ValidateFunc: validation.IntAtLeast(0),
								},
								"object_size_less_than": {
									Type:         schema.TypeInt,
									Optional:     true,
									ValidateFunc: validation.IntAtLeast(0),
								},
							},
						},
					},
				},
			},
		},
		"expiration": {
			Type:        schema.TypeList,
			Optional:    true,
			MaxItems:    1,
			Description: "Expiration for current object versions.",
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"days": {
						Type:         schema.TypeInt,
						Optional:     true,
						ValidateFunc: validation.IntAtLeast(1),
						Description:  "Number of days after object creation before expiration.",
					},
					"date": {
						Type:             schema.TypeString,
						Optional:         true,
						ValidateDiagFunc: validateLifecycleDate,
						Description:      "Absolute date (YYYY-MM-DD) after which objects expire. Mutually exclusive with `days`.",
					},
					"expired_object_delete_marker": {
						Type:        schema.TypeBool,
						Optional:    true,
						Description: "If true, remove expired-object delete markers when the object has no remaining non-current versions.",
					},
				},
			},
		},
		"transition": {
			Type:        schema.TypeList,
			Optional:    true,
			MaxItems:    1,
			Description: "Transition for current object versions to another storage class. Only one transition per rule; add additional rules for multi-stage transitions.",
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"days": {
						Type:         schema.TypeInt,
						Optional:     true,
						ValidateFunc: validation.IntAtLeast(0),
						Description:  "Number of days after object creation before transition.",
					},
					"date": {
						Type:             schema.TypeString,
						Optional:         true,
						ValidateDiagFunc: validateLifecycleDate,
						Description:      "Absolute date (YYYY-MM-DD) after which transition occurs. Mutually exclusive with `days`.",
					},
					"storage_class": {
						Type:         schema.TypeString,
						Required:     true,
						ValidateFunc: validation.StringLenBetween(1, 256),
						Description:  "Target storage class for the transition (e.g. `GLACIER`, `STANDARD_IA`, or a custom MinIO tier).",
					},
				},
			},
		},
		"noncurrent_version_expiration": {
			Type:        schema.TypeList,
			Optional:    true,
			MaxItems:    1,
			Description: "Expiration for non-current object versions. Requires versioning enabled on the bucket.",
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"noncurrent_days": {
						Type:         schema.TypeInt,
						Required:     true,
						ValidateFunc: validation.IntAtLeast(1),
						Description:  "Number of days after which non-current versions expire.",
					},
					"newer_noncurrent_versions": {
						Type:         schema.TypeInt,
						Optional:     true,
						ValidateFunc: validation.IntAtLeast(0),
						Description:  "Number of non-current versions to retain regardless of `noncurrent_days`.",
					},
				},
			},
		},
		"noncurrent_version_transition": {
			Type:        schema.TypeList,
			Optional:    true,
			MaxItems:    1,
			Description: "Transition for non-current object versions to another storage class. Requires versioning enabled on the bucket.",
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"noncurrent_days": {
						Type:         schema.TypeInt,
						Required:     true,
						ValidateFunc: validation.IntAtLeast(0),
						Description:  "Number of days after becoming non-current before the transition applies.",
					},
					"newer_noncurrent_versions": {
						Type:         schema.TypeInt,
						Optional:     true,
						ValidateFunc: validation.IntAtLeast(0),
						Description:  "Number of non-current versions to retain in the current class.",
					},
					"storage_class": {
						Type:         schema.TypeString,
						Required:     true,
						ValidateFunc: validation.StringLenBetween(1, 256),
					},
				},
			},
		},
		"abort_incomplete_multipart_upload": {
			Type:        schema.TypeList,
			Optional:    true,
			MaxItems:    1,
			Description: "Aborts and cleans up incomplete multipart uploads after a configurable number of days. Cannot be combined with tag-based filters on the rule.",
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"days_after_initiation": {
						Type:         schema.TypeInt,
						Required:     true,
						ValidateFunc: validation.IntAtLeast(1),
						Description:  "Number of days since upload initiation after which the multipart upload is aborted.",
					},
				},
			},
		},
	}
}

func customizeDiffS3BucketLifecycle(_ context.Context, d *schema.ResourceDiff, _ interface{}) error {
	rulesRaw, ok := d.Get("rule").([]interface{})
	if !ok {
		return nil
	}
	seenIDs := make(map[string]struct{}, len(rulesRaw))
	for i, ri := range rulesRaw {
		rule, ok := ri.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := rule["id"].(string)
		if id != "" {
			if _, dup := seenIDs[id]; dup {
				return fmt.Errorf("rule %d: duplicate rule id %q; each rule must have a unique id", i, id)
			}
			seenIDs[id] = struct{}{}
		}
		if err := validateLifecycleRule(id, rule); err != nil {
			return err
		}
	}
	return nil
}

func validateLifecycleRule(id string, rule map[string]interface{}) error {
	if id == "" {
		id = "<unknown>"
	}

	hasAction := false
	if exp, _ := rule["expiration"].([]interface{}); len(exp) > 0 {
		hasAction = true
		if err := validateLifecycleExpiration(id, exp[0]); err != nil {
			return err
		}
	}
	if t, _ := rule["transition"].([]interface{}); len(t) > 0 {
		hasAction = true
		if err := validateLifecycleTransitions(id, t); err != nil {
			return err
		}
	}
	if n, _ := rule["noncurrent_version_expiration"].([]interface{}); len(n) > 0 {
		hasAction = true
	}
	if n, _ := rule["noncurrent_version_transition"].([]interface{}); len(n) > 0 {
		hasAction = true
	}
	if a, _ := rule["abort_incomplete_multipart_upload"].([]interface{}); len(a) > 0 {
		hasAction = true
	}
	if !hasAction {
		return fmt.Errorf("rule %q: at least one of expiration, transition, noncurrent_version_expiration, noncurrent_version_transition, or abort_incomplete_multipart_upload must be specified", id)
	}

	if filters, _ := rule["filter"].([]interface{}); len(filters) > 0 {
		if err := validateLifecycleFilter(id, filters[0]); err != nil {
			return err
		}
	}
	return nil
}

func validateLifecycleExpiration(id string, raw interface{}) error {
	m, ok := raw.(map[string]interface{})
	if !ok || m == nil {
		return nil
	}
	days, _ := m["days"].(int)
	date, _ := m["date"].(string)
	deleteMarker, _ := m["expired_object_delete_marker"].(bool)

	set := 0
	if days > 0 {
		set++
	}
	if date != "" {
		set++
	}
	if deleteMarker {
		set++
	}
	if set == 0 {
		return fmt.Errorf("rule %q: expiration block requires one of days, date, or expired_object_delete_marker", id)
	}
	if set > 1 {
		return fmt.Errorf("rule %q: expiration fields days, date, and expired_object_delete_marker are mutually exclusive", id)
	}
	return nil
}

func validateLifecycleTransitions(id string, transitions []interface{}) error {
	for i, raw := range transitions {
		m, ok := raw.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		days, _ := m["days"].(int)
		date, _ := m["date"].(string)
		if days > 0 && date != "" {
			return fmt.Errorf("rule %q: transition[%d] fields days and date are mutually exclusive", id, i)
		}
		if days == 0 && date == "" {
			return fmt.Errorf("rule %q: transition[%d] requires either days or date", id, i)
		}
	}
	return nil
}

func validateLifecycleFilter(id string, raw interface{}) error {
	m, ok := raw.(map[string]interface{})
	if !ok || m == nil {
		return nil
	}
	prefix, _ := m["prefix"].(string)
	tags, _ := m["tag"].([]interface{})
	sizeGT, _ := m["object_size_greater_than"].(int)
	sizeLT, _ := m["object_size_less_than"].(int)
	and, _ := m["and"].([]interface{})

	topLevel := 0
	if prefix != "" {
		topLevel++
	}
	if len(tags) > 0 {
		topLevel++
	}
	if sizeGT > 0 {
		topLevel++
	}
	if sizeLT > 0 {
		topLevel++
	}

	if len(and) > 0 && topLevel > 0 {
		return fmt.Errorf("rule %q: filter.and is mutually exclusive with top-level prefix/tag/object_size_* in the same filter", id)
	}
	if topLevel > 1 {
		return fmt.Errorf("rule %q: top-level filter fields are mutually exclusive; use filter.and for composite filters", id)
	}
	if err := validateLifecycleSizeBounds(id, sizeGT, sizeLT); err != nil {
		return err
	}
	if len(and) > 0 {
		andMap, _ := and[0].(map[string]interface{})
		andGT := getIntValue(andMap, "object_size_greater_than")
		andLT := getIntValue(andMap, "object_size_less_than")
		if err := validateLifecycleSizeBounds(id, andGT, andLT); err != nil {
			return err
		}
	}
	return nil
}

func validateLifecycleSizeBounds(id string, sizeGT, sizeLT int) error {
	if sizeGT > 0 && sizeLT > 0 && sizeGT >= sizeLT {
		return fmt.Errorf("rule %q: object_size_greater_than (%d) must be less than object_size_less_than (%d)", id, sizeGT, sizeLT)
	}
	return nil
}

func validateLifecycleDate(v interface{}, _ cty.Path) diag.Diagnostics {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	if _, err := time.Parse(lifecycleDateFormat, s); err != nil {
		return diag.Errorf("date must be in format YYYY-MM-DD, got: %s", s)
	}
	return nil
}

func minioCreateS3BucketLifecycle(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	if _, err := c.BucketExists(ctx, bucket); err != nil {
		return NewResourceError("validating bucket", bucket, err)
	}

	oldConfig, err := c.GetBucketLifecycle(ctx, bucket)
	if err != nil && !isLifecycleNotFoundError(err) {
		return NewResourceError("reading existing lifecycle", bucket, err)
	}

	config, diagErr := buildLifecycleConfig(d)
	if diagErr != nil {
		return diagErr
	}

	if err := c.SetBucketLifecycle(ctx, bucket, config); err != nil {
		if oldConfig != nil {
			if rbErr := c.SetBucketLifecycle(ctx, bucket, oldConfig); rbErr != nil {
				return NewResourceError("setting lifecycle (rollback failed)", bucket, fmt.Errorf("%v; rollback error: %v", err, rbErr))
			}
		}
		return NewResourceError("setting lifecycle", bucket, err)
	}

	d.SetId(bucket)
	return minioReadS3BucketLifecycle(ctx, d, meta)
}

func minioReadS3BucketLifecycle(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Client
	bucket := d.Id()

	config, err := c.GetBucketLifecycle(ctx, bucket)
	if err != nil {
		if isS3CompatNotSupported(meta.(*S3MinioClient), err) {
			log.Printf("[INFO] Lifecycle rules not supported by backend; dropping %s from state", bucket)
			d.SetId("")
			return nil
		}
		if isLifecycleNotFoundError(err) {
			log.Printf("[WARN] Lifecycle configuration for %s not found; removing from state", bucket)
			d.SetId("")
			return nil
		}
		return NewResourceError("reading lifecycle", bucket, err)
	}

	if err := d.Set("bucket", bucket); err != nil {
		return NewResourceError("setting bucket", bucket, err)
	}

	abortFromState := abortBlocksFromState(d)

	rules := make([]map[string]interface{}, 0, len(config.Rules))
	for _, r := range config.Rules {
		flat := flattenLifecycleRule(r)
		if _, present := flat["abort_incomplete_multipart_upload"]; !present {
			if preserved, ok := abortFromState[r.ID]; ok {
				flat["abort_incomplete_multipart_upload"] = preserved
			}
		}
		rules = append(rules, flat)
	}
	if err := d.Set("rule", rules); err != nil {
		return NewResourceError("setting rules", bucket, err)
	}
	return nil
}

// abortBlocksFromState returns the user-configured abort_incomplete_multipart_upload
// block keyed by rule ID. MinIO does not always round-trip this field, so we fall
// back to the last-known config to avoid spurious drift on refresh.
func abortBlocksFromState(d *schema.ResourceData) map[string][]map[string]interface{} {
	out := map[string][]map[string]interface{}{}
	raw, _ := d.GetOk("rule")
	rules, ok := raw.([]interface{})
	if !ok {
		return out
	}
	for _, ri := range rules {
		rule, ok := ri.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := getStringValue(rule, "id")
		if id == "" {
			continue
		}
		abortList, ok := rule["abort_incomplete_multipart_upload"].([]interface{})
		if !ok || len(abortList) == 0 {
			continue
		}
		first, ok := abortList[0].(map[string]interface{})
		if !ok || first == nil {
			continue
		}
		if days := getIntValue(first, "days_after_initiation"); days > 0 {
			out[id] = []map[string]interface{}{{"days_after_initiation": days}}
		}
	}
	return out
}

func minioUpdateS3BucketLifecycle(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if !d.HasChange("rule") {
		return minioReadS3BucketLifecycle(ctx, d, meta)
	}
	return minioCreateS3BucketLifecycle(ctx, d, meta)
}

func minioDeleteS3BucketLifecycle(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Client
	bucket := d.Id()

	empty := lifecycle.NewConfiguration()
	if err := c.SetBucketLifecycle(ctx, bucket, empty); err != nil {
		if isLifecycleNotFoundError(err) {
			d.SetId("")
			return nil
		}
		return NewResourceError("deleting lifecycle", bucket, err)
	}
	d.SetId("")
	return nil
}

func buildLifecycleConfig(d *schema.ResourceData) (*lifecycle.Configuration, diag.Diagnostics) {
	config := lifecycle.NewConfiguration()
	rulesRaw, _ := d.Get("rule").([]interface{})

	for i, ri := range rulesRaw {
		rule, ok := ri.(map[string]interface{})
		if !ok {
			return nil, diag.Errorf("rule[%d]: invalid format", i)
		}
		built, err := buildLifecycleRule(rule)
		if err != nil {
			return nil, diag.FromErr(err)
		}
		config.Rules = append(config.Rules, built)
	}
	return config, nil
}

func buildLifecycleRule(rule map[string]interface{}) (lifecycle.Rule, error) {
	id, _ := rule["id"].(string)
	status, _ := rule["status"].(string)
	if status == "" {
		status = "Enabled"
	}

	out := lifecycle.Rule{
		ID:     id,
		Status: status,
	}

	if exp, _ := rule["expiration"].([]interface{}); len(exp) > 0 {
		if first, ok := exp[0].(map[string]interface{}); ok && first != nil {
			out.Expiration = buildLifecycleExpiration(first)
		}
	}

	if t, _ := rule["transition"].([]interface{}); len(t) > 0 {
		if first, ok := t[0].(map[string]interface{}); ok && first != nil {
			out.Transition = buildLifecycleTransition(first)
		}
	}

	if n, _ := rule["noncurrent_version_expiration"].([]interface{}); len(n) > 0 {
		if first, ok := n[0].(map[string]interface{}); ok && first != nil {
			out.NoncurrentVersionExpiration = lifecycle.NoncurrentVersionExpiration{
				NoncurrentDays:          lifecycle.ExpirationDays(getIntValue(first, "noncurrent_days")),
				NewerNoncurrentVersions: getIntValue(first, "newer_noncurrent_versions"),
			}
		}
	}

	if n, _ := rule["noncurrent_version_transition"].([]interface{}); len(n) > 0 {
		if first, ok := n[0].(map[string]interface{}); ok && first != nil {
			storageClass, _ := getStringValue(first, "storage_class")
			out.NoncurrentVersionTransition = lifecycle.NoncurrentVersionTransition{
				NoncurrentDays:          lifecycle.ExpirationDays(getIntValue(first, "noncurrent_days")),
				NewerNoncurrentVersions: getIntValue(first, "newer_noncurrent_versions"),
				StorageClass:            storageClass,
			}
		}
	}

	if a, _ := rule["abort_incomplete_multipart_upload"].([]interface{}); len(a) > 0 {
		if first, ok := a[0].(map[string]interface{}); ok && first != nil {
			out.AbortIncompleteMultipartUpload = lifecycle.AbortIncompleteMultipartUpload{
				DaysAfterInitiation: lifecycle.ExpirationDays(getIntValue(first, "days_after_initiation")),
			}
		}
	}

	if f, _ := rule["filter"].([]interface{}); len(f) > 0 {
		if first, ok := f[0].(map[string]interface{}); ok && first != nil {
			out.RuleFilter = buildLifecycleFilter(first)
		}
	} else {
		out.RuleFilter = lifecycle.Filter{ObjectSizeGreaterThan: lifecycleEmptyFilterSentinel}
	}

	return out, nil
}

func buildLifecycleExpiration(m map[string]interface{}) lifecycle.Expiration {
	if deleteMarker, _ := m["expired_object_delete_marker"].(bool); deleteMarker {
		return lifecycle.Expiration{DeleteMarker: true}
	}
	if days := getIntValue(m, "days"); days > 0 {
		return lifecycle.Expiration{Days: lifecycle.ExpirationDays(days)}
	}
	if date, _ := getStringValue(m, "date"); date != "" {
		if parsed, err := time.Parse(lifecycleDateFormat, date); err == nil {
			return lifecycle.Expiration{Date: lifecycle.ExpirationDate{Time: parsed}}
		}
	}
	return lifecycle.Expiration{}
}

func buildLifecycleTransition(m map[string]interface{}) lifecycle.Transition {
	storageClass, _ := getStringValue(m, "storage_class")
	if days := getIntValue(m, "days"); days > 0 {
		return lifecycle.Transition{
			Days:         lifecycle.ExpirationDays(days),
			StorageClass: storageClass,
		}
	}
	if date, _ := getStringValue(m, "date"); date != "" {
		if parsed, err := time.Parse(lifecycleDateFormat, date); err == nil {
			return lifecycle.Transition{
				Date:         lifecycle.ExpirationDate{Time: parsed},
				StorageClass: storageClass,
			}
		}
	}
	return lifecycle.Transition{StorageClass: storageClass}
}

func buildLifecycleFilter(m map[string]interface{}) lifecycle.Filter {
	if andRaw, _ := m["and"].([]interface{}); len(andRaw) > 0 {
		if first, ok := andRaw[0].(map[string]interface{}); ok && first != nil {
			prefix, _ := getStringValue(first, "prefix")
			and := lifecycle.And{
				Prefix:                prefix,
				ObjectSizeLessThan:    int64(getIntValue(first, "object_size_less_than")),
				ObjectSizeGreaterThan: int64(getIntValue(first, "object_size_greater_than")),
			}
			for k, v := range convertToStringMap(first["tags"]) {
				and.Tags = append(and.Tags, lifecycle.Tag{Key: k, Value: v})
			}
			return lifecycle.Filter{And: and}
		}
	}

	prefix, _ := getStringValue(m, "prefix")
	filter := lifecycle.Filter{
		Prefix:                prefix,
		ObjectSizeLessThan:    int64(getIntValue(m, "object_size_less_than")),
		ObjectSizeGreaterThan: int64(getIntValue(m, "object_size_greater_than")),
	}
	if tagsRaw, _ := m["tag"].([]interface{}); len(tagsRaw) > 0 {
		if first, ok := tagsRaw[0].(map[string]interface{}); ok && first != nil {
			key, _ := getStringValue(first, "key")
			value, _ := getStringValue(first, "value")
			filter.Tag = lifecycle.Tag{Key: key, Value: value}
		}
	}
	if filter.IsNull() {
		filter.ObjectSizeGreaterThan = lifecycleEmptyFilterSentinel
	}
	return filter
}

func flattenLifecycleRule(r lifecycle.Rule) map[string]interface{} {
	out := map[string]interface{}{
		"id":     r.ID,
		"status": r.Status,
	}

	if !r.Expiration.IsNull() {
		exp := map[string]interface{}{}
		if r.Expiration.IsDeleteMarkerExpirationEnabled() {
			exp["expired_object_delete_marker"] = true
		} else if r.Expiration.Days != 0 {
			exp["days"] = int(r.Expiration.Days)
		} else if !r.Expiration.Date.IsZero() {
			exp["date"] = r.Expiration.Date.Format(lifecycleDateFormat)
		}
		out["expiration"] = []map[string]interface{}{exp}
	}

	if !r.Transition.IsNull() {
		t := map[string]interface{}{
			"storage_class": r.Transition.StorageClass,
		}
		if !r.Transition.IsDaysNull() {
			t["days"] = int(r.Transition.Days)
		} else if !r.Transition.IsDateNull() {
			t["date"] = r.Transition.Date.Format(lifecycleDateFormat)
		}
		out["transition"] = []map[string]interface{}{t}
	}

	if r.NoncurrentVersionExpiration.NoncurrentDays != 0 || r.NoncurrentVersionExpiration.NewerNoncurrentVersions > 0 {
		out["noncurrent_version_expiration"] = []map[string]interface{}{{
			"noncurrent_days":           int(r.NoncurrentVersionExpiration.NoncurrentDays),
			"newer_noncurrent_versions": r.NoncurrentVersionExpiration.NewerNoncurrentVersions,
		}}
	}

	if r.NoncurrentVersionTransition.StorageClass != "" {
		out["noncurrent_version_transition"] = []map[string]interface{}{{
			"noncurrent_days":           int(r.NoncurrentVersionTransition.NoncurrentDays),
			"newer_noncurrent_versions": r.NoncurrentVersionTransition.NewerNoncurrentVersions,
			"storage_class":             r.NoncurrentVersionTransition.StorageClass,
		}}
	}

	if !r.AbortIncompleteMultipartUpload.IsDaysNull() {
		out["abort_incomplete_multipart_upload"] = []map[string]interface{}{{
			"days_after_initiation": int(r.AbortIncompleteMultipartUpload.DaysAfterInitiation),
		}}
	}

	if !filterIsEffectivelyEmpty(r.RuleFilter) {
		out["filter"] = []map[string]interface{}{flattenLifecycleFilter(r.RuleFilter)}
	}

	return out
}

func flattenLifecycleFilter(f lifecycle.Filter) map[string]interface{} {
	out := map[string]interface{}{}

	if !f.And.IsEmpty() {
		and := map[string]interface{}{}
		if f.And.Prefix != "" {
			and["prefix"] = f.And.Prefix
		}
		if f.And.ObjectSizeGreaterThan > 0 {
			and["object_size_greater_than"] = int(f.And.ObjectSizeGreaterThan)
		}
		if f.And.ObjectSizeLessThan > 0 {
			and["object_size_less_than"] = int(f.And.ObjectSizeLessThan)
		}
		if len(f.And.Tags) > 0 {
			tags := map[string]string{}
			for _, t := range f.And.Tags {
				tags[t.Key] = t.Value
			}
			and["tags"] = tags
		}
		out["and"] = []map[string]interface{}{and}
		return out
	}

	if f.Prefix != "" {
		out["prefix"] = f.Prefix
	}
	if f.ObjectSizeGreaterThan > 0 {
		out["object_size_greater_than"] = int(f.ObjectSizeGreaterThan)
	}
	if f.ObjectSizeLessThan > 0 {
		out["object_size_less_than"] = int(f.ObjectSizeLessThan)
	}
	if f.Tag.Key != "" {
		out["tag"] = []map[string]interface{}{{
			"key":   f.Tag.Key,
			"value": f.Tag.Value,
		}}
	}
	return out
}

func filterIsEffectivelyEmpty(f lifecycle.Filter) bool {
	if f.ObjectSizeGreaterThan == lifecycleEmptyFilterSentinel &&
		f.ObjectSizeLessThan == 0 &&
		f.Prefix == "" &&
		f.Tag.Key == "" &&
		f.And.IsEmpty() {
		return true
	}
	return f.IsNull()
}

