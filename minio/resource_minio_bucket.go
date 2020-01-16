package minio

import (
	"encoding/json"
	"log"

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
		},
	}
}

func minioCreateBucket(d *schema.ResourceData, meta interface{}) error {

	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Creating bucket: [%s] in region: [%s]", bucketConfig.S3MinioBucket, bucketConfig.MinioRegion)
	if err := s3utils.CheckValidBucketName(bucketConfig.S3MinioBucket); err != nil {
		return NewBucketError("Unable to create bucket", bucketConfig.S3MinioBucket)
	}

	if e, err := bucketConfig.MinioClient.BucketExists(bucketConfig.S3MinioBucket); err != nil {
		return NewBucketError("Unable to check bucket", bucketConfig.S3MinioBucket)
	} else if e {
		return NewBucketError("Bucket already exists!", bucketConfig.S3MinioBucket)
	}

	err := bucketConfig.MinioClient.MakeBucket(bucketConfig.S3MinioBucket, bucketConfig.MinioRegion)
	if err != nil {
		log.Printf("%s", NewBucketError("Unable to create bucket", bucketConfig.S3MinioBucket))
		return NewBucketError("Unable to create bucket", bucketConfig.S3MinioBucket)
	}

	d.SetId(bucketConfig.S3MinioBucket)

	errACL := aclBucket(bucketConfig)
	if errACL != nil {
		log.Printf("%s", NewBucketError("Unable to create bucket", bucketConfig.S3MinioBucket))
		return NewBucketError("[ACL] Unable to create bucket", errACL.Error())
	}

	log.Printf("[DEBUG] Created bucket: [%s] in region: [%s]", bucketConfig.S3MinioBucket, bucketConfig.MinioRegion)

	return nil
}

func minioReadBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Reading bucket [%s] in region [%s]", bucketConfig.S3MinioBucket, bucketConfig.MinioRegion)

	found, _ := bucketConfig.MinioClient.BucketExists(bucketConfig.S3MinioBucket)
	if !found {
		log.Printf("%s", NewBucketError("Unable to find bucket", bucketConfig.S3MinioBucket))
		return NewBucketError("Unable to find bucket", bucketConfig.S3MinioBucket)
	}

	log.Printf("[DEBUG] Bucket [%s] exists!", bucketConfig.S3MinioBucket)

	return nil
}

func minioUpdateBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Updating bucket. Bucket: [%s], Region: [%s]",
		bucketConfig.S3MinioBucket, bucketConfig.MinioRegion)

	errACL := aclBucket(bucketConfig)
	if errACL != nil {
		log.Printf("%s", NewBucketError("Unable to update bucket", bucketConfig.S3MinioBucket))
		return NewBucketError("[ACL] Unable to update bucket", bucketConfig.S3MinioBucket)
	}

	log.Printf("[DEBUG] Bucket [%s] updated!", bucketConfig.S3MinioBucket)

	return nil
}

func minioDeleteBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)
	log.Printf("[DEBUG] Deleting bucket [%s] from region [%s]", bucketConfig.S3MinioBucket, bucketConfig.MinioRegion)
	if bucketConfig.MinioClient.RemoveBucket(bucketConfig.S3MinioBucket) != nil {
		log.Printf("%s", NewBucketError("Unable to remove bucket", bucketConfig.S3MinioBucket))
		return NewBucketError("Unable to remove bucket", bucketConfig.S3MinioBucket)
	}

	log.Printf("[DEBUG] Deleted bucket: [%s] in region: [%s]", bucketConfig.S3MinioBucket, bucketConfig.MinioRegion)

	return nil
}

func aclBucket(bucketConfig *S3MinioBucket) error {

	defaultPolicies := map[string]string{
		"private":           "none", //private is set by minio default
		"public-write":      exportPolicyString(WriteOnlyPolicy(bucketConfig)),
		"public-read":       exportPolicyString(ReadOnlyPolicy(bucketConfig)),
		"public-read-write": exportPolicyString(ReadWritePolicy(bucketConfig)),
		"public":            exportPolicyString(PublicPolicy(bucketConfig)),
	}

	policyString, policyExists := defaultPolicies[bucketConfig.MinioACL]

	if !policyExists {
		return NewBucketError("Unsuported ACL", bucketConfig.MinioACL)
	}

	if policyString != "none" {
		if err := bucketConfig.MinioClient.SetBucketPolicy(bucketConfig.S3MinioBucket, policyString); err != nil {
			log.Printf("%s", NewBucketError("Unable to set bucket policy", bucketConfig.S3MinioBucket))
			return NewBucketError("Unable to set bucket policy", err.Error())
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

func exportPolicyString(policyStruct BucketPolicy) string {
	policyJSON, err := json.Marshal(policyStruct)
	if err != nil {
		log.Printf("%s", NewBucketError("Unable to parse bucket policy", err.Error()))
		return NewBucketError("Unable to parse bucket policy", err.Error()).Error()
	}
	return string(policyJSON)
}
