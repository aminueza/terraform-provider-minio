package minio

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"log"
)

func resourceMinioILMRule() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateILMRule,
		ReadContext:   minioReadILMRule,
		DeleteContext: minioDeleteILMRule,
		Importer: &schema.ResourceImporter{
			StateContext: minioImportILMRule,
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
func minioDeleteILMRule(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}
func minioImportILMRule(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	return nil, nil
}
