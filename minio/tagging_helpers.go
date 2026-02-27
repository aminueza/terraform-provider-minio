package minio

import (
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
)

// shouldSkipBucketTagging returns true when tagging operations should be skipped
// for the given bucket configuration (either because tagging is disabled via the
// provider flag or the config object is nil).
func shouldSkipBucketTagging(cfg *S3MinioBucket) bool {
	if cfg == nil {
		return false
	}

	return cfg.SkipBucketTagging
}

// preserveBucketTagsState ensures Terraform state retains the last known set of
// tags even when we skip remote API calls.
func preserveBucketTagsState(d *schema.ResourceData) {
	if d == nil {
		return
	}

	if v, ok := d.GetOkExists("tags"); ok {
		switch tags := v.(type) {
		case map[string]string:
			_ = d.Set("tags", tags)
		case map[string]interface{}:
			_ = d.Set("tags", convertToStringMap(tags))
		default:
			// ignore unexpected types; Terraform will maintain existing state
		}
	} else {
		_ = d.Set("tags", map[string]string{})
	}
}

// IsS3TaggingNotImplemented attempts to detect when bucket tagging operations
// are not supported by the remote S3-compatible backend. We look for well-known
// error codes, malformed XML payloads, or generic "not implemented" messages.
func IsS3TaggingNotImplemented(err error) bool {
	if err == nil {
		return false
	}

	var minioErr minio.ErrorResponse
	if errors.As(err, &minioErr) {
		if isTaggingNotImplementedCode(minioErr.Code) || isTaggingNotImplementedMessage(minioErr.Message) {
			return true
		}
	}

	if resp, ok := err.(*minio.ErrorResponse); ok {
		if isTaggingNotImplementedCode(resp.Code) || isTaggingNotImplementedMessage(resp.Message) {
			return true
		}
	}

	if isBucketTaggingUnexpectedResponse(err) {
		return true
	}

	if isGenericTaggingNotImplementedMessage(err) {
		return true
	}

	return false
}

func isTaggingNotImplementedCode(code string) bool {
	switch code {
	case "NotImplemented", "XNotImplemented", "NotImplementedError", "NotImplementedException":
		return true
	default:
		return false
	}
}

func isTaggingNotImplementedMessage(message string) bool {
	if message == "" {
		return false
	}

	lower := strings.ToLower(message)
	return strings.Contains(lower, "not implemented") || strings.Contains(lower, "not supported")
}

func isGenericTaggingNotImplementedMessage(err error) bool {
	if err == nil {
		return false
	}

	return isTaggingNotImplementedMessage(err.Error())
}

func isBucketTaggingUnexpectedResponse(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return strings.Contains(msg, "expected element type <Tagging>") && strings.Contains(msg, "<ListBucketResult>")
}
