package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

func dataSourceMinioILMPolicy() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the ILM lifecycle rules configured on an existing S3 bucket.",
		ReadContext: dataSourceMinioILMPolicyRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true},
			"rules": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id":     {Type: schema.TypeString, Computed: true},
						"status": {Type: schema.TypeString, Computed: true},
						"prefix": {Type: schema.TypeString, Computed: true},
						"expiration_days": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"transition_days": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"transition_storage_class": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"noncurrent_expiration_days": {
							Type:     schema.TypeInt,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func dataSourceMinioILMPolicyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	bucket := d.Get("bucket").(string)

	d.SetId(bucket)

	var diags diag.Diagnostics
	rules := []map[string]interface{}{}

	cfg, err := client.S3Client.GetBucketLifecycle(ctx, bucket)
	switch {
	case err == nil:
		rules = flattenILMDataSourceRules(cfg.Rules)
	case isLifecycleNotFoundError(err):
	case isS3CompatNotSupported(client, s3Compat405IsMissingFeature, err):
		diags = append(diags, dataSourceS3CompatGap(bucket, "rules", "bucket lifecycle"))
	default:
		return NewResourceError("reading bucket lifecycle", bucket, err)
	}

	if err := d.Set("rules", rules); err != nil {
		return NewResourceError("setting rules", bucket, err)
	}

	return diags
}

func flattenILMDataSourceRules(lifecycleRules []lifecycle.Rule) []map[string]interface{} {
	rules := make([]map[string]interface{}, 0, len(lifecycleRules))
	for _, r := range lifecycleRules {
		rule := map[string]interface{}{
			"id":     r.ID,
			"status": r.Status,
			"prefix": r.RuleFilter.Prefix,
		}

		if r.Expiration.Days > 0 {
			rule["expiration_days"] = int(r.Expiration.Days)
		}
		if r.Transition.Days > 0 {
			rule["transition_days"] = int(r.Transition.Days)
			rule["transition_storage_class"] = r.Transition.StorageClass
		}
		if r.NoncurrentVersionExpiration.NoncurrentDays > 0 {
			rule["noncurrent_expiration_days"] = int(r.NoncurrentVersionExpiration.NoncurrentDays)
		}

		rules = append(rules, rule)
	}
	return rules
}
