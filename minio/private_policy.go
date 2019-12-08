package minio

import (
	"fmt"

	"github.com/minio/minio-go/pkg/set"
)

//PrivatePolicy returns readonly policy
func PrivatePolicy(bucket *MinioBucket) BucketPolicy {
	bucketCondMap := make(ConditionMap)
	bucketCondKeyMap := make(ConditionKeyMap)
	bucketCondKeyMap.Add("aws:userId", set.CreateStringSet(bucket.MinioAccess))
	bucketCondMap.Add("StringNotLike", bucketCondKeyMap)
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []Stmt{
			{
				Sid:        "DenyAllS3Actions",
				Effect:     "Deny",
				Principal:  "*",
				Actions:    allBucketActions,
				Resources:  set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket), fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
				Conditions: bucketCondMap,
			},
		},
	}
}
