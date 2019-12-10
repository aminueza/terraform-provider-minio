package minio

import (
	"encoding/json"
	"log"
	"testing"

	"gotest.tools/assert"
)

func TestUploadPolicy(t *testing.T) {

	minio := &MinioBucket{
		MinioBucket: "test",
	}

	policy, err := json.Marshal(PrivatePolicy(minio))

	log.Print(string(policy))

	if err != nil {
		t.Error(err)
	}

	t.Log(policy)

	assert.Equal(t, err, nil)

}
