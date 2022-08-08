package minio

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"hash/crc32"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	minioSecretIDLength = 40
)

func tagsSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeMap,
		Optional: true,
	}
}

// generateSecretAccessKey - generate random base64 numeric value from a random seed.
func generateSecretAccessKey() (string, error) {
	rb := make([]byte, minioSecretIDLength)
	if _, e := rand.Read(rb); e != nil {
		return "", errors.New("could not generate Secret Key")
	}

	return string(Encode(rb)), nil
}

// Encode queues message
func Encode(value []byte) []byte {
	length := len(value)
	encoded := make([]byte, base64.URLEncoding.EncodedLen(length))
	base64.URLEncoding.Encode(encoded, value)
	return encoded
}

// getStringList get array of strings
func getStringList(listString []interface{}) []*string {
	arrayString := make([]*string, 0, len(listString))
	for _, v := range listString {
		value, ret := v.(string)
		if ret && value != "" {
			arrayString = append(arrayString, aws.String(v.(string)))
		}
	}
	return arrayString
}

// Contains check that an array has the given element
func Contains(slice []string, item string) bool {
	set := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		set[s] = struct{}{}
	}

	_, ok := set[item]
	return ok
}

// HashcodeString hashes a string to a unique hashcode.
//
// crc32 returns a `uint32`, but for our use we need
// a non-negative integer. Here we cast to an integer
// and invert it if the result is negative.
func HashcodeString(s string) int {
	v := int(crc32.ChecksumIEEE([]byte(s)))
	if v >= 0 {
		return v
	}
	if -v >= 0 {
		return -v
	}
	// v == MinInt
	return 0
}
