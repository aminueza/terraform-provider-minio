package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7"
)

var ValidityUnits = map[minio.ValidityUnit]bool{
	minio.Days:  true,
	minio.Years: true,
}

func resourceMinioBucketRetention() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateRetention,
		ReadContext:   minioReadRetention,
		UpdateContext: minioUpdateRetention,
		DeleteContext: minioDeleteRetention,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Description: `Manages object lock retention settings for a MinIO bucket. Object locking enforces Write-Once Read-Many (WORM) immutability to protect versioned objects from deletion.

Note: Object locking can only be enabled during bucket creation and requires versioning. You cannot enable object locking on an existing bucket.

This resource provides compliance with SEC17a-4(f), FINRA 4511(C), and CFTC 1.31(c)-(d) requirements.`,

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringLenBetween(0, 63)),
				Description:      "Name of the bucket to configure object locking. The bucket must be created with object locking enabled.",
			},
			"mode": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateDiagFunc: validateRetentionMode,
				Description: `Retention mode for the bucket. Valid values are:
                - GOVERNANCE: Prevents object modification by non-privileged users. Users with s3:BypassGovernanceRetention permission can modify objects.
                - COMPLIANCE: Prevents any object modification by all users, including the root user, until retention period expires.`,
			},
			"unit": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateDiagFunc: validateRetentionUnit,
				Description:      "Time unit for the validity period. Valid values are DAYS or YEARS.",
			},
			"validity_period": {
				Type:             schema.TypeInt,
				Required:         true,
				ValidateDiagFunc: validateValidityPeriod,
				Description:      "Duration for which objects should be retained under WORM lock, in the specified unit. Must be a positive integer.",
			},
		},
	}
}

func validateRetentionMode(v interface{}, p cty.Path) diag.Diagnostics {
	mode := minio.RetentionMode(v.(string))
	if !mode.IsValid() {
		return diag.Errorf("retention mode must be either GOVERNANCE or COMPLIANCE, got: %s", mode)
	}
	return nil
}

func validateRetentionUnit(v interface{}, p cty.Path) diag.Diagnostics {
	unit := minio.ValidityUnit(v.(string))
	if !ValidityUnits[unit] {
		return diag.Errorf("validity unit must be either DAYS or YEARS, got: %s", unit)
	}
	return nil
}
func validateValidityPeriod(v interface{}, p cty.Path) diag.Diagnostics {
	value := v.(int)
	if value < 1 {
		return diag.Errorf("validity period must be positive, got: %d", value)
	}
	return nil
}

func validateBucketObjectLock(ctx context.Context, client *minio.Client, bucket string) error {
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

	if !versioning.Enabled() { // Use the method, not the field
		return fmt.Errorf("bucket %s does not have versioning enabled. Object locking requires versioning", bucket)
	}

	// Check if object lock is enabled
	objectLock, _, _, _, err := client.GetObjectLockConfig(ctx, bucket)
	if err != nil {
		if strings.Contains(err.Error(), "Object Lock configuration does not exist") {
			return fmt.Errorf("bucket %s does not have object lock enabled. Object lock must be enabled when creating the bucket", bucket)
		}
		return fmt.Errorf("error checking object lock configuration: %w", err)
	}

	if objectLock != "Enabled" {
		return fmt.Errorf("bucket %s does not have object lock enabled. Object lock must be enabled when creating the bucket", bucket)
	}

	return nil
}

func minioCreateRetention(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Get("bucket").(string)
	var diags diag.Diagnostics

	// Validate bucket object lock status before proceeding
	if err := validateBucketObjectLock(ctx, client, bucket); err != nil {
		return diag.FromErr(err)
	}

	if hasLifecycleRules(ctx, client, bucket) {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  "Bucket has lifecycle rules configured",
			Detail: "This bucket has lifecycle management rules. Note that object expiration respects retention " +
				"settings. Objects cannot be deleted by lifecycle rules until their retention period expires.",
		})
	}

	mode := minio.RetentionMode(d.Get("mode").(string))
	unit := minio.ValidityUnit(d.Get("unit").(string))
	// Ensure validity period is non-negative before converting to uint
	validityVal := d.Get("validity_period").(int)
	if validityVal < 0 {
		log.Printf("[WARN] Negative validity period %d found, setting to 0", validityVal)
		validityVal = 0
	}
	validity := uint(validityVal)

	err := client.SetBucketObjectLockConfig(ctx, bucket, &mode, &validity, &unit)
	if err != nil {
		return diag.FromErr(fmt.Errorf("error setting bucket object lock config: %w", err))
	}

	d.SetId(bucket)

	readDiags := minioReadRetention(ctx, d, meta)
	diags = append(diags, readDiags...)
	return diags
}

func minioReadRetention(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client

	// First check if bucket still exists
	exists, err := client.BucketExists(ctx, d.Id())
	if err != nil {
		return diag.FromErr(fmt.Errorf("error checking bucket existence: %w", err))
	}
	if !exists {
		d.SetId("")
		return nil
	}

	mode, validity, unit, err := client.GetBucketObjectLockConfig(ctx, d.Id())
	if err != nil {
		// Check if the error indicates the retention config is gone
		if strings.Contains(err.Error(), "Object Lock configuration does not exist") {
			d.SetId("")
			return nil
		}
		return diag.FromErr(fmt.Errorf("error reading bucket retention config: %w", err))
	}

	// If any of the required fields are nil, the retention config is effectively gone
	if mode == nil || validity == nil || unit == nil {
		d.SetId("")
		return nil
	}

	if err := d.Set("bucket", d.Id()); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("mode", mode.String()); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("validity_period", *validity); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("unit", unit.String()); err != nil {
		return diag.FromErr(err)
	}

	return nil
}
func minioUpdateRetention(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client
	bucket := d.Id()

	// Validate bucket object lock status before proceeding
	if err := validateBucketObjectLock(ctx, client, bucket); err != nil {
		return diag.FromErr(err)
	}

	if d.HasChanges("mode", "unit", "validity_period") {
		mode := minio.RetentionMode(d.Get("mode").(string))
		unit := minio.ValidityUnit(d.Get("unit").(string))
		// Ensure validity period is non-negative before converting to uint
		validityVal := d.Get("validity_period").(int)
		if validityVal < 0 {
			log.Printf("[WARN] Negative validity period %d found, setting to 0", validityVal)
			validityVal = 0
		}
		validity := uint(validityVal)

		err := client.SetBucketObjectLockConfig(ctx, bucket, &mode, &validity, &unit)
		if err != nil {
			return diag.FromErr(fmt.Errorf("error updating bucket object lock config: %w", err))
		}
	}

	return minioReadRetention(ctx, d, meta)
}

func minioDeleteRetention(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client

	// To clear object lock config, we pass nil for all optional parameters
	err := client.SetBucketObjectLockConfig(ctx, d.Id(), nil, nil, nil)
	if err != nil {
		return diag.FromErr(fmt.Errorf("error clearing bucket object lock config: %v", err))
	}

	d.SetId("")
	return nil
}

func hasLifecycleRules(ctx context.Context, client *minio.Client, bucket string) bool {
	_, err := client.GetBucketLifecycle(ctx, bucket)
	return err == nil
}
