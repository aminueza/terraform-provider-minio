package minio

import (
	"encoding/json"
	"log"
	"testing"

	"gotest.tools/assert"
)

func TestReadPolicy(t *testing.T) {

	minio := &S3MinioBucket{
		S3MinioBucket: "test",
	}

	stringPolicy := `{"Version":"2012-10-17","Statement":[{"Sid":"ListAllBucket","Action":["s3:ListAllMyBuckets","s3:ListBucket"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::*"]},{"Sid":"AllObjectActionsMyBuckets","Action":["s3:GetObject","s3:ListBucket"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::test","arn:aws:s3:::test/*"]}]}`

	policy, err := json.Marshal(ReadOnlyPolicy(minio))

	log.Print(string(policy))
	log.Print(string(stringPolicy))

	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, string(policy), string(stringPolicy))

}
