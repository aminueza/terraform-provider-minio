package minio

import (
	"testing"
)

// FuzzNormalizeAndCompareJSONPolicies verifies that NormalizeAndCompareJSONPolicies
// never panics on arbitrary JSON-like input, including malformed documents.
func FuzzNormalizeAndCompareJSONPolicies(f *testing.F) {
	// Seed corpus: representative policy documents
	f.Add(`{"Version":"2012-10-17","Statement":[]}`, `{"Version":"2012-10-17","Statement":[]}`)
	f.Add(``, `{"Version":"2012-10-17","Statement":[]}`)
	f.Add(`{}`, `{}`)
	f.Add(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":"arn:aws:s3:::bucket/*"}]}`,
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::bucket/*"}]}`,
	)
	f.Add(`not-json`, `also-not-json`)
	f.Add(`{"Version":"2012-10-17"}`, ``)

	f.Fuzz(func(t *testing.T, oldPolicy, newPolicy string) {
		// Must not panic regardless of input
		_, _ = NormalizeAndCompareJSONPolicies(oldPolicy, newPolicy)
	})
}

// FuzzParseBandwidthLimit verifies that ParseBandwidthLimit never panics on
// arbitrary bandwidth string values, including malformed ones.
func FuzzParseBandwidthLimit(f *testing.F) {
	// Seed corpus: valid and invalid bandwidth strings
	f.Add("100MB")
	f.Add("1GB")
	f.Add("500k")
	f.Add("0")
	f.Add("")
	f.Add("invalid")
	f.Add("9999999999999999999999TB")
	f.Add("-1MB")

	f.Fuzz(func(t *testing.T, bandwidthStr string) {
		target := map[string]any{
			"bandwidth_limit": bandwidthStr,
		}
		// Must not panic regardless of input
		_, _, _ = ParseBandwidthLimit(target)
	})
}

// FuzzHashcodeString verifies that HashcodeString never panics and always
// returns a non-negative value for any string input.
func FuzzHashcodeString(f *testing.F) {
	f.Add("")
	f.Add("bucket-name")
	f.Add("arn:aws:iam::123456789012:root")
	f.Add("\x00\xff\xfe")

	f.Fuzz(func(t *testing.T, s string) {
		result := HashcodeString(s)
		if result < 0 {
			t.Errorf("HashcodeString(%q) returned negative value: %d", s, result)
		}
	})
}
