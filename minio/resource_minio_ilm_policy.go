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
						"expiration": {
							Type:             schema.TypeString,
							Optional:         true,
							ValidateDiagFunc: validateILMExpiration,
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

func validateILMExpiration(v interface{}, p cty.Path) (errors diag.Diagnostics) {
	value := v.(string)
	exp := parseILMExpiration(value)

	if (lifecycle.Expiration{}) == exp {
		return diag.Errorf("expiration must be a duration (5d) or date (1970-01-01)")
	}

	return
}

func minioCreateILMPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Client

	config := lifecycle.NewConfiguration()

	bucket := d.Get("bucket").(string)
	rules := d.Get("rule").([]interface{})
	for _, ruleI := range rules {
		rule := ruleI.(map[string]interface{})
		r := lifecycle.Rule{
			ID:         rule["id"].(string),
			Expiration: parseILMExpiration(rule["expiration"].(string)),
			Status:     "Enabled",
			RuleFilter: lifecycle.Filter{Prefix: rule["filter"].(string)},
		}
		config.Rules = append(config.Rules, r)
	}

	if err := c.SetBucketLifecycle(ctx, bucket, config); err != nil {
		return NewResourceError("creating bucket lifecycle failed", bucket, err)
	}

	d.SetId(bucket)

	return minioReadILMPolicy(ctx, d, meta)
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
		if r.Expiration.Days != 0 {
			expiration = fmt.Sprintf("%dd", r.Expiration.Days)
		} else {
			expiration = r.Expiration.Date.Format("2006-01-02")
		}

		rule := map[string]interface{}{
			"id":         r.ID,
			"expiration": expiration,
			"status":     r.Status,
			"filter":     r.RuleFilter.Prefix,
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
	if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
		return lifecycle.Expiration{Days: lifecycle.ExpirationDays(days)}
	}
	if date, err := time.Parse("2006-01-02", s); err == nil {
		return lifecycle.Expiration{Date: lifecycle.ExpirationDate{Time: date}}
	}

	return lifecycle.Expiration{}
}
