package minio

import (
	"fmt"

	"github.com/minio/minio-go/v6/pkg/set"
)

//PublicPolicy returns readonly policy
func PublicPolicy(bucket *S3MinioBucket) BucketPolicy {
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []Stmt{
			{
				Sid:       "AllowAllS3Actions",
				Effect:    "Allow",
				Principal: "*",
				Actions:   allBucketActions,
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.S3MinioBucket), fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.S3MinioBucket)}...),
			},
		},
	}
}
