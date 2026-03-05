package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioILMPolicy() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the ILM lifecycle rules configured on an existing S3 bucket.",
		Read:        dataSourceMinioILMPolicyRead,
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

func dataSourceMinioILMPolicyRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	d.SetId(bucket)

	cfg, err := client.GetBucketLifecycle(context.Background(), bucket)
	if err != nil {
		_ = d.Set("rules", []interface{}{})
		return nil
	}

	var rules []map[string]interface{}
	for _, r := range cfg.Rules {
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

	_ = d.Set("rules", rules)
	return nil
}
