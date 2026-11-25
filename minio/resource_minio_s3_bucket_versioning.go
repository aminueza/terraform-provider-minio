package minio

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strings"
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
				Type:        schema.TypeList,
				Required:    true,
				MaxItems:    1,
				Description: "Versioning configuration for the bucket.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"status": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.StringInSlice([]string{minio.Enabled, minio.Suspended}, false),
							Description:  "Versioning status: Enabled or Suspended.",
						},
						"excluded_prefixes": {
							Type:        schema.TypeList,
							Optional:    true,
							Description: "List of object key prefixes to exclude from versioning.",
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
						"exclude_folders": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Whether to exclude folders from versioning.",
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

	tflog.Debug(ctx, fmt.Sprintf("S3 bucket: %s, put versioning configuration: %v", bucketVersioningConfig.MinioBucket, versioningConfig))

	timeout := d.Timeout(schema.TimeoutCreate)
	if d.Id() != "" {
		timeout = d.Timeout(schema.TimeoutUpdate)
	}
	waitTimeout := timeout - 30*time.Second
	if waitTimeout < 30*time.Second {
		waitTimeout = 30 * time.Second
	}

	if err := waitForBucketReady(ctx, bucketVersioningConfig.MinioClient, bucketVersioningConfig.MinioBucket, waitTimeout); err != nil {
		return NewResourceError("error waiting for bucket to be ready", bucketVersioningConfig.MinioBucket, err)
	}

	// MinIO multi-drive deployments can lose the bucket policy when versioning
	// is set immediately after it. Capture the policy before the write so a
	// loss caused by SetBucketVersioning can be restored (issue #1389). The
	// read retries NoSuchBucket like the write below: with a configured region
	// GetBucketLocation is answered locally and does not prove the bucket is
	// visible yet.
	var policyBefore string
	beforeErr := retry.RetryContext(ctx, waitTimeout, func() *retry.RetryError {
		policy, err := bucketVersioningConfig.MinioClient.GetBucketPolicy(ctx, bucketVersioningConfig.MinioBucket)
		if err != nil {
			if isNoSuchBucketError(err) {
				tflog.Debug(ctx, fmt.Sprintf("Bucket %q not yet available for the policy snapshot, retrying...", bucketVersioningConfig.MinioBucket))
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}
		policyBefore = policy
		return nil
	})
	if beforeErr != nil {
		// The snapshot is a safety check the provider does for itself, not a
		// feature the configuration asked for. Credentials without
		// s3:GetBucketPolicy (AccessDenied) and backends without the policy
		// API (501) cannot lose a policy to this write, so they skip the check
		// with a warning and keep working as they did before the check
		// existed. Any other failure aborts before the write instead of
		// running unprotected.
		errResp := minio.ToErrorResponse(beforeErr)
		skipCheck := errResp.Code == "AccessDenied" || errResp.StatusCode == http.StatusForbidden ||
			errResp.Code == "NotImplemented" || errResp.StatusCode == http.StatusNotImplemented ||
			isS3CompatNotSupported(meta.(*S3MinioClient), s3Compat405IsMissingFeature, beforeErr)
		if !skipCheck {
			return NewResourceError("reading bucket policy before setting versioning", bucketVersioningConfig.MinioBucket, beforeErr)
		}
		tflog.Warn(ctx, fmt.Sprintf("Bucket %s: cannot read the policy before setting versioning, skipping the policy-loss check: %v", bucketVersioningConfig.MinioBucket, beforeErr))
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
				tflog.Debug(ctx, fmt.Sprintf("Bucket %q not yet available for versioning, retrying...", bucketVersioningConfig.MinioBucket))
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return NewResourceError("error putting bucket versioning configuration", bucketVersioningConfig.MinioBucket, s3CompatWriteError(meta.(*S3MinioClient), "bucket versioning configuration", err))
	}

	// The write succeeded: set the id before the verify/restore block so a
	// failed restore leaves the resource in state, tainted by the error,
	// instead of losing versioning that is live on the server.
	d.SetId(bucketVersioningConfig.MinioBucket)

	// A failed verification read counts as lost: re-applying the captured
	// policy is idempotent, and skipping the restore would silently drop the
	// protection this workaround exists for.
	if strings.TrimSpace(policyBefore) != "" && strings.TrimSpace(policyBefore) != "{}" {
		time.Sleep(500 * time.Millisecond)
		policyAfter, afterErr := bucketVersioningConfig.MinioClient.GetBucketPolicy(ctx, bucketVersioningConfig.MinioBucket)
		if afterErr != nil || strings.TrimSpace(policyAfter) == "" || strings.TrimSpace(policyAfter) == "{}" {
			tflog.Warn(ctx, fmt.Sprintf("Bucket %s policy was lost after SetBucketVersioning, restoring", bucketVersioningConfig.MinioBucket))
			if restoreErr := bucketVersioningConfig.MinioClient.SetBucketPolicy(ctx, bucketVersioningConfig.MinioBucket, policyBefore); restoreErr != nil {
				return NewResourceError("restoring bucket policy after setting versioning", bucketVersioningConfig.MinioBucket, restoreErr)
			}
		}
	}

	return minioReadBucketVersioning(ctx, d, meta)
}

func minioReadBucketVersioning(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketVersioningConfig := BucketVersioningConfig(d, meta)

	tflog.Debug(ctx, fmt.Sprintf("S3 bucket versioning, read for bucket: %s", d.Id()))

	if d.IsNewResource() {
		timeout := d.Timeout(schema.TimeoutRead)
		if err := waitForBucketReady(ctx, bucketVersioningConfig.MinioClient, d.Id(), timeout); err != nil {
			if isNoSuchBucketError(err) {
				tflog.Warn(ctx, fmt.Sprintf("Bucket %s not found after waiting, removing versioning resource from state", d.Id()))
				d.SetId("")
				return nil
			}
			return NewResourceError("error waiting for bucket to be ready", d.Id(), err)
		}
	} else {
		exists, err := bucketVersioningConfig.MinioClient.BucketExists(ctx, d.Id())
		if err != nil {
			return NewResourceError("checking bucket existence", d.Id(), err)
		}
		if !exists {
			tflog.Warn(ctx, fmt.Sprintf("Bucket %s no longer exists, removing versioning resource from state", d.Id()))
			d.SetId("")
			return nil
		}
	}

	var versioningConfig minio.BucketVersioningConfiguration

	cfg, readErr := bucketVersioningConfig.MinioClient.GetBucketVersioning(ctx, d.Id())
	if readErr != nil {
		if s3CompatReadUnsupported(ctx, meta.(*S3MinioClient), s3Compat405IsMissingFeature, d, "bucket versioning configuration", readErr) {
			return nil
		}
		return NewResourceError("failed to load bucket versioning", bucketVersioningConfig.MinioBucket, readErr)
	}
	versioningConfig = cfg

	if versioningConfig.Status == "" {
		retryTimeout := 5 * time.Second
		if d.IsNewResource() {
			retryTimeout = d.Timeout(schema.TimeoutRead)
		}
		retryErr := retry.RetryContext(ctx, retryTimeout, func() *retry.RetryError {
			cfg, err := bucketVersioningConfig.MinioClient.GetBucketVersioning(ctx, d.Id())
			if err != nil {
				if isNoSuchBucketError(err) && d.IsNewResource() {
					return retry.RetryableError(err)
				}
				return retry.NonRetryableError(err)
			}
			if cfg.Status == "" {
				return retry.RetryableError(fmt.Errorf("versioning status not yet available for bucket %s", d.Id()))
			}
			versioningConfig = cfg
			return nil
		})
		if retryErr != nil && d.IsNewResource() {
			return NewResourceError("failed to load bucket versioning", bucketVersioningConfig.MinioBucket, retryErr)
		}
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
		return NewResourceError("setting bucket attribute", d.Id(), err)
	}

	if err := d.Set("versioning_configuration", []interface{}{config}); err != nil {
		return NewResourceError("setting versioning configuration", d.Id(), err)
	}

	return nil
}

func minioDeleteBucketVersioning(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketVersioningConfig := BucketVersioningConfig(d, meta)

	if v := getBucketVersioningConfig(d.Get("versioning_configuration").([]interface{})); v != nil && v.Status == minio.Suspended {
		tflog.Debug(ctx, fmt.Sprintf("Removing bucket versioning for unversioned bucket (%s) from state", d.Id()))
		return nil
	}

	tflog.Debug(ctx, fmt.Sprintf("S3 bucket: %s, suspending versioning", bucketVersioningConfig.MinioBucket))

	err := bucketVersioningConfig.MinioClient.SuspendVersioning(ctx, bucketVersioningConfig.MinioBucket)
	if err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && minioErr.Code == "InvalidBucketState" {
			tflog.Warn(ctx, fmt.Sprintf("S3 bucket %s: cannot suspend versioning (%s), removing from state anyway", bucketVersioningConfig.MinioBucket, minioErr.Message))
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
