package minio

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	minio "github.com/minio/minio-go/v7"
)

func resourceMinioBucketVersioning() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioPutBucketVersioning,
		ReadContext:   minioReadBucketVersioning,
		UpdateContext: minioPutBucketVersioning,
		DeleteContext: minioDeleteBucketVersioning,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Read:   schema.DefaultTimeout(2 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Description: "Name of the bucket",
				Required:    true,
				ForceNew:    true,
			},
			"versioning_configuration": {
				Type:     schema.TypeList,
				Required: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"status": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.StringInSlice([]string{minio.Enabled, minio.Suspended}, false),
						},
						"excluded_prefixes": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
						"exclude_folders": {
							Type:     schema.TypeBool,
							Optional: true,
						},
					},
				},
			},
		},
	}
}

func minioPutBucketVersioning(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketVersioningConfig := BucketVersioningConfig(d, meta)
	versioningConfig := getBucketVersioningConfig(d.Get("versioning_configuration").([]interface{}))

	if versioningConfig == nil {
		return nil
	}

	log.Printf("[DEBUG] S3 bucket: %s, put versioning configuration: %v", bucketVersioningConfig.MinioBucket, versioningConfig)

	// Wait for bucket to be ready for eventual consistency
	timeout := d.Timeout(schema.TimeoutCreate)
	if d.Id() != "" {
		timeout = d.Timeout(schema.TimeoutUpdate)
	}
	// Reserve time for the actual operation
	waitTimeout := timeout - 30*time.Second
	if waitTimeout < 30*time.Second {
		waitTimeout = 30 * time.Second
	}

	if err := waitForBucketReady(ctx, bucketVersioningConfig.MinioClient, bucketVersioningConfig.MinioBucket, waitTimeout); err != nil {
		return NewResourceError("error waiting for bucket to be ready", bucketVersioningConfig.MinioBucket, err)
	}

	// Retry SetBucketVersioning for transient NoSuchBucket errors
	err := retry.RetryContext(ctx, waitTimeout, func() *retry.RetryError {
		err := bucketVersioningConfig.MinioClient.SetBucketVersioning(
			ctx,
			bucketVersioningConfig.MinioBucket,
			convertBucketVersioningConfig(*versioningConfig),
		)
		if err != nil {
			if isNoSuchBucketError(err) {
				log.Printf("[DEBUG] Bucket %q not yet available for versioning, retrying...", bucketVersioningConfig.MinioBucket)
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return NewResourceError("error putting bucket versioning configuration", bucketVersioningConfig.MinioBucket, err)
	}

	d.SetId(bucketVersioningConfig.MinioBucket)

	return minioReadBucketVersioning(ctx, d, meta)
}

func minioReadBucketVersioning(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketVersioningConfig := BucketVersioningConfig(d, meta)

	log.Printf("[DEBUG] S3 bucket versioning, read for bucket: %s", d.Id())

	// For new resources, wait for bucket to be ready
	if d.IsNewResource() {
		timeout := d.Timeout(schema.TimeoutRead)
		if err := waitForBucketReady(ctx, bucketVersioningConfig.MinioClient, d.Id(), timeout); err != nil {
			if isNoSuchBucketError(err) {
				log.Printf("[WARN] Bucket %s not found after waiting, removing versioning resource from state", d.Id())
				d.SetId("")
				return nil
			}
			return NewResourceError("error waiting for bucket to be ready", d.Id(), err)
		}
	} else {
		// For existing resources, check if bucket exists
		exists, err := bucketVersioningConfig.MinioClient.BucketExists(ctx, d.Id())
		if err != nil {
			return diag.FromErr(fmt.Errorf("error checking bucket existence: %w", err))
		}
		if !exists {
			log.Printf("[WARN] Bucket %s no longer exists, removing versioning resource from state", d.Id())
			d.SetId("")
			return nil
		}
	}

	versioningConfig, err := bucketVersioningConfig.MinioClient.GetBucketVersioning(ctx, d.Id())
	if err != nil {
		return NewResourceError("failed to load bucket versioning", bucketVersioningConfig.MinioBucket, err)
	}

	config := make(map[string]interface{})

	if versioningConfig.Status != "" {
		config["status"] = versioningConfig.Status
	}

	config["excluded_prefixes"] = []string{}
	for _, val := range versioningConfig.ExcludedPrefixes {
		config["excluded_prefixes"] = append(config["excluded_prefixes"].([]string), val.Prefix)
	}

	config["exclude_folders"] = versioningConfig.ExcludeFolders

	if err := d.Set("bucket", d.Id()); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("versioning_configuration", []interface{}{config}); err != nil {
		return diag.FromErr(fmt.Errorf("error setting versioning configuration: %w", err))
	}

	return nil
}

func minioDeleteBucketVersioning(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketVersioningConfig := BucketVersioningConfig(d, meta)

	if v := getBucketVersioningConfig(d.Get("versioning_configuration").([]interface{})); v != nil && v.Status == minio.Suspended {
		log.Printf("[DEBUG] Removing bucket versioning for unversioned bucket (%s) from state", d.Id())
		return nil
	}

	log.Printf("[DEBUG] S3 bucket: %s, suspending versioning", bucketVersioningConfig.MinioBucket)

	err := bucketVersioningConfig.MinioClient.SuspendVersioning(ctx, bucketVersioningConfig.MinioBucket)
	if err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && minioErr.Code == "InvalidBucketState" {
			log.Printf("[WARN] S3 bucket %s: cannot suspend versioning (%s), removing from state anyway", bucketVersioningConfig.MinioBucket, minioErr.Message)
			return nil
		}
		return NewResourceError("error suspending bucket versioning", bucketVersioningConfig.MinioBucket, err)
	}

	return nil
}

func convertBucketVersioningConfig(c S3MinioBucketVersioningConfiguration) minio.BucketVersioningConfiguration {
	conf := minio.BucketVersioningConfiguration{
		Status:         c.Status,
		ExcludeFolders: c.ExcludeFolders,
	}

	for _, prefix := range c.ExcludedPrefixes {
		conf.ExcludedPrefixes = append(conf.ExcludedPrefixes, minio.ExcludedPrefix{Prefix: prefix})
	}

	return conf
}

func getBucketVersioningConfig(v []interface{}) *S3MinioBucketVersioningConfiguration {
	if len(v) == 0 || v[0] == nil {
		return nil
	}

	tfMap, ok := v[0].(map[string]interface{})
	if !ok {
		return nil
	}

	result := &S3MinioBucketVersioningConfiguration{}

	if status, ok := tfMap["status"].(string); ok && status != "" {
		result.Status = status
	}

	if excludedPrefixes, ok := tfMap["excluded_prefixes"].([]interface{}); ok {
		for _, prefix := range excludedPrefixes {
			if v, ok := prefix.(string); ok {
				result.ExcludedPrefixes = append(result.ExcludedPrefixes, v)
			}
		}
	}

	if excludeFolders, ok := tfMap["exclude_folders"].(bool); ok {
		result.ExcludeFolders = excludeFolders
	}

	return result
}
