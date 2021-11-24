package minio

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

// NewResourceError creates a new error with the given msg argument.
func NewResourceError(msg string, resource string, err interface{}) diag.Diagnostics {
	switch err.(type) {
	case diag.Diagnostics:
		return append(err.(diag.Diagnostics), diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("[FATAL] %s (%s)", msg, resource),
		})
	case error:
		return diag.Errorf("[FATAL] %s (%s): %s", msg, resource, err)
	}
	return diag.Errorf("[FATAL] %s (%s): %v", msg, resource, err)
}

// NewResourceErrorStr creates a new error with the given msg argument.
func NewResourceErrorStr(msg string, resource string, err interface{}) string {
	diags := NewResourceError(msg, resource, err)
	strs := make([]string, len(diags))
	for _, d := range diags {
		strs = append(strs, d.Summary)
	}
	return strings.Join(strs, ", ")
}
