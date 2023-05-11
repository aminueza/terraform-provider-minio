package minio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/minio/minio-go/v7"

	"github.com/minio/madmin-go"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7/pkg/s3utils"
)

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
				Optional:      true,
				Computed:      true,
				ForceNew:      true,
				ConflictsWith: []string{"bucket_prefix"},
				ValidateFunc:  validation.StringLenBetween(0, 63),
			},
			"bucket_prefix": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"bucket"},
				ValidateFunc:  validation.StringLenBetween(0, 63-id.UniqueIDSuffixLength),
			},
			"force_destroy": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"acl": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "private",
				ForceNew: false,
			},
			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"bucket_domain_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"quota": {
				Type:     schema.TypeInt,
				Optional: true,
			},
			"object_locking": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				ForceNew: false,
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

	return minioUpdateBucket(ctx, d, meta)
}

func minioReadBucket(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Reading bucket [%s] in region [%s]", d.Id(), bucketConfig.MinioRegion)

	found, err := bucketConfig.MinioClient.BucketExists(ctx, d.Id())
	if !found {
		log.Printf("%s", NewResourceErrorStr("unable to find bucket", d.Id(), err))
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
		log.Printf("[DEBUG] Updating bucket, quota changed. Bucket: [%s], Region: [%s]",
			bucketConfig.MinioBucket, bucketConfig.MinioRegion)

		bucketQuota := madmin.BucketQuota{Quota: uint64(d.Get("quota").(int)), Type: madmin.HardQuota}

		if err := minioSetBucketQuota(ctx, bucketConfig, &bucketQuota); err != nil {
			log.Printf("%s", NewResourceErrorStr("unable to update bucket", bucketConfig.MinioBucket, err))
			return NewResourceError("[Quota] Unable to update bucket", bucketConfig.MinioBucket, err)
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
						Recursive: true,
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

	if policyString != "" {
		if err := bucketConfig.MinioClient.SetBucketPolicy(ctx, bucketConfig.MinioBucket, policyString); err != nil {
			log.Printf("%s", NewResourceErrorStr("unable to set bucket policy", bucketConfig.MinioBucket, err))
			return NewResourceError("unable to set bucket policy", bucketConfig.MinioBucket, err)
		}
	}

	return nil
}

func minioSetBucketQuota(ctx context.Context, bucketConfig *S3MinioBucket, bucketQuota *madmin.BucketQuota) diag.Diagnostics {

	if !bucketQuota.IsValid() {
		return NewResourceError("invalid quota", fmt.Sprint(bucketQuota.Quota), errors.New("quota must be larger than 0"))
	}

	if err := bucketConfig.MinioAdmin.SetBucketQuota(ctx, bucketConfig.MinioBucket, bucketQuota); err != nil {
		log.Printf("%s", NewResourceErrorStr("unable to set bucket quota", bucketConfig.MinioBucket, err))
		return NewResourceError("unable to set bucket quota", bucketConfig.MinioBucket, err)
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
