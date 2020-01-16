package minio

import (
	"fmt"

	"github.com/minio/minio-go/v6/pkg/set"
)

//ReadWritePolicy returns readonly policy
func ReadWritePolicy(bucket *S3MinioBucket) BucketPolicy {
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []Stmt{
			{
				Sid:       "ListObjectsInBucket",
				Actions:   readOnlyBucketActions,
				Effect:    "Allow",
				Principal: "*",
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.S3MinioBucket)}...),
			},
			{
				Sid:       "UploadObjectActions",
				Actions:   uploadObjectActions,
				Effect:    "Allow",
				Principal: "*",
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.S3MinioBucket)}...),
			},
		},
	}
}
