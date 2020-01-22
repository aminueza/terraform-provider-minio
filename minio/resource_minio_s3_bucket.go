package minio

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/minio/minio-go/v6/pkg/s3utils"
)

func resourceMinioBucket() *schema.Resource {
	return &schema.Resource{
		Create: minioCreateBucket,
		Read:   minioReadBucket,
		Update: minioUpdateBucket,
		Delete: minioDeleteBucket,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:     schema.TypeString,
				Required: true,
			},
			"acl": {
				Type:     schema.TypeString,
				Required: true,
			},
			"bucket_domain_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func minioCreateBucket(d *schema.ResourceData, meta interface{}) error {

	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Creating bucket: [%s] in region: [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	if err := s3utils.CheckValidBucketName(bucketConfig.MinioBucket); err != nil {
		return NewResourceError("Unable to create bucket", bucketConfig.MinioBucket, err)
	}

	if e, err := bucketConfig.MinioClient.BucketExists(bucketConfig.MinioBucket); err != nil {
		return NewResourceError("Unable to check bucket", bucketConfig.MinioBucket, err)
	} else if e {
		return NewResourceError("Bucket already exists!", bucketConfig.MinioBucket, err)
	}

	err := bucketConfig.MinioClient.MakeBucket(bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	if err != nil {
		log.Printf("%s", NewResourceError("Unable to create bucket", bucketConfig.MinioBucket, err))
		return NewResourceError("Unable to create bucket", bucketConfig.MinioBucket, err)
	}

	errACL := aclBucket(bucketConfig)
	if errACL != nil {
		log.Printf("%s", NewResourceError("Unable to create bucket", bucketConfig.MinioBucket, errACL))
		return NewResourceError("[ACL] Unable to create bucket", bucketConfig.MinioBucket, errACL)
	}

	log.Printf("[DEBUG] Created bucket: [%s] in region: [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)

	d.SetId(bucketConfig.MinioBucket)

	return minioReadBucket(d, meta)
}

func minioReadBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Reading bucket [%s] in region [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)

	found, err := bucketConfig.MinioClient.BucketExists(bucketConfig.MinioBucket)
	if !found {
		log.Printf("%s", NewResourceError("Unable to find bucket", bucketConfig.MinioBucket, err))
		return NewResourceError("Unable to find bucket", bucketConfig.MinioBucket, err)
	}

	log.Printf("[DEBUG] Bucket [%s] exists!", bucketConfig.MinioBucket)

	bucketURL := bucketConfig.MinioClient.EndpointURL()

	_ = d.Set("bucket_domain_name", string(bucketDomainName(bucketConfig.MinioBucket, bucketURL)))

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

	return nil
}

func minioDeleteBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)
	log.Printf("[DEBUG] Deleting bucket [%s] from region [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	if err := bucketConfig.MinioClient.RemoveBucket(bucketConfig.MinioBucket); err != nil {
		log.Printf("%s", NewResourceError("Unable to remove bucket", bucketConfig.MinioBucket, err))
		return NewResourceError("Unable to remove bucket", bucketConfig.MinioBucket, err)
	}

	log.Printf("[DEBUG] Deleted bucket: [%s] in region: [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)
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
