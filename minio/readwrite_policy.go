package minio

import (
	"fmt"

	"github.com/minio/minio-go/v7/pkg/policy"

	"github.com/minio/minio-go/v7/pkg/set"
)

// ReadWritePolicy returns a policy where objects can be uploaded and read
func ReadWritePolicy(bucket *S3MinioBucket) BucketPolicy {
	bucketResource := fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket)
	objectResource := fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)

	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "ListObjectsInBucket",
				Actions:   readOnlyBucketActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet(bucketResource),
			},
			{
				Sid:       "UploadObjectActions",
				Actions:   uploadObjectActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet(objectResource),
			},
		},
	}
}
