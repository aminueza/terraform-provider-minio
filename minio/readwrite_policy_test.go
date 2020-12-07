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

	stringPolicy := `{"Version":"2012-10-17","Statement":[{"Sid":"ListObjectsInBucket","Action":["s3:ListBucket"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::test"]},{"Sid":"UploadObjectActions","Action":["s3:PutObject"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::test/*"]}]}`

	policy, err := json.Marshal(ReadWritePolicy(minio))

	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, string(policy), string(stringPolicy))

}
