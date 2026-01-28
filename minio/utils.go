package minio

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"hash/crc32"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/dustin/go-humanize"
	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
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

func shortDur(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

// NormalizeAndCompareJSONPolicies compares two policy JSON strings and returns the appropriate one to use.
// It chooses the old policy if the policies are equivalent, otherwise returns the new one.
// It also normalizes the chosen policy to ensure consistent formatting.
// This function is used to prevent unnecessary updates when policies are semantically equivalent.
func NormalizeAndCompareJSONPolicies(oldPolicy, newPolicy string) (string, error) {
	// Handle empty policies
	if strings.TrimSpace(newPolicy) == "" {
		return "", nil
	}

	if strings.TrimSpace(newPolicy) == "{}" {
		return "{}", nil
	}

	if strings.TrimSpace(oldPolicy) == "" || strings.TrimSpace(oldPolicy) == "{}" {
		// If old policy is empty, use the new one but normalize it first
		normalizedPolicy, err := structure.NormalizeJsonString(newPolicy)
		if err != nil {
			return "", err
		}
		return normalizedPolicy, nil
	}

	// Check if policies are equivalent
	equivalent, err := awspolicy.PoliciesAreEquivalent(oldPolicy, newPolicy)
	if err != nil {
		return "", err
	}

	if equivalent {
		// If policies are equivalent, prefer the existing one for state consistency
		return oldPolicy, nil
	}

	// Policies are different, use the new one but normalize it
	normalizedPolicy, err := structure.NormalizeJsonString(newPolicy)
	if err != nil {
		return "", err
	}
	return normalizedPolicy, nil
}

// SafeUint64ToInt64 converts a uint64 to int64, returning MaxInt64 if the value exceeds it.
func SafeUint64ToInt64(val uint64) (int64, bool) {
	if val > uint64(math.MaxInt64) {
		return math.MaxInt64, false
	}
	return int64(val), true
}

// SafeInt64ToInt64 safely handles int64 values that represent unsigned quantities (like uptime).
// Returns 0 if negative, otherwise returns the value unchanged.
func SafeInt64ToInt64(val int64) int64 {
	if val < 0 {
		return 0
	}
	return val
}

// ParseBandwidthLimit extracts and parses the bandwidth limit from a target map.
// It handles both the legacy attribute "bandwidth_limt" and the new attribute "bandwidth_limit".
// Returns the parsed bandwidth value, a boolean indicating success, and any diagnostic errors.
func ParseBandwidthLimit(target map[string]any) (uint64, bool, diag.Diagnostics) {
	var ok bool
	var bandwidthStr string
	var legacyLimitValue string
	var limitValue string
	var errs diag.Diagnostics

	// Check for legacy attribute first (with typo)
	if legacyLimitValue, ok = target["bandwidth_limt"].(string); ok {
		bandwidthStr = legacyLimitValue
	} else if limitValue, ok = target["bandwidth_limit"].(string); ok {
		bandwidthStr = limitValue
	}

	if bandwidthStr == "" {
		return 0, false, nil
	}

	bandwidth, err := humanize.ParseBytes(bandwidthStr)
	if err != nil {
		log.Printf("[WARN] invalid bandwidth value %q: %v", bandwidthStr, err)
		errs = append(errs, diag.Errorf("bandwidth_limit is invalid. Make sure to use k, m, g as prefix only")...)
		return 0, false, errs
	}

	// Check if bandwidth exceeds maximum int64 value
	if bandwidth > uint64(math.MaxInt64) {
		log.Printf("[WARN] Configured bandwidth limit (%s) exceeds maximum supported value (%s)", humanize.Bytes(bandwidth), humanize.Bytes(uint64(math.MaxInt64)))
	}

	return bandwidth, true, nil
}
