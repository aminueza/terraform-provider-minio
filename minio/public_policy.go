package minio

import (
	"fmt"

	"github.com/minio/minio-go/v7/pkg/policy"

	"github.com/minio/minio-go/v7/pkg/set"
)

// PublicPolicy returns policy where everyone can fully list/modify objects
func PublicPolicy(bucket *S3MinioBucket) BucketPolicy {
	bucketResource := fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket)
	objectResource := fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)

	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "ListBucketActions",
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Actions:   publicBucketActions,
				Resources: set.CreateStringSet(bucketResource),
			},
			{
				Sid:       "AllObjectActions",
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Actions:   publicObjectActions,
				Resources: set.CreateStringSet(objectResource),
			},
		},
	}
}
