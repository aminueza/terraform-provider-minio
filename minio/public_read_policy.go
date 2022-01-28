package minio

import (
	"fmt"
	"github.com/minio/minio-go/v7/pkg/policy"

	"github.com/minio/minio-go/v7/pkg/set"
)

// PublicReadPolicy returns policy where everyone can read objects
func PublicReadPolicy(bucket *S3MinioBucket) BucketPolicy {
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "AllowAllS3Actions",
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Actions:   readListMyObjectActions,
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket), fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
			},
		},
	}
}
