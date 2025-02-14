package minio

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

const (
	// ErrorSeverityFatal indicates a fatal error that prevents further execution
	ErrorSeverityFatal = "[FATAL]"
)

// ResourceError represents an error that occurred while managing a MinIO resource
type ResourceError struct {
	Message  string
	Resource string
	Err      error
}

// Error implements the error interface for ResourceError
func (e *ResourceError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s %s (%s): %v", ErrorSeverityFatal, e.Message, e.Resource, e.Err)
	}
	return fmt.Sprintf("%s %s (%s)", ErrorSeverityFatal, e.Message, e.Resource)
}

// NewResourceError creates a diagnostic error for a MinIO resource operation.
// It handles different error types and formats them consistently:
//   - diag.Diagnostics: appends a new diagnostic
//   - error: creates a new diagnostic with error details
//   - other: creates a new diagnostic with string representation
//
// Parameters:
//   - msg: A descriptive message about what operation failed
//   - resource: The name or identifier of the resource that failed
//   - err: The underlying error (can be diag.Diagnostics, error, or other type)
//
// Returns:
//   - diag.Diagnostics containing the formatted error
func NewResourceError(msg string, resource string, err interface{}) diag.Diagnostics {
	// Create a ResourceError for consistent error formatting
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
		return diag.Errorf("%s", resErr.Error())
	default:
		return diag.Errorf("%s %s (%s): %v", ErrorSeverityFatal, msg, resource, err)
	}
}

// NewResourceErrorStr creates a string representation of a resource error.
// This is useful when you need to return a string error message instead of diagnostics.
//
// Parameters:
//   - msg: A descriptive message about what operation failed
//   - resource: The name or identifier of the resource that failed
//   - err: The underlying error (can be diag.Diagnostics, error, or other type)
//
// Returns:
//   - A string containing all error messages joined with commas
func NewResourceErrorStr(msg string, resource string, err interface{}) string {
	diags := NewResourceError(msg, resource, err)
	if len(diags) == 0 {
		return ""
	}

	// Create a slice with the correct initial capacity
	strs := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.Summary != "" {
			strs = append(strs, d.Summary)
		}
	}

	return strings.Join(strs, ", ")
}
