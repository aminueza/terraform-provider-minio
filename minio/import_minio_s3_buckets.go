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
	"time"

	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
)

func resourceMinioS3BucketImportState(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) ([]*schema.ResourceData, error) {

	if diag := minioReadBucket(ctx, d, meta); diag.HasError() {
		return nil, fmt.Errorf("could not read minio bucket")
	}

	bucketConfig := BucketConfig(d, meta)

	conn := meta.(*S3MinioClient).S3Client

	bucketObjectLocking, _, _, _, err := conn.GetObjectLockConfig(ctx, d.Id())
	object_locking := err == nil && bucketObjectLocking == "Enabled"
	_ = d.Set("object_locking", object_locking)

	pol, err := conn.GetBucketPolicy(ctx, d.Id())
	if err != nil {
		return nil, fmt.Errorf("error importing Minio S3 bucket policy: %s", err)
	}

	if pol == "" {
		_ = d.Set("acl", "private")
		return []*schema.ResourceData{d}, nil
	}

	_ = d.Set("acl", policyToACLName(bucketConfig, pol))

	return []*schema.ResourceData{d}, nil
}

func policyToACLName(bucketConfig *S3MinioBucket, pol string) string {

	defaultPolicies := map[string]string{
		"public-read":       exportPolicyString(ReadOnlyPolicy(bucketConfig), bucketConfig.MinioBucket),
		"public-write":      exportPolicyString(WriteOnlyPolicy(bucketConfig), bucketConfig.MinioBucket),
		"public-read-write": exportPolicyString(ReadWritePolicy(bucketConfig), bucketConfig.MinioBucket),
		"public":            exportPolicyString(PublicPolicy(bucketConfig), bucketConfig.MinioBucket),
	}

	for name, defaultPolicy := range defaultPolicies {
		if equivalent, err := awspolicy.PoliciesAreEquivalent(defaultPolicy, pol); err == nil && equivalent {
			return name
		}
	}

	return "private"
}

func exportPolicyString(policyStruct BucketPolicy, bucketName string) string {
	policyJSON, err := json.Marshal(policyStruct)
	if err != nil {
		log.Printf("%s", NewResourceErrorStr("unable to parse bucket policy", bucketName, err))
		return NewResourceError("unable to parse bucket policy", bucketName, err)[0].Summary
	}
	return string(policyJSON)
}

func minioReadBucket(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Reading bucket [%s] in region [%s]", d.Id(), bucketConfig.MinioRegion)

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

	if shouldSkipBucketTagging(bucketConfig) {
		preserveBucketTagsState(d)
		return nil
	}

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

func bucketArn(bucket string) string {
	return fmt.Sprintf("arn:aws:s3:::%s", bucket)
}

func bucketDomainName(bucket string, bucketURL *url.URL) string {
	return fmt.Sprintf("%s/%s", bucketURL, bucket)
}
