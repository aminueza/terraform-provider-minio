package minio

import (
	"fmt"
	"github.com/minio/minio-go/v7/pkg/policy"

	"github.com/minio/minio-go/v7/pkg/set"
)

// WriteOnlyPolicy returns policy where objects can be listed and written
func WriteOnlyPolicy(bucket *S3MinioBucket) BucketPolicy {
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "ListBucketAction",
				Actions:   readOnlyBucketActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket)}...),
			},
			{
				Sid:       "AllObjectActionsMyBuckets",
				Actions:   writeOnlyObjectActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
			},
		},
	}
}
