package minio

import (
	"fmt"
	"log"

	"github.com/hashicorp/terraform/helper/schema"
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
			"debug": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
	}
}

func minioCreateBucket(d *schema.ResourceData, meta interface{}) error {

	bucketConfig := BucketConfig(d, meta)
	debubBool := bucketConfig.MinioDebug

	if debubBool {
		log.Printf("[DEBUG] Creating bucket: [%s] in region: [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	}

	err := bucketConfig.MinioClient.MakeBucket(bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	if err != nil {
		log.Printf("%s", NewBucketError("Unable to create bucket", bucketConfig.MinioBucket))
		return NewBucketError("Unable to create bucket", bucketConfig.MinioBucket)
	}
	// errACL := aclBucket(bucketConfig)
	// if errACL != nil {
	// 	log.Printf("%s", NewBucketError("Unable to create bucket", bucketConfig.MinioBucket))
	// 	return NewBucketError("[ACL] Unable to create bucket", bucketConfig.MinioBucket)
	// }

	if debubBool {
		log.Printf("[DEBUG] Created bucket: [%s] in region: [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	}
	return nil
}

func minioReadBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)
	debubBool := bucketConfig.MinioDebug

	if debubBool {
		log.Printf("[DEBUG] Reading bucket [%s] in region [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	}

	found, _ := bucketConfig.MinioClient.BucketExists(bucketConfig.MinioBucket)
	if !found {
		log.Printf("%s", NewBucketError("Unable to find bucket", bucketConfig.MinioBucket))
		return NewBucketError("Unable to find bucket", bucketConfig.MinioBucket)
	}

	if debubBool {
		log.Printf("[DEBUG] Bucket [%s] exists!", bucketConfig.MinioBucket)
	}
	return nil
}

func minioUpdateBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)
	debubBool := bucketConfig.MinioDebug
	if debubBool {
		log.Printf("[DEBUG] Updating bucket. Bucket: [%s], Region: [%s]",
			bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	}

	// errACL := aclBucket(bucketConfig)
	// if errACL != nil {
	// 	log.Printf("%s", NewBucketError("Unable to update bucket", bucketConfig.MinioBucket))
	// 	return NewBucketError("[ACL] Unable to update bucket", bucketConfig.MinioBucket)
	// }

	if debubBool {
		log.Printf("[DEBUG] Bucket [%s] updated!", bucketConfig.MinioBucket)
	}
	return nil
}

func minioDeleteBucket(d *schema.ResourceData, meta interface{}) error {
	bucketConfig := BucketConfig(d, meta)
	debubBool := bucketConfig.MinioDebug
	if debubBool {
		log.Printf("[DEBUG] Deleting bucket [%s] from region [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	}
	if bucketConfig.MinioClient.RemoveBucket(bucketConfig.MinioBucket) != nil {
		log.Printf("%s", NewBucketError("Unable to remove bucket", bucketConfig.MinioBucket))
		return NewBucketError("Unable to remove bucket", bucketConfig.MinioBucket)
	}

	if debubBool {
		log.Printf("[DEBUG] Deleted bucket: [%s] in region: [%s]", bucketConfig.MinioBucket, bucketConfig.MinioRegion)
	}

	return nil
}

func aclBucket(bucketConfig *MinioBucket) error {
	var err error
	switch bucketConfig.MinioACL {
	case "private":
		policy := fmt.Sprintf("%#v", PrivatePolicy(bucketConfig))
		err = bucketConfig.MinioClient.SetBucketPolicy(bucketConfig.MinioBucket, policy)
	// case "public-write":
	// 	policy := fmt.Sprintf("%#v", ReadOnlyPolicy(bucketConfig))
	// 	err = bucketConfig.MinioClient.SetBucketPolicy(bucketConfig.MinioBucket, policy)
	case "public-read":
		policy := fmt.Sprintf("%#v", ReadOnlyPolicy(bucketConfig))
		err = bucketConfig.MinioClient.SetBucketPolicy(bucketConfig.MinioBucket, policy)
	case "public-read-write":
		policy := fmt.Sprintf("%#v", ReadWritePolicy(bucketConfig))
		err = bucketConfig.MinioClient.SetBucketPolicy(bucketConfig.MinioBucket, policy)
	case "public":
		policy := fmt.Sprintf("%#v", PublicPolicy(bucketConfig))
		err = bucketConfig.MinioClient.SetBucketPolicy(bucketConfig.MinioBucket, policy)
	default:
		err = NewBucketError("Unsuported ACL", bucketConfig.MinioACL)
	}
	return err
}
