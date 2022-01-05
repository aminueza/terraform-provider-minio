package minio

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"log"
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
		Schema: map[string]*schema.Schema{
			"rules": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"expiration": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"status": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"filter": {
							Type:     schema.TypeString,
							Computed: true,
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

	rules := d.Get("rules").([]map[string]interface{})
	for _, rule := range rules {
		r := lifecycle.Rule{
			ID:         rule["id"].(string),
			Expiration: lifecycle.Expiration{Days: rule["expiration"].(lifecycle.ExpirationDays)},
			Status:     rule["status"].(string),
			RuleFilter: lifecycle.Filter{Prefix: rule["filter"].(string)},
		}
		config.Rules = append(config.Rules, r)
	}

	if err := c.SetBucketLifecycle(ctx, d.Id(), config); err != nil {
		return NewResourceError("creating bucket lifecycle failed", d.Id(), err)
	}

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
		NewResourceError("deleting lifecycle configuration failed", d.Id(), err)
	}

	d.SetId("")

	return nil
}
