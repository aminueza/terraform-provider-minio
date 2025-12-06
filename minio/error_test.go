package minio

import (
	"errors"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestExtractErrorDetail_MinioError(t *testing.T) {
	err := minio.ErrorResponse{
		Code:       "NoSuchBucket",
		Message:    "The specified bucket does not exist",
		RequestID:  "ABC123",
		BucketName: "test-bucket",
	}

	detail := extractErrorDetail(err)

	if detail == "" {
		t.Error("expected non-empty detail")
	}

	if !strings.Contains(detail, "NoSuchBucket") {
		t.Error("expected detail to contain error code")
	}

	if !strings.Contains(detail, "test-bucket") {
		t.Error("expected detail to contain bucket name")
	}
}

func TestExtractErrorDetail_GenericError(t *testing.T) {
	err := errors.New("generic error")
	detail := extractErrorDetail(err)

	if detail != "" {
		t.Errorf("expected empty detail for generic error, got: %s", detail)
	}
}

func TestNewResourceError_WithMinioError(t *testing.T) {
	err := minio.ErrorResponse{
		Code:    "AccessDenied",
		Message: "Access Denied",
	}

	diags := NewResourceError("failed to create bucket", "test-bucket", err)

	if len(diags) < 2 {
		t.Errorf("expected at least 2 diagnostics (error + detail), got %d", len(diags))
	}
}

func TestEnhanceConnectionError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected string
	}{
		{
			name:     "connection refused",
			errMsg:   "dial tcp 127.0.0.1:9000: connection refused",
			expected: "Connection refused. Verify that the MinIO server is running and the endpoint is correct.",
		},
		{
			name:     "no such host",
			errMsg:   "dial tcp: lookup invalid.host: no such host",
			expected: "Host not found. Verify the MinIO server hostname is correct.",
		},
		{
			name:     "certificate error",
			errMsg:   "x509: certificate signed by unknown authority",
			expected: "SSL/TLS certificate error. If using self-signed certificates, set minio_insecure = true.",
		},
		{
			name:     "invalid character (HTML response)",
			errMsg:   "invalid character '<' looking for beginning of value",
			expected: "Invalid response from server. This often indicates the endpoint URL is incorrect (e.g., pointing to a web page instead of the MinIO API).",
		},
		{
			name:     "generic error",
			errMsg:   "some other error",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			result := enhanceConnectionError(err)
			if result != tt.expected {
				t.Errorf("enhanceConnectionError(%q) = %q, want %q", tt.errMsg, result, tt.expected)
			}
		})
	}
}

func TestEnhanceConnectionError_NilError(t *testing.T) {
	result := enhanceConnectionError(nil)
	if result != "" {
		t.Errorf("enhanceConnectionError(nil) = %q, want empty string", result)
	}
}

func TestEnhanceAuthError(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name:     "AccessDenied",
			code:     "AccessDenied",
			expected: "Access denied. Verify your access key and secret key are correct.",
		},
		{
			name:     "InvalidAccessKeyId",
			code:     "InvalidAccessKeyId",
			expected: "Invalid access key. The access key does not exist.",
		},
		{
			name:     "SignatureDoesNotMatch",
			code:     "SignatureDoesNotMatch",
			expected: "Signature mismatch. The secret key may be incorrect.",
		},
		{
			name:     "other error code",
			code:     "NoSuchBucket",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := minio.ErrorResponse{Code: tt.code}
			result := enhanceAuthError(err)
			if result != tt.expected {
				t.Errorf("enhanceAuthError(%q) = %q, want %q", tt.code, result, tt.expected)
			}
		})
	}
}

func TestExtractErrorDetail_WithAuthHint(t *testing.T) {
	err := minio.ErrorResponse{
		Code:    "AccessDenied",
		Message: "Access Denied",
	}

	detail := extractErrorDetail(err)

	if !strings.Contains(detail, "Code: AccessDenied") {
		t.Error("expected detail to contain error code")
	}

	if !strings.Contains(detail, "Hint:") {
		t.Error("expected detail to contain hint")
	}

	if !strings.Contains(detail, "Verify your access key") {
		t.Error("expected detail to contain auth hint message")
	}
}

func TestExtractErrorDetail_WithConnectionHint(t *testing.T) {
	err := errors.New("dial tcp 127.0.0.1:9000: connection refused")

	detail := extractErrorDetail(err)

	if !strings.Contains(detail, "Hint:") {
		t.Error("expected detail to contain hint")
	}

	if !strings.Contains(detail, "Connection refused") {
		t.Error("expected detail to contain connection hint message")
	}
}
