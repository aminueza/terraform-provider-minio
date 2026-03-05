package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketCorsConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the CORS configuration of an existing S3 bucket.",
		Read:        dataSourceMinioS3BucketCorsConfigRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true},
			"cors_rule": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"allowed_headers": {Type: schema.TypeList, Computed: true, Elem: &schema.Schema{Type: schema.TypeString}},
						"allowed_methods": {Type: schema.TypeList, Computed: true, Elem: &schema.Schema{Type: schema.TypeString}},
						"allowed_origins": {Type: schema.TypeList, Computed: true, Elem: &schema.Schema{Type: schema.TypeString}},
						"expose_headers":  {Type: schema.TypeList, Computed: true, Elem: &schema.Schema{Type: schema.TypeString}},
						"max_age_seconds": {Type: schema.TypeInt, Computed: true},
					},
				},
			},
		},
	}
}

func dataSourceMinioS3BucketCorsConfigRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)

	d.SetId(bucket)

	cfg, err := client.GetBucketCors(context.Background(), bucket)
	if err != nil || cfg == nil {
		_ = d.Set("cors_rule", []interface{}{})
		return nil
	}

	var rules []map[string]interface{}
	for _, r := range cfg.CORSRules {
		rules = append(rules, map[string]interface{}{
			"allowed_headers": r.AllowedHeader,
			"allowed_methods": r.AllowedMethod,
			"allowed_origins": r.AllowedOrigin,
			"expose_headers":  r.ExposeHeader,
			"max_age_seconds": r.MaxAgeSeconds,
		})
	}

	_ = d.Set("cors_rule", rules)
	return nil
}
