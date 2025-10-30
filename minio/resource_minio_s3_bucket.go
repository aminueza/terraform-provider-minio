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
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/minio/minio-go/v7"

	"github.com/minio/madmin-go/v3"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7/pkg/s3utils"
)

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
				Description:   "Prefix of the bucket",
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"bucket"},
				ValidateFunc:  validation.StringLenBetween(0, 63-id.UniqueIDSuffixLength),
			},
			"force_destroy": {
				Type:        schema.TypeBool,
				Description: "Force destroy the bucket (default: false)",
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
		},
	}
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
	var err error

	bucketConfig := BucketConfig(d, meta)
	log.Printf("[DEBUG] Deleting bucket [%s] from region [%s]", d.Id(), bucketConfig.MinioRegion)
	if err = bucketConfig.MinioClient.RemoveBucket(ctx, d.Id()); err != nil {
		if strings.Contains(err.Error(), "empty") {
			if bucketConfig.MinioForceDestroy {
				objectsCh := make(chan minio.ObjectInfo)

				// Send object names that are needed to be removed to objectsCh
				go func() {
					defer close(objectsCh)

					ctx, cancel := context.WithCancel(ctx)

					// Indicate to our routine to exit cleanly upon return.
					defer cancel()

					// List all objects from a bucket-name with a matching prefix.
					for object := range bucketConfig.MinioClient.ListObjects(ctx, d.Id(), minio.ListObjectsOptions{
						Recursive:    true,
						WithVersions: true,
					}) {
						if object.Err != nil {
							log.Fatalln(object.Err)
						}
						objectsCh <- object
					}
				}()

				errorCh := bucketConfig.MinioClient.RemoveObjects(ctx, d.Id(), objectsCh, minio.RemoveObjectsOptions{})

				if len(errorCh) > 0 {
					return NewResourceError("unable to remove bucket", d.Id(), errors.New("could not delete objects"))
				}

				return minioDeleteBucket(ctx, d, meta)
			}

		}

		log.Printf("%s", NewResourceErrorStr("unable to remove bucket", d.Id(), err))

		return NewResourceError("unable to remove bucket", d.Id(), err)
	}

	log.Printf("[DEBUG] Deleted bucket: [%s] in region: [%s]", d.Id(), bucketConfig.MinioRegion)

	_ = d.Set("bucket_domain_name", "")

	return nil

}

func minioSetBucketACL(ctx context.Context, bucketConfig *S3MinioBucket) diag.Diagnostics {

	defaultPolicies := map[string]string{
		"private":           "",
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
	return fmt.Sprintf("%s/minio/%s", bucketConfig, bucket)
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
