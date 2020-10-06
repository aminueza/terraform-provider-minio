package minio

import (
	"fmt"

	"github.com/minio/minio-go/v7/pkg/set"
)

//ReadOnlyPolicy returns readonly policy
func ReadOnlyPolicy(bucket *S3MinioBucket) BucketPolicy {
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []Stmt{
			{
				Sid:       "ListAllBucket",
				Actions:   readOnlyAllBucketsActions,
				Effect:    "Allow",
				Principal: "*",
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s*", awsResourcePrefix)}...),
			},
			{
				Sid:       "AllObjectActionsMyBuckets",
				Actions:   readListMyObjectActions,
				Effect:    "Allow",
				Principal: "*",
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket), fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
			},
		},
	}
}
