package minio

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestSafeUint64ToInt64(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected int64
		ok       bool
	}{
		{
			name:     "zero value",
			input:    0,
			expected: 0,
			ok:       true,
		},
		{
			name:     "small value",
			input:    1024,
			expected: 1024,
			ok:       true,
		},
		{
			name:     "typical disk space (1TB)",
			input:    1099511627776,
			expected: 1099511627776,
			ok:       true,
		},
		{
			name:     "large disk space (1PB)",
			input:    1125899906842624,
			expected: 1125899906842624,
			ok:       true,
		},
		{
			name:     "max int64 value",
			input:    math.MaxInt64,
			expected: math.MaxInt64,
			ok:       true,
		},
		{
			name:     "max int64 + 1 (overflow)",
			input:    uint64(math.MaxInt64) + 1,
			expected: math.MaxInt64,
			ok:       false,
		},
		{
			name:     "max uint64 value (overflow)",
			input:    math.MaxUint64,
			expected: math.MaxInt64,
			ok:       false,
		},
		{
			name:     "10 exabytes (overflow)",
			input:    10000000000000000000,
			expected: math.MaxInt64,
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := SafeUint64ToInt64(tt.input)

			if ok != tt.ok {
				t.Errorf("SafeUint64ToInt64(%d) ok = %v, want %v", tt.input, ok, tt.ok)
			}

			if result != tt.expected {
				t.Errorf("SafeUint64ToInt64(%d) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSafeUint64ToInt64_NoNegative(t *testing.T) {
	testValues := []uint64{
		0,
		1,
		math.MaxInt64,
		math.MaxInt64 + 1,
		math.MaxUint64,
	}

	for _, val := range testValues {
		result, _ := SafeUint64ToInt64(val)
		if result < 0 {
			t.Errorf("SafeUint64ToInt64(%d) returned negative value: %d", val, result)
		}
	}
}

func BenchmarkSafeUint64ToInt64(b *testing.B) {
	testValues := []uint64{
		1024,
		1099511627776, // 1TB
		math.MaxInt64,
		math.MaxUint64,
	}

	for _, val := range testValues {
		b.Run("", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				SafeUint64ToInt64(val)
			}
		})
	}
}

// objectVersionID, parseObjectVersionID and the object-lock read
// classifier are shared helpers in utils.go; their tests live here with
// the rest of the file's tests. See issue #1378.

const testVersionID = "3f1b1a5e-0f7a-4b3f-9c0e-4a1e2cb6d000"

func TestParseObjectVersionID(t *testing.T) {
	cases := []struct {
		id                 string
		bucket, key, verID string
	}{
		{"b/reports#2026.csv#", "b", "reports#2026.csv", ""},
		{"b/reports#2026.csv#" + testVersionID, "b", "reports#2026.csv", testVersionID},
		{"b/k#null", "b", "k", "null"},
		{"b/dir/a#b#", "b", "dir/a#b", ""},
		{"b/k", "b", "k", ""},
		{"b/k#" + testVersionID, "b", "k", testVersionID},
	}
	for _, c := range cases {
		bucket, key, verID, err := parseObjectVersionID(c.id)
		if err != nil {
			t.Errorf("parseObjectVersionID(%q) returned error: %v", c.id, err)
			continue
		}
		if bucket != c.bucket || key != c.key || verID != c.verID {
			t.Errorf("parseObjectVersionID(%q) = (%q, %q, %q), want (%q, %q, %q)",
				c.id, bucket, key, verID, c.bucket, c.key, c.verID)
		}
	}
}

// A stored ID has to import back, including for a key that contains the "#"
// separator itself.
func TestObjectVersionIDRoundTrip(t *testing.T) {
	cases := []struct{ bucket, key, verID string }{
		{"b", "k", ""},
		{"b", "k", "null"},
		{"b", "k", testVersionID},
		{"b", "reports#2026.csv", ""},
		{"b", "reports#2026.csv", testVersionID},
		{"b", "dir/a#b", ""},
		{"b", "key#with#many#hashes", ""},
	}
	for _, c := range cases {
		id := objectVersionID(c.bucket, c.key, c.verID)
		bucket, key, verID, err := parseObjectVersionID(id)
		if err != nil {
			t.Errorf("objectVersionID(%q, %q, %q) = %q, which does not parse: %v",
				c.bucket, c.key, c.verID, id, err)
			continue
		}
		if bucket != c.bucket || key != c.key || verID != c.verID {
			t.Errorf("objectVersionID(%q, %q, %q) = %q, parsed back as (%q, %q, %q)",
				c.bucket, c.key, c.verID, id, bucket, key, verID)
		}
	}
}

// A key without "#" keeps the ID it has always had, so existing states are not
// rewritten.
func TestObjectVersionIDKeepsUnversionedFormat(t *testing.T) {
	if got := objectVersionID("b", "plain.txt", ""); got != "b/plain.txt" {
		t.Errorf("objectVersionID = %q, want %q", got, "b/plain.txt")
	}
}

// An ID that yields no bucket or no key has to fail before any API call is made
// with an empty object name.
func TestParseObjectVersionIDRejectsIncompleteID(t *testing.T) {
	for _, id := range []string{"", "bucket", "/key", "bucket/", "bucket/#" + testVersionID} {
		if _, _, _, err := parseObjectVersionID(id); err == nil {
			t.Errorf("parseObjectVersionID(%q) accepted an ID with no bucket or no key", id)
		}
	}
}

func TestObjectLockReadGone(t *testing.T) {
	cases := []struct {
		name string
		resp minio.ErrorResponse
		want bool
	}{
		{"deleted object", minio.ErrorResponse{Code: "NoSuchKey"}, true},
		{"deleted version", minio.ErrorResponse{Code: "NoSuchVersion"}, true},
		{"deleted bucket", minio.ErrorResponse{Code: "NoSuchBucket"}, true},
		{"bucket lost its lock configuration", minio.ErrorResponse{
			Code:    "InvalidRequest",
			Message: "Bucket is missing ObjectLockConfiguration",
		}, true},
		{"unrelated InvalidRequest", minio.ErrorResponse{Code: "InvalidRequest", Message: "Invalid request"}, false},
		{"denied", minio.ErrorResponse{Code: "AccessDenied"}, false},
		{"object has no lock metadata", minio.ErrorResponse{Code: "NoSuchObjectLockConfiguration"}, false},
	}
	for _, c := range cases {
		if got := objectLockReadGone(c.resp); got != c.want {
			t.Errorf("%s: objectLockReadGone(%q) = %v, want %v", c.name, c.resp.Code, got, c.want)
		}
	}
}

func TestObjectVersionImportHint(t *testing.T) {
	invalidVersion := minio.ErrorResponse{Code: "InvalidArgument", Message: "Invalid version id specified"}

	if err := objectVersionImportHint(invalidVersion, false); err != invalidVersion {
		t.Error("the hint was added to an error that did not come from an imported ID")
	}
	if err := objectVersionImportHint(minio.ErrorResponse{Code: "AccessDenied"}, true); err == nil ||
		errors.Is(err, invalidVersion) {
		t.Error("the hint was added to an unrelated error")
	}

	hinted := objectVersionImportHint(invalidVersion, true)
	if !errors.Is(hinted, invalidVersion) {
		t.Fatal("the hint replaced the server error instead of wrapping it")
	}
	if !strings.Contains(hinted.Error(), `end the ID with "#"`) || !strings.Contains(hinted.Error(), "Invalid version id specified") {
		t.Errorf("hinted error does not name the fix: %v", hinted)
	}
}
