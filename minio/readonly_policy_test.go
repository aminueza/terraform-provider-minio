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

	stringPolicy := `{"Version":"2012-10-17","Statement":[{"Sid":"ListAllBucket","Action":["s3:ListAllMyBuckets","s3:ListBucket"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::*"]},{"Sid":"AllObjectActionsMyBuckets","Action":["s3:GetObject","s3:ListBucket"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::test","arn:aws:s3:::test/*"]}]}`

	var expected BucketPolicy
	if err := json.Unmarshal([]byte(stringPolicy), &expected); err != nil {
		t.Error(err)
	}

	policy := ReadOnlyPolicy(minio)
	assert.DeepEqual(t, expected, policy)

}
