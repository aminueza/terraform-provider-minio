package minio

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/crc32"
	"log"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	minioAccessID = 20
	minioSecretID = 40
)

var (
	alphaNumericTable = []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	blurChar          = "_"
	length            int
)

// ParseString parses a string to bool
func ParseString(s string) bool {
	debugbool, err := strconv.ParseBool(s)
	if err != nil {
		log.Print(err)
	}
	return debugbool
}

func tagsSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeMap,
		Optional: true,
	}
}

func tagsSchemaComputed() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeMap,
		Optional: true,
		Computed: true,
	}
}

func tagsSchemaForceNew() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeMap,
		Optional: true,
		ForceNew: true,
	}
}

// generateAccessKeyID - generate random alpha numeric value using only uppercase characters
// takes input as size in integer
func generateAccessKeyID() ([]byte, error) {
	alpha := make([]byte, minioAccessID)
	if _, e := rand.Read(alpha); e != nil {
		return nil, errors.New("could not generate Access Key")
	}
	for i := 0; i < minioAccessID; i++ {
		alpha[i] = alphaNumericTable[alpha[i]%byte(len(alphaNumericTable))]
	}
	return alpha, nil
}

// generateSecretAccessKey - generate random base64 numeric value from a random seed.
func generateSecretAccessKey() (string, error) {
	rb := make([]byte, minioSecretID)
	if _, e := rand.Read(rb); e != nil {
		return "", errors.New("could not generate Secret Key")
	}

	return string(Encode(rb)), nil
}

// mustGenerateAccessKeyID - must generate random alpha numeric value using only uppercase characters
// takes input as size in integer
func mustGenerateAccessKeyID() []byte {
	alpha, err := generateAccessKeyID()

	if err != nil {
		fmt.Print("Unable to generate accessKeyID.")
	}
	return alpha
}

// Encode queues message
func Encode(value []byte) []byte {
	length = len(value)
	encoded := make([]byte, base64.URLEncoding.EncodedLen(length))
	base64.URLEncoding.Encode(encoded, value)
	return encoded
}

// Decode queues message
func Decode(value []byte) ([]byte, error) {
	length = len(value)
	decoded := make([]byte, base64.URLEncoding.DecodedLen(length))

	n, err := base64.URLEncoding.Decode(decoded, value)
	if err != nil {
		return nil, err
	}
	return decoded[:n], nil
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

// Contains check that a Array has the given element
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
// crc32 returns a uint32, but for our use we need
// and non negative integer. Here we cast to an integer
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
