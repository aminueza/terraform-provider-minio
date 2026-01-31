package minio

import (
	"context"
	"fmt"
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
		Description: `Manages object lock configuration for a MinIO S3 bucket. Object locking enforces Write-Once Read-Many (WORM) immutability to protect versioned objects from deletion or modification.

This resource configures default retention settings that apply automatically to all new objects in the bucket without requiring per-object configuration.

Note: Object locking must be enabled at bucket creation. You cannot enable object locking on an existing bucket unless using MinIO RELEASE.2025-05-20T20-30-00Z or later.

Compliance standards: SEC17a-4(f), FINRA 4511(C), CFTC 1.31(c)-(d)`,

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(1, 63),
				Description:  "Name of the bucket to configure object locking. The bucket must have object locking enabled.",
			},
			"object_lock_enabled": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "Enabled",
				ValidateFunc: validation.StringInSlice([]string{"Enabled"}, false),
				Description:  "Indicates whether this bucket has an Object Lock configuration enabled. Valid value: Enabled. Defaults to Enabled.",
			},
			"rule": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: "Object Lock rule configuration for default retention",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"default_retention": {
							Type:        schema.TypeList,
							Required:    true,
							MaxItems:    1,
							Description: "Default retention period for objects in this bucket",
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"mode": {
										Type:         schema.TypeString,
										Required:     true,
										ValidateFunc: validation.StringInSlice([]string{"GOVERNANCE", "COMPLIANCE"}, false),
										Description: `Retention mode. Valid values:
- GOVERNANCE: Prevents object modification by non-privileged users. Users with s3:BypassGovernanceRetention permission can modify objects.
- COMPLIANCE: Prevents any object modification by all users, including the root user, until retention period expires.`,
									},
									"days": {
										Type:          schema.TypeInt,
										Optional:      true,
										ValidateFunc:  validation.IntAtLeast(1),
										ConflictsWith: []string{"rule.0.default_retention.0.years"},
										Description:   "Number of days for which objects should be retained. Conflicts with years.",
									},
									"years": {
										Type:          schema.TypeInt,
										Optional:      true,
										ValidateFunc:  validation.IntAtLeast(1),
										ConflictsWith: []string{"rule.0.default_retention.0.days"},
										Description:   "Number of years for which objects should be retained. Conflicts with days.",
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

	// Validate bucket object lock prerequisites
	if err := validateObjectLockPrerequisites(ctx, objectLockConfig.MinioClient, objectLockConfig.MinioBucket); err != nil {
		return diag.FromErr(err)
	}

	// Apply object lock configuration
	if err := applyObjectLockConfiguration(ctx, d, objectLockConfig.MinioClient, objectLockConfig.MinioBucket); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(objectLockConfig.MinioBucket)
	return minioReadObjectLockConfiguration(ctx, d, meta)
}

func minioReadObjectLockConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Id()

	// Check if bucket exists
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return diag.FromErr(fmt.Errorf("error checking bucket existence: %w", err))
	}
	if !exists {
		d.SetId("")
		return nil
	}

	// Get object lock configuration
	objectLockStatus, mode, validity, unit, err := client.GetObjectLockConfig(ctx, bucket)
	if err != nil {
		if strings.Contains(err.Error(), "Object Lock configuration does not exist") {
			d.SetId("")
			return nil
		}
		return diag.FromErr(fmt.Errorf("error reading object lock configuration: %w", err))
	}

	// Set simple attributes
	if err := d.Set("bucket", bucket); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("object_lock_enabled", objectLockStatus); err != nil {
		return diag.FromErr(err)
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
			return diag.FromErr(err)
		}
	}

	return nil
}

func minioUpdateObjectLockConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	objectLockConfig := BucketObjectLockConfigurationConfig(d, meta)
	objectLockConfig.MinioBucket = d.Id()

	// Validate bucket object lock prerequisites
	if err := validateObjectLockPrerequisites(ctx, objectLockConfig.MinioClient, objectLockConfig.MinioBucket); err != nil {
		return diag.FromErr(err)
	}

	// Apply updated configuration
	if err := applyObjectLockConfiguration(ctx, d, objectLockConfig.MinioClient, objectLockConfig.MinioBucket); err != nil {
		return diag.FromErr(err)
	}

	return minioReadObjectLockConfiguration(ctx, d, meta)
}

func minioDeleteObjectLockConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	objectLockConfig := BucketObjectLockConfigurationConfig(d, meta)
	objectLockConfig.MinioBucket = d.Id()

	// Clear object lock configuration by setting all parameters to nil
	err := objectLockConfig.MinioClient.SetBucketObjectLockConfig(ctx, objectLockConfig.MinioBucket, nil, nil, nil)
	if err != nil {
		return diag.FromErr(fmt.Errorf("error clearing object lock configuration: %w", err))
	}

	d.SetId("")
	return nil
}

// Helper functions

func validateObjectLockPrerequisites(ctx context.Context, client *minio.Client, bucket string) error {
	// Check if bucket exists
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("error checking bucket existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket %s does not exist", bucket)
	}

	// Check if versioning is enabled (required for object locking)
	versioning, err := client.GetBucketVersioning(ctx, bucket)
	if err != nil {
		return fmt.Errorf("error checking bucket versioning: %w", err)
	}

	if !versioning.Enabled() {
		return fmt.Errorf("bucket %s does not have versioning enabled. Object locking requires versioning to be enabled", bucket)
	}

	// Check if object lock is enabled
	objectLockStatus, _, _, _, err := client.GetObjectLockConfig(ctx, bucket)
	if err != nil {
		if strings.Contains(err.Error(), "Object Lock configuration does not exist") {
			return fmt.Errorf("bucket %s does not have object lock enabled. Object lock must be enabled when creating the bucket", bucket)
		}
		return fmt.Errorf("error checking object lock configuration: %w", err)
	}

	if objectLockStatus != "Enabled" {
		return fmt.Errorf("bucket %s does not have object lock enabled. Object lock must be enabled when creating the bucket", bucket)
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

	// Apply configuration
	err := client.SetBucketObjectLockConfig(ctx, bucket, &mode, &validity, &unit)
	if err != nil {
		return fmt.Errorf("error setting object lock configuration: %w", err)
	}

	return nil
}
