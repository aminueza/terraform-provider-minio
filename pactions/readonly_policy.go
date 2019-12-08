package pactions

import (
	"fmt"

	"github.com/minio/minio-go/pkg/set"
)

//ReadOnlyPolicy returns readonly policy
func ReadOnlyPolicy(bucket *MinioBucket) BucketPolicy {
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []Stmt{
			{
				Actions:   readOnlyAllBucketsActions,
				Effect:    "Allow",
				Principal: "*",
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s*", awsResourcePrefix)}...),
				Sid:       "ListAllBucket",
			},
			{
				Actions:   readOnlyBucketActions,
				Effect:    "Deny",
				Principal: "*",
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket), fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
				Sid:       "DenyOtherBuckets",
			},
			{
				Actions:   readListMyObjectActions,
				Effect:    "Allow",
				Principal: "*",
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket), fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
				Sid:       "AllObjectActionsMyBuckets",
			},
		},
	}
}
