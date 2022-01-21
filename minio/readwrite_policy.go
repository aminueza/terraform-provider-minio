package minio

import (
	"fmt"
	"github.com/minio/minio-go/v7/pkg/policy"

	"github.com/minio/minio-go/v7/pkg/set"
)

//ReadWritePolicy returns readonly policy
func ReadWritePolicy(bucket *S3MinioBucket) BucketPolicy {
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "ListObjectsInBucket",
				Actions:   readOnlyBucketActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket)}...),
			},
			{
				Sid:       "UploadObjectActions",
				Actions:   uploadObjectActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
			},
		},
	}
}
