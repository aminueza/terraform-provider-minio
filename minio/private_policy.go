package minio

import (
	"fmt"

	"github.com/minio/minio-go/v6/pkg/set"
)

//PrivatePolicy returns readonly policy
func PrivatePolicy(bucket *MinioBucket) BucketPolicy {
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []Stmt{
			{
				Sid:       "DenyAllS3Actions",
				Effect:    "Deny",
				Principal: "*",
				Actions:   allBucketActions,
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket), fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
			},
		},
	}
}
