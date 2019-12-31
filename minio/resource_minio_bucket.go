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

	log.Printf("[DEBUG] Creating bucket: [%s] in region: [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	if err := s3utils.CheckValidBucketName(bucketConfig.MinioBucket); err != nil {
		return NewBucketError("Unable to create bucket", bucketConfig.MinioBucket)
	}

	if e, err := bucketConfig.MinioClient.BucketExists(bucketConfig.MinioBucket); err != nil {
		return NewBucketError("Unable to check bucket", bucketConfig.MinioBucket)
	} else if e {
		return NewBucketError("Bucket already exists!", bucketConfig.MinioBucket)
	}

	err := bucketConfig.MinioClient.MakeBucket(bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	if err != nil {
		log.Printf("%s", NewBucketError("Unable to create bucket", bucketConfig.MinioBucket))
		return NewBucketError("Unable to create bucket", bucketConfig.MinioBucket)
	}

	d.SetId(bucketConfig.MinioBucket)

	errACL := aclBucket(bucketConfig)
	if errACL != nil {
		log.Printf("%s", NewBucketError("Unable to create bucket", bucketConfig.MinioBucket))
		return NewBucketError("[ACL] Unable to create bucket", errACL.Error())
	}

	log.Printf("[DEBUG] Created bucket: [%s] in region: [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)

	return nil
}

func minioReadBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Reading bucket [%s] in region [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)

	found, _ := bucketConfig.MinioClient.BucketExists(bucketConfig.MinioBucket)
	if !found {
		log.Printf("%s", NewBucketError("Unable to find bucket", bucketConfig.MinioBucket))
		return NewBucketError("Unable to find bucket", bucketConfig.MinioBucket)
	}

	log.Printf("[DEBUG] Bucket [%s] exists!", bucketConfig.MinioBucket)

	return nil
}

func minioUpdateBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)

	log.Printf("[DEBUG] Updating bucket. Bucket: [%s], Region: [%s]",
		bucketConfig.MinioBucket, bucketConfig.MinioRegion)

	errACL := aclBucket(bucketConfig)
	if errACL != nil {
		log.Printf("%s", NewBucketError("Unable to update bucket", bucketConfig.MinioBucket))
		return NewBucketError("[ACL] Unable to update bucket", bucketConfig.MinioBucket)
	}

	log.Printf("[DEBUG] Bucket [%s] updated!", bucketConfig.MinioBucket)

	return nil
}

func minioDeleteBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)
	log.Printf("[DEBUG] Deleting bucket [%s] from region [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	if bucketConfig.MinioClient.RemoveBucket(bucketConfig.MinioBucket) != nil {
		log.Printf("%s", NewBucketError("Unable to remove bucket", bucketConfig.MinioBucket))
		return NewBucketError("Unable to remove bucket", bucketConfig.MinioBucket)
	}

	log.Printf("[DEBUG] Deleted bucket: [%s] in region: [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)

	return nil
}

func aclBucket(bucketConfig *MinioBucket) error {

	defaultPolicies := map[string]string{
		"private":           exportPolicyString(PrivatePolicy(bucketConfig)),
		"public-write":      exportPolicyString(WriteOnlyPolicy(bucketConfig)),
		"public-read":       exportPolicyString(ReadOnlyPolicy(bucketConfig)),
		"public-read-write": exportPolicyString(ReadWritePolicy(bucketConfig)),
		"public":            exportPolicyString(PublicPolicy(bucketConfig)),
	}

	policyString, policyExists := defaultPolicies[bucketConfig.MinioACL]

	log.Print(policyExists)

	if !policyExists {
		return NewBucketError("Unsuported ACL", bucketConfig.MinioACL)
	}

	// if !findValuePolicies(bucketConfig) {
	// 	if err := bucketConfig.MinioAdmin.AddCannedPolicy(bucketConfig.MinioACL, string(policyString)); err != nil {
	// 		log.Printf("%s", NewBucketError("Unable to add policy", bucketConfig.MinioACL))
	// 		return NewBucketError("Unable to add policy", policyString)
	// 	}
	// }

	if err := bucketConfig.MinioClient.SetBucketPolicy(bucketConfig.MinioBucket, policyString); err != nil {
		log.Printf("%s", NewBucketError("Unable to set bucket policy", bucketConfig.MinioBucket))
		return NewBucketError("Unable to set bucket policy", err.Error())
	}

	return nil
}

func findValuePolicies(bucketConfig *MinioBucket) bool {
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
