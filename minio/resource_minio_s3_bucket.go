package minio

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/s3utils"
	"github.com/minio/minio-go/v7/pkg/tags"
)

func IsS3TaggingNotImplemented(err error) bool {
	var minioErr minio.ErrorResponse

	if errors.As(err, &minioErr) {
		return minioErr.Code == "NotImplemented"
	}

	if resp, ok := err.(*minio.ErrorResponse); ok {
		return resp.Code == "NotImplemented"
	}

	return false
}

type RetryConfig struct {
	MaxRetries  int
	MaxBackoff  time.Duration
	BackoffBase float64
}

func getRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:  6,
		MaxBackoff:  20 * time.Second,
		BackoffBase: 2.0,
	}
}

func resourceMinioBucket() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateBucket,
		ReadContext:   minioReadBucket,
		UpdateContext: minioUpdateBucket,
		DeleteContext: minioDeleteBucket,
		Importer: &schema.ResourceImporter{
			StateContext: resourceMinioS3BucketImportState,
		},

		SchemaVersion: 0,

		CustomizeDiff: customizeBucketDiff,

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:          schema.TypeString,
				Description:   "Name of the bucket",
				Optional:      true,
				Computed:      true,
				ForceNew:      true,
				ConflictsWith: []string{"bucket_prefix"},
				ValidateFunc:  validation.StringLenBetween(0, 63),
			},
			"bucket_prefix": {
				Type:          schema.TypeString,
				Description:   "Prefix of the bucket. Only used during bucket creation; ignored for existing resources.",
				Optional:      true,
				Computed:      true,
				ForceNew:      true,
				ConflictsWith: []string{"bucket"},
				ValidateFunc:  validation.StringLenBetween(0, 63-id.UniqueIDSuffixLength),
			},
			"force_destroy": {
				Type:        schema.TypeBool,
				Description: "A boolean that indicates all objects (including locked objects) should be deleted from the bucket so that the bucket can be destroyed without error. These objects are not recoverable.",
				Optional:    true,
				Default:     false,
			},
			"acl": {
				Type:        schema.TypeString,
				Description: "Bucket's Access Control List (default: private)",
				Optional:    true,
				Default:     "private",
				ForceNew:    false,
			},
			"arn": {
				Type:        schema.TypeString,
				Description: "ARN of the bucket",
				Computed:    true,
			},
			"bucket_domain_name": {
				Type:        schema.TypeString,
				Description: "The bucket domain name",
				Computed:    true,
			},
			"quota": {
				Type:        schema.TypeInt,
				Description: "Quota of the bucket",
				Optional:    true,
			},
			"object_locking": {
				Type:        schema.TypeBool,
				Description: "Enable object locking for the bucket (default: false)",
				Optional:    true,
				Default:     false,
				ForceNew:    false,
			},
			"tags": {
				Type:        schema.TypeMap,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "A map of tags to assign to the bucket",
			},
		},
	}
}

func customizeBucketDiff(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
	if d.Id() == "" {
		return nil
	}

	oldBucket, newBucket := d.GetChange("bucket")
	oldPrefix, newPrefix := d.GetChange("bucket_prefix")

	// Case 1: Switching from bucket to bucket_prefix
	if oldBucket.(string) != "" && newPrefix.(string) != "" && oldPrefix.(string) == "" {
		existingBucket := oldBucket.(string)
		prefix := newPrefix.(string)

		compatible := strings.HasPrefix(existingBucket, prefix) || prefix == existingBucket || prefix == existingBucket+"-"
		if compatible {
			log.Printf("[DEBUG] Bucket [%s] is compatible with prefix [%s], suppressing ForceNew", existingBucket, prefix)
			if err := d.SetNew("bucket", existingBucket); err != nil {
				return err
			}
			if err := d.SetNew("bucket_prefix", oldPrefix.(string)); err != nil {
				return err
			}
			return nil
		}
	}

	// Case 2: Switching from bucket_prefix to bucket (or changing bucket)
	if newBucket.(string) != "" && newBucket.(string) == d.Id() {
		if oldPrefix.(string) != "" && newPrefix.(string) == "" {
			log.Printf("[DEBUG] New bucket [%s] matches existing ID, suppressing ForceNew", newBucket.(string))
			if err := d.SetNew("bucket_prefix", oldPrefix.(string)); err != nil {
				return err
			}
			return nil
		}
	}

	return nil
}

func minioCreateBucket(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var bucket string
	var region string

	bucketConfig := BucketConfig(d, meta)

	if name := bucketConfig.MinioBucket; name != "" {
		bucket = name
	} else if prefix := bucketConfig.MinioBucketPrefix; prefix != "" {
		bucket = id.PrefixedUniqueId(prefix)
	} else {
		bucket = id.UniqueId()
	}

	if bucketConfig.MinioRegion == "" {
		region = "us-east-1"
	} else {
		region = bucketConfig.MinioRegion
	}

	log.Printf("[DEBUG] Creating bucket: [%s] in region: [%s]", bucket, region)
	if err := s3utils.CheckValidBucketName(bucket); err != nil {
		return NewResourceError("unable to create bucket", bucket, err)
	}

	if e, err := bucketConfig.MinioClient.BucketExists(ctx, bucket); err != nil {
		return NewResourceError("unable to check bucket", bucket, err)
	} else if e {
		return NewResourceError("bucket already exists!", bucket, err)
	}

	err := bucketConfig.MinioClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{
		Region:        region,
		ObjectLocking: bucketConfig.ObjectLockingEnabled,
	})

	if err != nil {
		log.Printf("%s", NewResourceErrorStr("unable to create bucket", bucket, err))
		return NewResourceError("unable to create bucket", bucket, err)
	}

	_ = d.Set("bucket", bucket)
	d.SetId(bucket)

	bucketConfig = BucketConfig(d, meta)

	if errACL := minioSetBucketACL(ctx, bucketConfig); errACL != nil {
		log.Printf("%s", NewResourceErrorStr("unable to create bucket", bucket, errACL))
		return NewResourceError("[ACL] Unable to create bucket", bucket, errACL)
	}

	log.Printf("[DEBUG] Created bucket: [%s] in region: [%s]", bucket, region)

	if v, ok := d.GetOk("tags"); ok {
		tagsMap := v.(map[string]interface{})
		bucketTags, err := tags.NewTags(convertToStringMap(tagsMap), false)
		if err != nil {
			return NewResourceError("error creating bucket tags", bucket, err)
		}

		err = bucketConfig.MinioClient.SetBucketTagging(ctx, bucket, bucketTags)
		if err != nil {
			if !IsS3TaggingNotImplemented(err) {
				return NewResourceError("error setting bucket tags", bucket, err)
			}
		}
	}

	found, err := bucketConfig.MinioClient.BucketExists(ctx, bucket)
	if err != nil {
		log.Printf("[WARNING] Error verifying bucket creation: %s", err)
	} else if !found {
		log.Printf("[WARNING] Bucket [%s] not immediately visible after creation, proceeding anyway", bucket)
	}

	return minioUpdateBucket(ctx, d, meta)
}

func minioReadBucket(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Reading bucket [%s] in region [%s]", d.Id(), bucketConfig.MinioRegion)

	// Retry logic to handle eventual consistency issues with some MinIO implementations
	// (e.g., Hetzner's MinIO may report bucket as not existing immediately after creation)
	// Use truncated exponential backoff with jitter as in AWS SDKs:
	// seconds_to_sleep_i = min(b*r^i, MAX_BACKOFF)
	// where b = random number between 0 and 1; r = 2; MAX_BACKOFF = 20 seconds for most SDKs
	var found bool
	var err error
	retryConfig := getRetryConfig()

	for i := 0; i < retryConfig.MaxRetries; i++ {
		if ctx.Err() != nil {
			return NewResourceError("context cancelled during bucket existence check", d.Id(), ctx.Err())
		}

		found, err = bucketConfig.MinioClient.BucketExists(ctx, d.Id())
		if err != nil {
			log.Printf("[ERROR] Error checking if bucket exists: %s", err)
			return NewResourceError("error checking bucket existence", d.Id(), err)
		}

		if found {
			break
		}

		if checkFound, diagErr := diagnoseMissingBucket(ctx, bucketConfig, d.Id()); diagErr != nil {
			return diagErr
		} else if checkFound {
			found = true
			break
		}

		if i < retryConfig.MaxRetries-1 {
			var jitter float64
			var randomBytes [8]byte
			if _, err := rand.Read(randomBytes[:]); err != nil {
				log.Printf("[WARNING] Failed to generate random jitter: %s", err)
				jitter = 0.5
			} else {
				jitter = float64(binary.BigEndian.Uint64(randomBytes[:])) / float64(math.MaxUint64)
			}
			backoffSeconds := jitter * math.Pow(retryConfig.BackoffBase, float64(i))
			sleep := min(time.Duration(backoffSeconds*float64(time.Second)), retryConfig.MaxBackoff)
			log.Printf("[DEBUG] Bucket [%s] not found on attempt %d/%d, retrying in %v...", d.Id(), i+1, retryConfig.MaxRetries, sleep)
			time.Sleep(sleep)
		}
	}

	if !found {
		log.Printf("[INFO] Bucket [%s] not found after %d attempts, removing from state", d.Id(), retryConfig.MaxRetries)
		d.SetId("")
		return nil
	}

	log.Printf("[DEBUG] Bucket [%s] exists!", d.Id())

	if _, ok := d.GetOk("bucket"); !ok {
		_ = d.Set("bucket", d.Id())
	}

	bucketURL := bucketConfig.MinioClient.EndpointURL()

	_ = d.Set("arn", bucketArn(d.Id()))
	_ = d.Set("bucket_domain_name", bucketDomainName(d.Id(), bucketURL))

	bucketTags, err := bucketConfig.MinioClient.GetBucketTagging(ctx, d.Id())
	if err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && minioErr.Code == "NoSuchTagSet" {
			_ = d.Set("tags", map[string]string{})
		} else if IsS3TaggingNotImplemented(err) {
			return nil
		} else {
			return NewResourceError("error reading bucket tags", d.Id(), err)
		}
	} else {
		_ = d.Set("tags", bucketTags.ToMap())
	}

	return nil
}

func minioUpdateBucket(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketConfig := BucketConfig(d, meta)

	if d.HasChange("acl") {
		log.Printf("[DEBUG] Updating bucket. Bucket: [%s], Region: [%s]",
			bucketConfig.MinioBucket, bucketConfig.MinioRegion)

		if err := minioSetBucketACL(ctx, bucketConfig); err != nil {
			log.Printf("%s", NewResourceErrorStr("unable to update bucket", bucketConfig.MinioBucket, err))
			return NewResourceError("[ACL] Unable to update bucket", bucketConfig.MinioBucket, err)
		}

		log.Printf("[DEBUG] Bucket [%s] updated!", bucketConfig.MinioBucket)
		_ = d.Set("acl", bucketConfig.MinioACL)
	}

	if d.HasChange("tags") {
		if v, ok := d.GetOk("tags"); ok && len(v.(map[string]interface{})) > 0 {
			tagsMap := v.(map[string]interface{})
			bucketTags, err := tags.NewTags(convertToStringMap(tagsMap), false)
			if err != nil {
				return NewResourceError("error creating bucket tags", d.Id(), err)
			}

			err = bucketConfig.MinioClient.SetBucketTagging(ctx, d.Id(), bucketTags)
			if err != nil {
				if !IsS3TaggingNotImplemented(err) {
					return NewResourceError("error updating bucket tags", d.Id(), err)
				}
			}
		} else {
			err := bucketConfig.MinioClient.RemoveBucketTagging(ctx, d.Id())
			if err != nil {
				if !IsS3TaggingNotImplemented(err) {
					return NewResourceError("error removing bucket tags", d.Id(), err)
				}
			}
		}
	}

	if d.HasChange("quota") {
		log.Printf("[DEBUG] Setting bucket quota")
		quotaInt := d.Get("quota").(int)
		if quotaInt < 0 {
			return diag.Errorf("bucket quota must be a non-negative value, got: %d", quotaInt)
		}
		bucketQuota := madmin.BucketQuota{Quota: uint64(quotaInt), Type: madmin.HardQuota}

		if err := bucketConfig.MinioAdmin.SetBucketQuota(ctx, bucketConfig.MinioBucket, &bucketQuota); err != nil {
			return diag.Errorf("error setting bucket quota %v: %s", bucketConfig.MinioBucket, err)
		}

		log.Printf("[DEBUG] Bucket [%s] updated!", bucketConfig.MinioBucket)
		_ = d.Set("quota", bucketQuota.Quota)
	}

	return minioReadBucket(ctx, d, meta)
}

func minioDeleteBucket(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketConfig := BucketConfig(d, meta)
	bucketName := d.Id()

	log.Printf("[DEBUG] Deleting bucket [%s] from region [%s]", bucketName, bucketConfig.MinioRegion)

	hasObjects, diagErr := bucketHasObjects(ctx, bucketConfig.MinioClient, bucketName)
	if diagErr != nil {
		return diagErr
	}

	if hasObjects {
		if !bucketConfig.MinioForceDestroy {
			return diag.Errorf(
				"bucket %q is not empty. Set force_destroy = true to delete all objects and the bucket, "+
					"or manually empty the bucket first", bucketName)
		}

		log.Printf("[DEBUG] Force destroying bucket %s - deleting all objects", bucketName)

		objectsCh := make(chan minio.ObjectInfo)
		var listErr error

		go func() {
			defer close(objectsCh)

			for object := range bucketConfig.MinioClient.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
				Recursive:    true,
				WithVersions: true,
			}) {
				if object.Err != nil {
					listErr = object.Err
					log.Printf("[ERROR] Error listing objects for deletion: %s", object.Err)
					return
				}
				objectsCh <- object
			}
		}()

		errorCh := bucketConfig.MinioClient.RemoveObjects(ctx, bucketName, objectsCh, minio.RemoveObjectsOptions{})

		for removeErr := range errorCh {
			log.Printf("[ERROR] Error deleting object %s: %s", removeErr.ObjectName, removeErr.Err)
			return NewResourceError("error deleting object during force destroy", removeErr.ObjectName, removeErr.Err)
		}

		if listErr != nil {
			return NewResourceError("error listing objects for deletion", bucketName, listErr)
		}

		log.Printf("[DEBUG] All objects deleted from bucket %s", bucketName)
	}

	if err := bucketConfig.MinioClient.RemoveBucket(ctx, bucketName); err != nil {
		log.Printf("%s", NewResourceErrorStr("unable to remove bucket", bucketName, err))
		return NewResourceError("unable to remove bucket", bucketName, err)
	}

	log.Printf("[DEBUG] Deleted bucket: [%s] in region: [%s]", bucketName, bucketConfig.MinioRegion)

	_ = d.Set("bucket_domain_name", "")

	return nil
}

func minioSetBucketACL(ctx context.Context, bucketConfig *S3MinioBucket) diag.Diagnostics {
	if bucketConfig.MinioACL == "private" {
		if err := removeBucketPolicy(ctx, bucketConfig); err != nil {
			return err
		}
		return nil
	}

	defaultPolicies := map[string]string{
		"public-write":      exportPolicyString(WriteOnlyPolicy(bucketConfig), bucketConfig.MinioBucket),
		"public-read":       exportPolicyString(ReadOnlyPolicy(bucketConfig), bucketConfig.MinioBucket),
		"public-read-write": exportPolicyString(ReadWritePolicy(bucketConfig), bucketConfig.MinioBucket),
		"public":            exportPolicyString(PublicPolicy(bucketConfig), bucketConfig.MinioBucket),
	}

	policyString, policyExists := defaultPolicies[bucketConfig.MinioACL]

	if !policyExists {
		return NewResourceError("unsupported ACL", bucketConfig.MinioACL, errors.New("(valid acl: private, public-write, public-read, public-read-write, public)"))
	}

	// Only some providers support bucket policies, so we skip setting a policy if the bucket policy is empty. See issue #608.
	if policyString != "" {
		if err := bucketConfig.MinioClient.SetBucketPolicy(ctx, bucketConfig.MinioBucket, policyString); err != nil {
			log.Printf("%s", NewResourceErrorStr("unable to set bucket policy", bucketConfig.MinioBucket, err))
			return NewResourceError("unable to set bucket policy", bucketConfig.MinioBucket, err)
		}
	}

	return nil
}

func removeBucketPolicy(ctx context.Context, bucketConfig *S3MinioBucket) diag.Diagnostics {
	if err := bucketConfig.MinioClient.SetBucketPolicy(ctx, bucketConfig.MinioBucket, ""); err != nil {
		errResp := minio.ToErrorResponse(err)

		if errResp.Code == "NoSuchBucketPolicy" || errResp.StatusCode == http.StatusNotFound {
			return nil
		}

		if errResp.Code == "NotImplemented" || errResp.StatusCode == http.StatusNotImplemented {
			log.Printf("[INFO] Backend does not support removing bucket policies, skipping removal for bucket %q: %v", bucketConfig.MinioBucket, err)
			return nil
		}

		if errResp.Code == "MethodNotAllowed" || errResp.StatusCode == http.StatusMethodNotAllowed {
			log.Printf("[INFO] Backend rejected policy removal request for bucket %q (method not allowed); assuming private policy", bucketConfig.MinioBucket)
			return nil
		}

		return NewResourceError("unable to remove bucket policy", bucketConfig.MinioBucket, err)
	}

	return nil
}

func exportPolicyString(policyStruct BucketPolicy, bucketName string) string {
	policyJSON, err := json.Marshal(policyStruct)
	if err != nil {
		log.Printf("%s", NewResourceErrorStr("unable to parse bucket policy", bucketName, err))
		return NewResourceError("unable to parse bucket policy", bucketName, err)[0].Summary
	}
	return string(policyJSON)
}

func bucketArn(bucket string) string {
	return fmt.Sprintf("%s%s", awsResourcePrefix, bucket)
}

func bucketDomainName(bucket string, bucketConfig *url.URL) string {
	return fmt.Sprintf("%s/%s", bucketConfig, bucket)
}

func validateS3BucketName(value string) error {
	if (len(value) < 3) || (len(value) > 63) {
		return fmt.Errorf("%q must contain from 3 to 63 characters", value)
	}
	if !regexp.MustCompile(`^[0-9a-z-.]+$`).MatchString(value) {
		return fmt.Errorf("only lowercase alphanumeric characters and hyphens allowed in %q", value)
	}
	if regexp.MustCompile(`^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$`).MatchString(value) {
		return fmt.Errorf("%q must not be formatted as an IP address", value)
	}
	if strings.HasPrefix(value, `.`) {
		return fmt.Errorf("%q cannot start with a period", value)
	}
	if strings.HasSuffix(value, `.`) {
		return fmt.Errorf("%q cannot end with a period", value)
	}
	if strings.Contains(value, `..`) {
		return fmt.Errorf("%q can be only one period between labels", value)
	}

	return nil
}

func diagnoseMissingBucket(ctx context.Context, bucketConfig *S3MinioBucket, bucket string) (bool, diag.Diagnostics) {
	location, err := bucketConfig.MinioClient.GetBucketLocation(ctx, bucket)
	if err == nil {
		log.Printf("[DEBUG] Bucket [%s] location %q confirmed after existence check failure", bucket, location)
		return true, nil
	}

	errResp := minio.ToErrorResponse(err)

	if isCredentialError(errResp) {
		log.Printf("%s", NewResourceErrorStr("access denied while verifying bucket", bucket, err))
		return false, NewResourceError("access denied while verifying bucket", bucket, err)
	}

	if errResp.Code == "NoSuchBucket" || errResp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	return false, NewResourceError("error verifying bucket existence", bucket, err)
}

func isCredentialError(errResp minio.ErrorResponse) bool {
	if errResp.StatusCode == http.StatusForbidden {
		return true
	}

	switch errResp.Code {
	case "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch", "InvalidSecurity", "ExpiredToken", "InvalidToken", "RequestTimeTooSkewed":
		return true
	default:
		return false
	}
}

// isNoSuchBucketError checks if the error indicates the bucket does not exist.
func isNoSuchBucketError(err error) bool {
	if err == nil {
		return false
	}

	errResp := minio.ToErrorResponse(err)
	if errResp.Code == "NoSuchBucket" || errResp.StatusCode == http.StatusNotFound {
		return true
	}

	// Fallback string check for non-standard error responses
	errStr := err.Error()
	return strings.Contains(errStr, "NoSuchBucket") || strings.Contains(errStr, "does not exist")
}

// waitForBucketReady waits for a bucket to become available for operations.
func waitForBucketReady(ctx context.Context, client *minio.Client, bucket string, timeout time.Duration) error {
	return retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		_, err := client.GetBucketLocation(ctx, bucket)
		if err == nil {
			return nil
		}

		errResp := minio.ToErrorResponse(err)

		// Fail fast on credential errors
		if isCredentialError(errResp) {
			return retry.NonRetryableError(fmt.Errorf("access denied while waiting for bucket %q: %w", bucket, err))
		}

		// Retry on NoSuchBucket for eventual consistency
		if isNoSuchBucketError(err) {
			log.Printf("[DEBUG] Bucket %q not yet available, retrying...", bucket)
			return retry.RetryableError(fmt.Errorf("bucket %q not yet available: %w", bucket, err))
		}

		// Non-retryable for other errors
		return retry.NonRetryableError(fmt.Errorf("error checking bucket %q availability: %w", bucket, err))
	})
}

// bucketHasObjects checks if a bucket contains at least one object.
// Returns (true, nil) if objects exist, (false, nil) if empty, or (false, error) on failure.
func bucketHasObjects(ctx context.Context, client *minio.Client, bucketName string) (bool, diag.Diagnostics) {
	objectsCh := client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Recursive: true,
		MaxKeys:   1,
	})

	obj, ok := <-objectsCh
	if !ok {
		return false, nil
	}
	if obj.Err != nil {
		return false, NewResourceError("error listing bucket objects", bucketName, obj.Err)
	}
	return true, nil
}
