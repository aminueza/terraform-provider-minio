package minio

import (
	"fmt"
	"sort"

	"github.com/minio/minio-go/v7/pkg/policy"

	"github.com/minio/minio-go/v7/pkg/set"
)

// ReadOnlyPolicy returns policy where objects can be listed and read
func ReadOnlyPolicy(bucket *S3MinioBucket) BucketPolicy {
	resources := []string{
		fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket),
		fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket),
	}
	sort.Strings(resources)

	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "ListAllBucket",
				Actions:   readOnlyAllBucketsActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s*", awsResourcePrefix)}...),
			},
			{
				Sid:       "AllObjectActionsMyBuckets",
				Actions:   readListMyObjectActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet(resources...),
			},
		},
	}
}
