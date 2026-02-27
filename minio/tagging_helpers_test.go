package minio

import (
	"encoding/xml"
	"errors"
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestShouldSkipBucketTagging(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *S3MinioBucket
		expected bool
	}{
		{
			name:     "nil config",
			cfg:      nil,
			expected: false,
		},
		{
			name: "skip enabled",
			cfg: &S3MinioBucket{
				SkipBucketTagging: true,
			},
			expected: true,
		},
		{
			name: "skip disabled",
			cfg: &S3MinioBucket{
				SkipBucketTagging: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSkipBucketTagging(tt.cfg); got != tt.expected {
				t.Errorf("shouldSkipBucketTagging() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsS3TaggingNotImplementedExtended(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name: "NotImplemented error code",
			err: &minio.ErrorResponse{
				Code: "NotImplemented",
			},
			expected: true,
		},
		{
			name: "XNotImplemented error code",
			err: &minio.ErrorResponse{
				Code: "XNotImplemented",
			},
			expected: true,
		},
		{
			name: "NotImplementedError error code",
			err: &minio.ErrorResponse{
				Code: "NotImplementedError",
			},
			expected: true,
		},
		{
			name: "NotImplementedException error code",
			err: &minio.ErrorResponse{
				Code: "NotImplementedException",
			},
			expected: true,
		},
		{
			name: "other error code",
			err: &minio.ErrorResponse{
				Code: "AccessDenied",
			},
			expected: false,
		},
		{
			name:     "unexpected XML response",
			err:      xml.UnmarshalError("expected element type <Tagging> but have <ListBucketResult>"),
			expected: true,
		},
		{
			name:     "generic not implemented message",
			err:      errors.New("This feature is not implemented"),
			expected: true,
		},
		{
			name:     "generic not supported message",
			err:      errors.New("This feature is not supported"),
			expected: true,
		},
		{
			name:     "other error message",
			err:      errors.New("Access denied"),
			expected: false,
		},
		{
			name: "message with not implemented",
			err: &minio.ErrorResponse{
				Code:    "SomeError",
				Message: "Tagging operations are not implemented on this endpoint",
			},
			expected: true,
		},
		{
			name: "message with not supported",
			err: &minio.ErrorResponse{
				Code:    "SomeError",
				Message: "Tagging operations are not supported on this endpoint",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsS3TaggingNotImplemented(tt.err); got != tt.expected {
				t.Errorf("IsS3TaggingNotImplemented() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsTaggingNotImplementedCode(t *testing.T) {
	tests := []struct {
		code     string
		expected bool
	}{
		{"NotImplemented", true},
		{"XNotImplemented", true},
		{"NotImplementedError", true},
		{"NotImplementedException", true},
		{"AccessDenied", false},
		{"NoSuchBucket", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := isTaggingNotImplementedCode(tt.code); got != tt.expected {
				t.Errorf("isTaggingNotImplementedCode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsTaggingNotImplementedMessage(t *testing.T) {
	tests := []struct {
		message  string
		expected bool
	}{
		{"This feature is not implemented", true},
		{"This feature is not supported", true},
		{"NOT IMPLEMENTED", true},
		{"NOT SUPPORTED", true},
		{"This feature is unavailable", false},
		{"Access denied", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			if got := isTaggingNotImplementedMessage(tt.message); got != tt.expected {
				t.Errorf("isTaggingNotImplementedMessage() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsBucketTaggingUnexpectedResponse(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "exact XML unmarshal error with ListBucketResult",
			err:      xml.UnmarshalError("expected element type <Tagging> but have <ListBucketResult>"),
			expected: true,
		},
		{
			name:     "XML unmarshal error with non-ListBucketResult element",
			err:      xml.UnmarshalError("expected element type <Tagging> but have <SomethingElse>"),
			expected: false,
		},
		{
			name:     "generic error text with ListBucketResult",
			err:      errors.New("expected element type <Tagging> but have <ListBucketResult>"),
			expected: true,
		},
		{
			name:     "unrelated error message",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBucketTaggingUnexpectedResponse(tt.err); got != tt.expected {
				t.Errorf("isBucketTaggingUnexpectedResponse() = %v, want %v", got, tt.expected)
			}
		})
	}
}
