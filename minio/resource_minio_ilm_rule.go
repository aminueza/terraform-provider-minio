package minio

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

func resourceMinioILMRule() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateILMRule,
		ReadContext:   minioReadILMRule,
		DeleteContext: minioDeleteILMRule,
		UpdateContext: minioUpdateILMRule,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Description: "`minio_ilm_rule` handles lifecycle settings for a given `minio_s3_bucket`.",
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(0, 63),
			},
			"rules": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Required: true,
						},
						"expiration": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"status": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"filter": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},
		},
	}
}

func minioCreateILMRule(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Client

	config := lifecycle.NewConfiguration()

	bucket := d.Get("bucket").(string)
	rules := d.Get("rules").([]interface{})
	for _, ruleI := range rules {
		rule := ruleI.(map[string]interface{})
		r := lifecycle.Rule{
			ID:         rule["id"].(string),
			Expiration: lifecycle.Expiration{Days: lifecycle.ExpirationDays(rule["expiration"].(int))},
			Status:     "Enabled",
			RuleFilter: lifecycle.Filter{Prefix: rule["filter"].(string)},
		}
		config.Rules = append(config.Rules, r)
	}

	if err := c.SetBucketLifecycle(ctx, bucket, config); err != nil {
		return NewResourceError("creating bucket lifecycle failed", bucket, err)
	}

	d.SetId(bucket)

	minioReadILMRule(ctx, d, meta)

	return nil
}

func minioReadILMRule(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Client

	rules := make([]map[string]interface{}, 0)
	config, err := c.GetBucketLifecycle(ctx, d.Id())
	if err != nil {
		// TODO: distinguish between error and 404 not found
		log.Println(NewResourceErrorStr("reading lifecycle configuration failed", d.Id(), err))
		d.SetId("")
		return nil
	}

	for _, r := range config.Rules {
		rule := map[string]interface{}{
			"id":         r.ID,
			"expiration": r.Expiration.Days,
			"status":     r.Status,
			"filter":     r.RuleFilter.Prefix,
		}
		rules = append(rules, rule)
	}

	if err := d.Set("rules", rules); err != nil {
		return NewResourceError("reading lifecycle configuration failed", d.Id(), err)
	}

	return nil
}

func minioUpdateILMRule(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if d.HasChange("rules") {
		minioCreateILMRule(ctx, d, meta)
	}

	return minioReadILMRule(ctx, d, meta)
}

func minioDeleteILMRule(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Client

	config := lifecycle.NewConfiguration()

	if err := c.SetBucketLifecycle(ctx, d.Id(), config); err != nil {
		return NewResourceError("deleting lifecycle configuration failed", d.Id(), err)
	}

	d.SetId("")

	return nil
}
