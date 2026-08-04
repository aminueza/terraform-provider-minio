package minio

import (
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"
)

func TestReadWritePolicy(t *testing.T) {

	minio := &S3MinioBucket{
		MinioBucket: "test",
	}

	stringPolicy := `{"Version":"2012-10-17","Statement":[{"Sid":"ListBucketActions","Action":["s3:GetBucketLocation","s3:ListBucket","s3:ListBucketMultipartUploads"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::test"]},{"Sid":"AllObjectActions","Action":["s3:AbortMultipartUpload","s3:DeleteObject","s3:GetObject","s3:ListMultipartUploadParts","s3:PutObject"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::test/*"]}]}`

	var expected BucketPolicy
	if err := json.Unmarshal([]byte(stringPolicy), &expected); err != nil {
		t.Error(err)
	}

	policy := ReadWritePolicy(minio)
	assert.DeepEqual(t, expected, policy)

}
