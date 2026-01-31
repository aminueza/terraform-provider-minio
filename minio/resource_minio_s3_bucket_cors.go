package minio

import (
	"context"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7/pkg/cors"
)

func resourceMinioS3BucketCors() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateBucketCors,
		ReadContext:   minioReadBucketCors,
		UpdateContext: minioUpdateBucketCors,
		DeleteContext: minioDeleteBucketCors,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the bucket to apply CORS configuration",
			},
			"cors_rule": {
				Type:        schema.TypeList,
				Required:    true,
				Description: "List of CORS rules",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Unique identifier for the rule",
						},
						"allowed_headers": {
							Type:        schema.TypeList,
							Optional:    true,
							Description: "Headers that are allowed in a preflight OPTIONS request",
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
						"allowed_methods": {
							Type:        schema.TypeList,
							Required:    true,
							Description: "HTTP methods that the origin is allowed to execute (GET, PUT, POST, DELETE, HEAD)",
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
						"allowed_origins": {
							Type:        schema.TypeList,
							Required:    true,
							Description: "Origins that are allowed to access the bucket",
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
						"expose_headers": {
							Type:        schema.TypeList,
							Optional:    true,
							Description: "Headers in the response that customers are able to access from their applications",
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
						"max_age_seconds": {
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "Time in seconds that browser can cache the response for a preflight request",
						},
					},
				},
			},
		},
	}
}

func minioCreateBucketCors(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketCorsConfig := BucketCorsConfig(d, meta)

	log.Printf("[DEBUG] Creating CORS configuration for bucket: %s", bucketCorsConfig.MinioBucket)

	corsConfig := buildCorsConfig(d)

	err := bucketCorsConfig.MinioClient.SetBucketCors(ctx, bucketCorsConfig.MinioBucket, corsConfig)
	if err != nil {
		return NewResourceError("creating CORS configuration", bucketCorsConfig.MinioBucket, err)
	}

	d.SetId(bucketCorsConfig.MinioBucket)

	log.Printf("[DEBUG] Created CORS configuration for bucket: %s", bucketCorsConfig.MinioBucket)

	return minioReadBucketCors(ctx, d, meta)
}

func minioReadBucketCors(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketCorsConfig := BucketCorsConfig(d, meta)

	log.Printf("[DEBUG] Reading CORS configuration for bucket: %s", d.Id())

	corsConfig, err := bucketCorsConfig.MinioClient.GetBucketCors(ctx, d.Id())
	if err != nil {
		if isNoSuchBucketError(err) {
			log.Printf("[WARN] Bucket %s not found, removing CORS resource from state", d.Id())
			d.SetId("")
			return nil
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "CORS configuration does not exist") || strings.Contains(errMsg, "NoSuchCORSConfiguration") {
			log.Printf("[WARN] CORS configuration for bucket %s does not exist, removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return NewResourceError("reading CORS configuration", d.Id(), err)
	}

	if corsConfig == nil || len(corsConfig.CORSRules) == 0 {
		log.Printf("[WARN] CORS configuration for bucket %s is empty, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	corsRules := flattenCorsRules(corsConfig.CORSRules)
	if err := d.Set("cors_rule", corsRules); err != nil {
		return NewResourceError("setting cors_rule", d.Id(), err)
	}

	if err := d.Set("bucket", d.Id()); err != nil {
		return NewResourceError("setting bucket", d.Id(), err)
	}

	return nil
}

func minioUpdateBucketCors(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketCorsConfig := BucketCorsConfig(d, meta)

	log.Printf("[DEBUG] Updating CORS configuration for bucket: %s", bucketCorsConfig.MinioBucket)

	if d.HasChange("cors_rule") {
		corsConfig := buildCorsConfig(d)

		err := bucketCorsConfig.MinioClient.SetBucketCors(ctx, bucketCorsConfig.MinioBucket, corsConfig)
		if err != nil {
			return NewResourceError("updating CORS configuration", bucketCorsConfig.MinioBucket, err)
		}
	}

	log.Printf("[DEBUG] Updated CORS configuration for bucket: %s", bucketCorsConfig.MinioBucket)

	return minioReadBucketCors(ctx, d, meta)
}

func minioDeleteBucketCors(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketCorsConfig := BucketCorsConfig(d, meta)

	log.Printf("[DEBUG] Deleting CORS configuration for bucket: %s", bucketCorsConfig.MinioBucket)

	emptyConfig := &cors.Config{
		CORSRules: []cors.Rule{},
	}

	err := bucketCorsConfig.MinioClient.SetBucketCors(ctx, bucketCorsConfig.MinioBucket, emptyConfig)
	if err != nil {
		if isNoSuchBucketError(err) {
			log.Printf("[WARN] Bucket %s not found during CORS deletion", bucketCorsConfig.MinioBucket)
			return nil
		}
		return NewResourceError("deleting CORS configuration", bucketCorsConfig.MinioBucket, err)
	}

	log.Printf("[DEBUG] Deleted CORS configuration for bucket: %s", bucketCorsConfig.MinioBucket)

	return nil
}

func buildCorsConfig(d *schema.ResourceData) *cors.Config {
	corsRules := d.Get("cors_rule").([]interface{})
	rules := make([]cors.Rule, 0, len(corsRules))

	for _, ruleInterface := range corsRules {
		ruleMap, ok := ruleInterface.(map[string]interface{})
		if !ok {
			continue
		}

		rule := cors.Rule{}

		if v, ok := ruleMap["id"].(string); ok && v != "" {
			rule.ID = v
		}

		if v, ok := ruleMap["allowed_headers"].([]interface{}); ok && len(v) > 0 {
			rule.AllowedHeader = make([]string, len(v))
			for i, header := range v {
				if h, ok := header.(string); ok {
					rule.AllowedHeader[i] = h
				}
			}
		}

		if v, ok := ruleMap["allowed_methods"].([]interface{}); ok && len(v) > 0 {
			rule.AllowedMethod = make([]string, len(v))
			for i, method := range v {
				if m, ok := method.(string); ok {
					rule.AllowedMethod[i] = m
				}
			}
		}

		if v, ok := ruleMap["allowed_origins"].([]interface{}); ok && len(v) > 0 {
			rule.AllowedOrigin = make([]string, len(v))
			for i, origin := range v {
				if o, ok := origin.(string); ok {
					rule.AllowedOrigin[i] = o
				}
			}
		}

		if v, ok := ruleMap["expose_headers"].([]interface{}); ok && len(v) > 0 {
			rule.ExposeHeader = make([]string, len(v))
			for i, header := range v {
				if h, ok := header.(string); ok {
					rule.ExposeHeader[i] = h
				}
			}
		}

		if v, ok := ruleMap["max_age_seconds"].(int); ok && v > 0 {
			rule.MaxAgeSeconds = v
		}

		rules = append(rules, rule)
	}

	return &cors.Config{
		CORSRules: rules,
	}
}

func flattenCorsRules(rules []cors.Rule) []interface{} {
	result := make([]interface{}, 0, len(rules))

	for _, rule := range rules {
		ruleMap := make(map[string]interface{})

		if rule.ID != "" {
			ruleMap["id"] = rule.ID
		}

		if len(rule.AllowedHeader) > 0 {
			ruleMap["allowed_headers"] = rule.AllowedHeader
		}

		if len(rule.AllowedMethod) > 0 {
			ruleMap["allowed_methods"] = rule.AllowedMethod
		}

		if len(rule.AllowedOrigin) > 0 {
			ruleMap["allowed_origins"] = rule.AllowedOrigin
		}

		if len(rule.ExposeHeader) > 0 {
			ruleMap["expose_headers"] = rule.ExposeHeader
		}

		if rule.MaxAgeSeconds > 0 {
			ruleMap["max_age_seconds"] = rule.MaxAgeSeconds
		}

		result = append(result, ruleMap)
	}

	return result
}
