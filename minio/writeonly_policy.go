package minio

import (
	"fmt"

	"github.com/minio/minio-go/v6/pkg/set"
)

//WriteOnlyPolicy returns writeonly policy
func WriteOnlyPolicy(bucket *MinioBucket) BucketPolicy {
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
				Sid:       "ListBucketAction",
			},
			{
				Actions: writeOnlyObjectActions,
				Effect:  "Allow",
				Principal: Princ{
					AWS: set.CreateStringSet("*"),
				},
				Resources: set.CreateStringSet([]string{fmt.Sprintf("%s%s/*", awsResourcePrefix, bucket.MinioBucket)}...),
				Sid:       "AllObjectActionsMyBuckets",
			},
		},
	}
}
