package minio

import (
	"fmt"

	"github.com/minio/minio-go/v7/pkg/policy"

	"github.com/minio/minio-go/v7/pkg/set"
)

// WriteOnlyPolicy returns policy where objects can be written but not read.
// The shape matches what `mc anonymous set upload` writes, so `mc anonymous get` and every
// other minio-go consumer report the bucket as `upload` instead of `custom`.
func WriteOnlyPolicy(bucket *S3MinioBucket) BucketPolicy {
	bucketResource := fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket)
	objectResource := fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)

	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []policy.Statement{
			{
				Sid:       "ListBucketActions",
				Actions:   uploadBucketActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet(bucketResource),
			},
			{
				Sid:       "WriteObjectActions",
				Actions:   writeOnlyObjectActions,
				Effect:    "Allow",
				Principal: policy.User{AWS: set.CreateStringSet("*")},
				Resources: set.CreateStringSet(objectResource),
			},
		},
	}
}
