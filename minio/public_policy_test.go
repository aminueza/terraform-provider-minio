package minio

import (
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"
)

func TestPublicPolicy(t *testing.T) {

	minio := &S3MinioBucket{
		MinioBucket: "test",
	}

	stringPolicy := `{"Version":"2012-10-17","Statement":[{"Sid":"AllowAllS3Actions","Action":["s3:AbortMultipartUpload","s3:CreateBucket","s3:DeleteBucket","s3:DeleteBucketPolicy","s3:DeleteObject","s3:GetBucketLocation","s3:GetBucketNotification","s3:GetBucketPolicy","s3:GetObject","s3:HeadBucket","s3:ListAllMyBuckets","s3:ListBucket","s3:ListBucketMultipartUploads","s3:ListMultipartUploadParts","s3:ListenBucketNotification","s3:PutBucketNotification","s3:PutBucketPolicy","s3:PutObject"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::test","arn:aws:s3:::test/*"]}]}`

	policy, err := json.Marshal(PublicPolicy(minio))

	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, string(policy), string(stringPolicy))

}
