package minio

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7"
)

const (
	ErrorSeverityFatal = "[FATAL]"
)

type ResourceError struct {
	Message  string
	Resource string
	Err      error
}

func (e *ResourceError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s %s (%s): %v", ErrorSeverityFatal, e.Message, e.Resource, e.Err)
	}
	return fmt.Sprintf("%s %s (%s)", ErrorSeverityFatal, e.Message, e.Resource)
}

func NewResourceError(msg string, resource string, err interface{}) diag.Diagnostics {
	resErr := &ResourceError{
		Message:  msg,
		Resource: resource,
	}

	switch e := err.(type) {
	case diag.Diagnostics:
		return append(e, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  resErr.Error(),
		})
	case error:
		resErr.Err = e
		diags := diag.Errorf("%s", resErr.Error())

		if detail := extractErrorDetail(e); detail != "" {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Server Response Details",
				Detail:   detail,
			})
		}

		return diags
	default:
		return diag.Errorf("%s %s (%s): %v", ErrorSeverityFatal, msg, resource, err)
	}
}

func NewResourceErrorStr(msg string, resource string, err interface{}) string {
	diags := NewResourceError(msg, resource, err)
	if len(diags) == 0 {
		return ""
	}

	strs := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.Summary != "" {
			strs = append(strs, d.Summary)
		}
	}

	return strings.Join(strs, ", ")
}

func extractErrorDetail(err error) string {
	if err == nil {
		return ""
	}

	var details []string

	var minioErr minio.ErrorResponse
	if errors.As(err, &minioErr) {
		details = append(details, fmt.Sprintf("Code: %s", minioErr.Code))
		details = append(details, fmt.Sprintf("Message: %s", minioErr.Message))
		if minioErr.RequestID != "" {
			details = append(details, fmt.Sprintf("RequestID: %s", minioErr.RequestID))
		}
		if minioErr.BucketName != "" {
			details = append(details, fmt.Sprintf("Bucket: %s", minioErr.BucketName))
		}
		if minioErr.Key != "" {
			details = append(details, fmt.Sprintf("Key: %s", minioErr.Key))
		}
		if minioErr.Server != "" {
			details = append(details, fmt.Sprintf("Server: %s", minioErr.Server))
		}
		if hint := enhanceAuthError(minioErr); hint != "" {
			details = append(details, "")
			details = append(details, fmt.Sprintf("Hint: %s", hint))
		}
	}

	var madminErr madmin.ErrorResponse
	if errors.As(err, &madminErr) {
		details = append(details, fmt.Sprintf("Code: %s", madminErr.Code))
		details = append(details, fmt.Sprintf("Message: %s", madminErr.Message))
		if madminErr.RequestID != "" {
			details = append(details, fmt.Sprintf("RequestID: %s", madminErr.RequestID))
		}
	}

	if hint := enhanceConnectionError(err); hint != "" {
		if len(details) > 0 {
			details = append(details, "")
		}
		details = append(details, fmt.Sprintf("Hint: %s", hint))
	}

	if len(details) > 0 {
		return strings.Join(details, "\n")
	}

	return ""
}

func enhanceConnectionError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	if strings.Contains(errStr, "connection refused") {
		return "Connection refused. Verify that the MinIO server is running and the endpoint is correct."
	}

	if strings.Contains(errStr, "no such host") {
		return "Host not found. Verify the MinIO server hostname is correct."
	}

	if strings.Contains(errStr, "certificate") || strings.Contains(errStr, "x509") {
		return "SSL/TLS certificate error. If using self-signed certificates, set minio_insecure = true."
	}

	if strings.Contains(errStr, "invalid character") {
		return "Invalid response from server. This often indicates the endpoint URL is incorrect (e.g., pointing to a web page instead of the MinIO API)."
	}

	return ""
}

func enhanceAuthError(minioErr minio.ErrorResponse) string {
	switch minioErr.Code {
	case "AccessDenied":
		return "Access denied. Verify your access key and secret key are correct."
	case "InvalidAccessKeyId":
		return "Invalid access key. The access key does not exist."
	case "SignatureDoesNotMatch":
		return "Signature mismatch. The secret key may be incorrect."
	}
	return ""
}
