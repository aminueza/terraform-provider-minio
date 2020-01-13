package minio

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
)

const (
	minioAccessID = 20
	minioSecretID = 40
)

var (
	alphaNumericTable = []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	blurChar          = "_"
)

//ParseString parses a string to bool
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
		return nil, errors.New("Could not generate Access Key")
	}
	for i := 0; i < minioAccessID; i++ {
		alpha[i] = alphaNumericTable[alpha[i]%byte(len(alphaNumericTable))]
	}
	return alpha, nil
}

// generateSecretAccessKey - generate random base64 numeric value from a random seed.
func generateSecretAccessKey() ([]byte, error) {
	rb := make([]byte, minioSecretID)
	if _, e := rand.Read(rb); e != nil {
		return nil, errors.New("Could not generate Secret Key")
	}
	return []byte(base64.StdEncoding.EncodeToString(rb))[:minioSecretID], nil
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

// mustGenerateSecretAccessKey - generate random base64 numeric value from a random seed.
func mustGenerateSecretAccessKey() []byte {
	secretKey, err := generateSecretAccessKey()
	if err != nil {
		fmt.Print("Unable to generate secretAccessKey.")
	}
	return secretKey
}

//md5Encode creates md5 key
func md5Encode(str string) string {
	md5HashInBytes := md5.Sum([]byte(str))
	md5HashInString := hex.EncodeToString(md5HashInBytes[:])
	return md5HashInString
}

//generateToken Generates token
func generateToken(appID string, accessKey string) string {
	md5Str := md5Encode(appID + blurChar + accessKey)
	token := strings.ToUpper(md5Str)
	return token
}
