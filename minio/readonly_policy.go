package minio

import (
	"fmt"

	"github.com/minio/minio-go/v7/pkg/policy"

	"github.com/minio/minio-go/v7/pkg/set"
)

// ReadOnlyPolicy returns policy where objects can be listed and read.
// The shape matches what `mc anonymous set download` writes, so `mc anonymous get` and every
// other minio-go consumer report the bucket as `download` instead of `custom`.
func ReadOnlyPolicy(bucket *S3MinioBucket) BucketPolicy {
	bucketResource := fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket)
	objectResource := fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)

	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "ListBucketActions",
				Actions:   downloadBucketActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet(bucketResource),
			},
			{
				Sid:       "ReadObjectActions",
				Actions:   readOnlyObjectActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet(objectResource),
			},
		},
	}
}
