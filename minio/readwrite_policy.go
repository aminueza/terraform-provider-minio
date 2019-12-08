package minio

import (
	"fmt"

	"github.com/minio/minio-go/pkg/set"
)

//ReadWritePolicy returns readonly policy
func ReadWritePolicy(bucket *MinioBucket) BucketPolicy {
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []Stmt{
			{
				Actions:   readOnlyBucketActions,
				Effect:    "Allow",
				Principal: "*",
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket)}...),
				Sid:       "ListObjectsInBucket",
			},
			{
				Actions:   allObjectActions,
				Effect:    "Allow",
				Principal: "*",
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
				Sid:       "AllObjectActions",
			},
		},
	}
}
