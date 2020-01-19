package minio

import (
	"fmt"

	"github.com/minio/minio-go/v6/pkg/set"
)

//WriteOnlyPolicy returns writeonly policy
func WriteOnlyPolicy(bucket *S3MinioBucket) BucketPolicy {
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []Stmt{
			{
				Sid:       "ListBucketAction",
				Actions:   readOnlyBucketActions,
				Effect:    "Allow",
				Principal: "*",
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket)}...),
			},
			{
				Sid:       "AllObjectActionsMyBuckets",
				Actions:   writeOnlyObjectActions,
				Effect:    "Allow",
				Principal: "*",
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
			},
		},
	}
}
