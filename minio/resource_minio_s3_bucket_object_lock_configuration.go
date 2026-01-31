package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7"
)

func resourceMinioS3BucketObjectLockConfiguration() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateObjectLockConfiguration,
		ReadContext:   minioReadObjectLockConfiguration,
		UpdateContext: minioUpdateObjectLockConfiguration,
		DeleteContext: minioDeleteObjectLockConfiguration,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Description: `Configures object lock (WORM) retention policies at the bucket level. Sets default retention that applies to all new objects automatically.

Object locking must be enabled when creating the bucket - can't add it later unless you're on MinIO RELEASE.2025-05-20T20-30-00Z+.

Useful for compliance: SEC17a-4(f), FINRA 4511(C), CFTC 1.31(c)-(d)`,

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(1, 63),
				Description:  "Bucket name. Must have object locking enabled at creation time.",
			},
			"object_lock_enabled": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "Enabled",
				ValidateFunc: validation.StringInSlice([]string{"Enabled"}, false),
				Description:  "Object lock status. Only valid value is 'Enabled'.",
			},
			"rule": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: "Retention rule configuration",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"default_retention": {
							Type:        schema.TypeList,
							Required:    true,
							MaxItems:    1,
							Description: "Default retention applied to all new objects",
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"mode": {
										Type:         schema.TypeString,
										Required:     true,
										ValidateFunc: validation.StringInSlice([]string{"GOVERNANCE", "COMPLIANCE"}, false),
										Description:  "GOVERNANCE (bypassable with permissions) or COMPLIANCE (strict, no overrides)",
									},
									"days": {
										Type:          schema.TypeInt,
										Optional:      true,
										ValidateFunc:  validation.IntAtLeast(1),
										ConflictsWith: []string{"rule.0.default_retention.0.years"},
										Description:   "Retention period in days. Mutually exclusive with years.",
									},
									"years": {
										Type:          schema.TypeInt,
										Optional:      true,
										ValidateFunc:  validation.IntAtLeast(1),
										ConflictsWith: []string{"rule.0.default_retention.0.days"},
										Description:   "Retention period in years. Mutually exclusive with days.",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func minioCreateObjectLockConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	objectLockConfig := BucketObjectLockConfigurationConfig(d, meta)

	log.Printf("[DEBUG] Creating object lock configuration for bucket: %s", objectLockConfig.MinioBucket)

	if err := validateObjectLockPrerequisites(ctx, objectLockConfig.MinioClient, objectLockConfig.MinioBucket); err != nil {
		return NewResourceError("validating object lock prerequisites", objectLockConfig.MinioBucket, err)
	}

	if err := applyObjectLockConfiguration(ctx, d, objectLockConfig.MinioClient, objectLockConfig.MinioBucket); err != nil {
		return NewResourceError("applying object lock configuration", objectLockConfig.MinioBucket, err)
	}

	d.SetId(objectLockConfig.MinioBucket)
	log.Printf("[DEBUG] Created object lock configuration for bucket: %s", objectLockConfig.MinioBucket)
	return minioReadObjectLockConfiguration(ctx, d, meta)
}

func minioReadObjectLockConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Id()

	log.Printf("[DEBUG] Reading object lock configuration for bucket: %s", bucket)

	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return NewResourceError("checking bucket existence", bucket, err)
	}
	if !exists {
		d.SetId("")
		return nil
	}

	objectLockStatus, mode, validity, unit, err := client.GetObjectLockConfig(ctx, bucket)
	if err != nil {
		if strings.Contains(err.Error(), "Object Lock configuration does not exist") {
			d.SetId("")
			return nil
		}
		return NewResourceError("reading object lock configuration", bucket, err)
	}

	if err := d.Set("bucket", bucket); err != nil {
		return NewResourceError("setting bucket", bucket, err)
	}

	if err := d.Set("object_lock_enabled", objectLockStatus); err != nil {
		return NewResourceError("setting object_lock_enabled", bucket, err)
	}

	// Build rule structure if we have retention configuration
	if mode != nil && validity != nil && unit != nil {
		defaultRetention := map[string]interface{}{
			"mode": mode.String(),
		}

		// Set either days or years based on the unit
		switch *unit {
		case minio.Days:
			defaultRetention["days"] = int(*validity)
		case minio.Years:
			defaultRetention["years"] = int(*validity)
		}

		rule := []interface{}{
			map[string]interface{}{
				"default_retention": []interface{}{defaultRetention},
			},
		}

		if err := d.Set("rule", rule); err != nil {
			return NewResourceError("setting rule", bucket, err)
		}
	}

	return nil
}

func minioUpdateObjectLockConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	objectLockConfig := BucketObjectLockConfigurationConfig(d, meta)
	objectLockConfig.MinioBucket = d.Id()

	log.Printf("[DEBUG] Updating object lock configuration for bucket: %s", objectLockConfig.MinioBucket)

	if err := validateObjectLockPrerequisites(ctx, objectLockConfig.MinioClient, objectLockConfig.MinioBucket); err != nil {
		return NewResourceError("validating object lock prerequisites", objectLockConfig.MinioBucket, err)
	}

	if err := applyObjectLockConfiguration(ctx, d, objectLockConfig.MinioClient, objectLockConfig.MinioBucket); err != nil {
		return NewResourceError("updating object lock configuration", objectLockConfig.MinioBucket, err)
	}

	log.Printf("[DEBUG] Updated object lock configuration for bucket: %s", objectLockConfig.MinioBucket)
	return minioReadObjectLockConfiguration(ctx, d, meta)
}

func minioDeleteObjectLockConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	objectLockConfig := BucketObjectLockConfigurationConfig(d, meta)
	objectLockConfig.MinioBucket = d.Id()

	log.Printf("[DEBUG] Clearing object lock configuration for bucket: %s", objectLockConfig.MinioBucket)

	err := objectLockConfig.MinioClient.SetBucketObjectLockConfig(ctx, objectLockConfig.MinioBucket, nil, nil, nil)
	if err != nil {
		return NewResourceError("clearing object lock configuration", objectLockConfig.MinioBucket, err)
	}

	log.Printf("[DEBUG] Cleared object lock configuration for bucket: %s", objectLockConfig.MinioBucket)
	d.SetId("")
	return nil
}

func validateObjectLockPrerequisites(ctx context.Context, client *minio.Client, bucket string) error {
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("error checking bucket existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket %s does not exist", bucket)
	}

	// Object locking requires versioning
	versioning, err := client.GetBucketVersioning(ctx, bucket)
	if err != nil {
		return fmt.Errorf("error checking bucket versioning: %w", err)
	}

	if !versioning.Enabled() {
		return fmt.Errorf("bucket %s does not have versioning enabled. Object locking requires versioning", bucket)
	}

	objectLockStatus, _, _, _, err := client.GetObjectLockConfig(ctx, bucket)
	if err != nil {
		if strings.Contains(err.Error(), "Object Lock configuration does not exist") {
			return fmt.Errorf("bucket %s doesn't have object lock enabled (must be set at bucket creation)", bucket)
		}
		return fmt.Errorf("error checking object lock configuration: %w", err)
	}

	if objectLockStatus != "Enabled" {
		return fmt.Errorf("bucket %s doesn't have object lock enabled", bucket)
	}

	return nil
}

func applyObjectLockConfiguration(ctx context.Context, d *schema.ResourceData, client *minio.Client, bucket string) error {
	// Check if rule is configured
	rules := d.Get("rule").([]interface{})
	if len(rules) == 0 {
		// No rule configured, clear any existing retention
		return client.SetBucketObjectLockConfig(ctx, bucket, nil, nil, nil)
	}

	// Extract retention configuration from nested structure
	rule := rules[0].(map[string]interface{})
	defaultRetentions := rule["default_retention"].([]interface{})

	if len(defaultRetentions) == 0 {
		// No default retention configured
		return client.SetBucketObjectLockConfig(ctx, bucket, nil, nil, nil)
	}

	retention := defaultRetentions[0].(map[string]interface{})
	modeStr := retention["mode"].(string)
	mode := minio.RetentionMode(modeStr)

	// Determine validity and unit
	var validity uint
	var unit minio.ValidityUnit

	if days, ok := retention["days"].(int); ok && days > 0 {
		validity = uint(days)
		unit = minio.Days
	} else if years, ok := retention["years"].(int); ok && years > 0 {
		validity = uint(years)
		unit = minio.Years
	} else {
		return fmt.Errorf("either days or years must be specified in default_retention")
	}

	log.Printf("[DEBUG] Applying object lock config to bucket %s: mode=%s, validity=%d, unit=%s", bucket, mode, validity, unit)

	err := client.SetBucketObjectLockConfig(ctx, bucket, &mode, &validity, &unit)
	if err != nil {
		return fmt.Errorf("error setting object lock configuration: %w", err)
	}

	return nil
}
