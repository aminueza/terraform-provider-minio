package minio

import (
	"fmt"

	"github.com/minio/minio-go/v7/pkg/policy"

	"github.com/minio/minio-go/v7/pkg/set"
)

// WriteOnlyPolicy returns policy where objects can be listed and written
func WriteOnlyPolicy(bucket *S3MinioBucket) BucketPolicy {
	bucketResource := fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket)
	objectResource := fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)

	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "ListBucketAction",
				Actions:   readOnlyBucketActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet(bucketResource),
			},
			{
				Sid:       "AllObjectActionsMyBuckets",
				Actions:   writeOnlyObjectActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet(objectResource),
			},
		},
	}
}
