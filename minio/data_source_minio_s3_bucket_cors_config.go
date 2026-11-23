package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketCorsConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the CORS configuration of an existing S3 bucket.",
		ReadContext: dataSourceMinioS3BucketCorsConfigRead,
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

func dataSourceMinioS3BucketCorsConfigRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	bucket := d.Get("bucket").(string)

	var diags diag.Diagnostics
	cfg, err := client.S3Client.GetBucketCors(ctx, bucket)
	switch {
	case err == nil:
	case isS3CompatNotSupported(client, s3Compat405IsMissingFeature, err):
		diags = append(diags, dataSourceS3CompatGap(bucket, "cors_rule", "CORS configuration"))
	default:
		return NewResourceError("reading bucket CORS configuration", bucket, err)
	}

	d.SetId(bucket)

	rules := make([]map[string]interface{}, 0)
	if cfg != nil {
		rules = make([]map[string]interface{}, 0, len(cfg.CORSRules))
		for _, r := range cfg.CORSRules {
			rules = append(rules, map[string]interface{}{
				"allowed_headers": r.AllowedHeader,
				"allowed_methods": r.AllowedMethod,
				"allowed_origins": r.AllowedOrigin,
				"expose_headers":  r.ExposeHeader,
				"max_age_seconds": r.MaxAgeSeconds,
			})
		}
	}

	if err := d.Set("cors_rule", rules); err != nil {
		return NewResourceError("setting cors_rule", bucket, err)
	}
	return diags
}
