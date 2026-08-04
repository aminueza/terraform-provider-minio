package minio

import (
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"
)

func TestReadPolicy(t *testing.T) {

	minio := &S3MinioBucket{
		MinioBucket: "test",
	}

	stringPolicy := `{"Version":"2012-10-17","Statement":[{"Sid":"ListBucketActions","Action":["s3:GetBucketLocation","s3:ListBucket"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::test"]},{"Sid":"ReadObjectActions","Action":["s3:GetObject"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::test/*"]}]}`

	var expected BucketPolicy
	if err := json.Unmarshal([]byte(stringPolicy), &expected); err != nil {
		t.Error(err)
	}

	policy := ReadOnlyPolicy(minio)
	assert.DeepEqual(t, expected, policy)

}
