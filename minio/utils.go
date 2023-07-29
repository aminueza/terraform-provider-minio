package minio

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"hash/crc32"
	"log"
	"sync"

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

func Filter(slice []string, item string) ([]string, bool) {
	var out []string
	for _, s := range slice {
		if s != item {
			out = append(out, s)
		}
	}
	return out, len(out) != len(slice)
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

// MutexKV is a simple key/value store for arbitrary mutexes. It can be used to
// serialize changes across arbitrary collaborators that share knowledge of the
// keys they must serialize on.
//
// The initial use case is to let aws_security_group_rule resources serialize
// their access to individual security groups based on SG ID.
type MutexKV struct {
	lock  sync.Mutex
	store map[string]*sync.Mutex
}

// Locks the mutex for the given key. Caller is responsible for calling Unlock
// for the same key
func (m *MutexKV) Lock(key string) {
	log.Printf("[DEBUG] Locking %q", key)
	m.get(key).Lock()
	log.Printf("[DEBUG] Locked %q", key)
}

// Unlock the mutex for the given key. Caller must have called Lock for the same key first
func (m *MutexKV) Unlock(key string) {
	log.Printf("[DEBUG] Unlocking %q", key)
	m.get(key).Unlock()
	log.Printf("[DEBUG] Unlocked %q", key)
}

// Returns a mutex for the given key, no guarantee of its lock status
func (m *MutexKV) get(key string) *sync.Mutex {
	m.lock.Lock()
	defer m.lock.Unlock()
	mutex, ok := m.store[key]
	if !ok {
		mutex = &sync.Mutex{}
		m.store[key] = mutex
	}
	return mutex
}

// Returns a properly initialized MutexKV
func NewMutexKV() *MutexKV {
	return &MutexKV{
		store: make(map[string]*sync.Mutex),
	}
}
