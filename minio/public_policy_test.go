package minio

import (
	"encoding/json"
	"log"
	"testing"

	"gotest.tools/assert"
)

func TestPublicPolicy(t *testing.T) {

	minio := &MinioBucket{
		MinioBucket: "test",
	}

	stringPolicy := `{"Version":"2012-10-17","Statement":[{"Action":["s3:*"],"Effect":"Allow","Principal":{"AWS":["*"]},"Resource":["arn:aws:s3:::test","arn:aws:s3:::test/*"],"Sid":"AllowAllS3Actions"}]}`

	policy, err := json.Marshal(PublicPolicy(minio))

	log.Print(string(policy))
	log.Print(string(stringPolicy))

	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, string(policy), string(stringPolicy))

}
