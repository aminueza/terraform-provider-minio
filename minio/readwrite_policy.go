package minio

import (
	"fmt"

	"github.com/minio/minio-go/v6/pkg/set"
)

//ReadWritePolicy returns readonly policy
func ReadWritePolicy(bucket *MinioBucket) BucketPolicy {
	return BucketPolicy{
		Version: "2012-10-17",
		Statements: []Stmt{
			{
				Actions: readOnlyBucketActions,
				Effect:  "Allow",
				Principal: Princ{
					AWS: set.CreateStringSet("*"),
				},
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s", awsResourcePrefix, bucket.MinioBucket)}...),
				Sid:       "ListObjectsInBucket",
			},
			{
				Actions: uploadObjectActions,
				Effect:  "Allow",
				Principal: Princ{
					AWS: set.CreateStringSet("*"),
				},
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
				Sid:       "UploadObjectActions",
			},
		},
	}
}
