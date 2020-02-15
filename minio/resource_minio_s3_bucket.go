package minio

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/minio/minio-go/v6/pkg/s3utils"
)

func resourceMinioBucket() *schema.Resource {
	return &schema.Resource{
		Create: minioCreateBucket,
		Read:   minioReadBucket,
		Update: minioUpdateBucket,
		Delete: minioDeleteBucket,
		Importer: &schema.ResourceImporter{
			State: resourceMinioS3BucketImportState,
		},

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
				ValidateFunc:  validation.StringLenBetween(0, 63-resource.UniqueIDSuffixLength),
			},
			"force_destroy": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"acl": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"bucket_domain_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func minioCreateBucket(d *schema.ResourceData, meta interface{}) error {

	var bucket string
	var region string

	bucketConfig := BucketConfig(d, meta)

	if name := bucketConfig.MinioBucket; name != "" {
		bucket = name
	} else if prefix := bucketConfig.MinioBucketPrefix; prefix != "" {
		bucket = resource.PrefixedUniqueId(prefix)
	} else {
		bucket = resource.UniqueId()
	}

	if bucketConfig.MinioRegion == "" {
		region = "us-east-1"
	} else {
		region = bucketConfig.MinioRegion
	}

	log.Printf("[DEBUG] Creating bucket: [%s] in region: [%s]", bucket, region)
	if err := s3utils.CheckValidBucketName(bucket); err != nil {
		return NewResourceError("Unable to create bucket", bucket, err)
	}

	if e, err := bucketConfig.MinioClient.BucketExists(bucket); err != nil {
		return NewResourceError("Unable to check bucket", bucket, err)
	} else if e {
		return NewResourceError("Bucket already exists!", bucket, err)
	}

	err := bucketConfig.MinioClient.MakeBucket(bucket, region)
	if err != nil {
		log.Printf("%s", NewResourceError("Unable to create bucket", bucket, err))
		return NewResourceError("Unable to create bucket", bucket, err)
	}

	errACL := aclBucket(bucketConfig)
	if errACL != nil {
		log.Printf("%s", NewResourceError("Unable to create bucket", bucket, errACL))
		return NewResourceError("[ACL] Unable to create bucket", bucket, errACL)
	}

	log.Printf("[DEBUG] Created bucket: [%s] in region: [%s]", bucket, region)

	d.SetId(bucket)

	return minioUpdateBucket(d, meta)
}

func minioReadBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Reading bucket [%s] in region [%s]", d.Id(), bucketConfig.MinioRegion)

	found, err := bucketConfig.MinioClient.BucketExists(d.Id())
	if !found {
		log.Printf("%s", NewResourceError("Unable to find bucket", d.Id(), err))
		return NewResourceError("Unable to find bucket", d.Id(), err)
	}

	log.Printf("[DEBUG] Bucket [%s] exists!", d.Id())

	bucketURL := bucketConfig.MinioClient.EndpointURL()

	_ = d.Set("bucket_domain_name", string(bucketDomainName(d.Id(), bucketURL)))

	return nil
}

func minioUpdateBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Updating bucket. Bucket: [%s], Region: [%s]",
		bucketConfig.MinioBucket, bucketConfig.MinioRegion)

	errACL := aclBucket(bucketConfig)
	if errACL != nil {
		log.Printf("%s", NewResourceError("Unable to update bucket", bucketConfig.MinioBucket, errACL))
		return NewResourceError("[ACL] Unable to update bucket", bucketConfig.MinioBucket, errACL)
	}

	log.Printf("[DEBUG] Bucket [%s] updated!", bucketConfig.MinioBucket)

	return minioReadBucket(d, meta)
}

func minioDeleteBucket(d *schema.ResourceData, meta interface{}) error {
	var err error

	bucketConfig := BucketConfig(d, meta)
	log.Printf("[DEBUG] Deleting bucket [%s] from region [%s]", d.Id(), bucketConfig.MinioRegion)
	if err = bucketConfig.MinioClient.RemoveBucket(d.Id()); err != nil {
		if strings.Contains(err.Error(), "empty") {
			if bucketConfig.MinioForceDestroy {
				objectsCh := make(chan string)

				// Send object names that are needed to be removed to objectsCh
				go func() {
					defer close(objectsCh)

					doneCh := make(chan struct{})

					// Indicate to our routine to exit cleanly upon return.
					defer close(doneCh)

					// List all objects from a bucket-name with a matching prefix.
					for object := range bucketConfig.MinioClient.ListObjects(d.Id(), "", true, doneCh) {
						if object.Err != nil {
							log.Fatalln(object.Err)
						}
						objectsCh <- object.Key
					}
				}()

				errorCh := bucketConfig.MinioClient.RemoveObjects(d.Id(), objectsCh)

				if len(errorCh) > 0 {
					return NewResourceError("Unable to remove bucket", d.Id(), errors.New("Could not delete objects"))
				}

				return minioDeleteBucket(d, meta)
			}

		}

		log.Printf("%s", NewResourceError("Unable to remove bucket", d.Id(), err))

		return NewResourceError("Unable to remove bucket", d.Id(), err)
	}

	log.Printf("[DEBUG] Deleted bucket: [%s] in region: [%s]", d.Id(), bucketConfig.MinioRegion)

	_ = d.Set("bucket_domain_name", "")

	return nil

}

func aclBucket(bucketConfig *S3MinioBucket) error {

	defaultPolicies := map[string]string{
		"private":           "none", //private is set by minio default
		"public-write":      exportPolicyString(WriteOnlyPolicy(bucketConfig), bucketConfig.MinioBucket),
		"public-read":       exportPolicyString(ReadOnlyPolicy(bucketConfig), bucketConfig.MinioBucket),
		"public-read-write": exportPolicyString(ReadWritePolicy(bucketConfig), bucketConfig.MinioBucket),
		"public":            exportPolicyString(PublicPolicy(bucketConfig), bucketConfig.MinioBucket),
	}

	policyString, policyExists := defaultPolicies[bucketConfig.MinioACL]

	if !policyExists {
		return NewResourceError("Unsuported ACL", bucketConfig.MinioACL, errors.New("(valid acl: private, public-write, public-read, public-read-write, public)"))
	}

	if policyString != "none" {
		if err := bucketConfig.MinioClient.SetBucketPolicy(bucketConfig.MinioBucket, policyString); err != nil {
			log.Printf("%s", NewResourceError("Unable to set bucket policy", bucketConfig.MinioBucket, err))
			return NewResourceError("Unable to set bucket policy", bucketConfig.MinioBucket, err)
		}
	}

	return nil
}

func findValuePolicies(bucketConfig *S3MinioBucket) bool {
	policies, _ := bucketConfig.MinioAdmin.ListCannedPolicies()
	for key := range policies {
		value := string(key)
		if value == bucketConfig.MinioACL {
			return true
		}
	}
	return false
}

func exportPolicyString(policyStruct BucketPolicy, bucketName string) string {
	policyJSON, err := json.Marshal(policyStruct)
	if err != nil {
		log.Printf("%s", NewResourceError("Unable to parse bucket policy", bucketName, err))
		return NewResourceError("Unable to parse bucket policy", bucketName, err).Error()
	}
	return string(policyJSON)
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
