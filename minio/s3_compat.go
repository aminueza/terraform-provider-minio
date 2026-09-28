package minio

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7"
)

// A backend that never implemented an S3 feature answers with 501 NotImplemented,
// and one that implements it on another method answers with 405 MethodNotAllowed.
// Matching the typed code keeps an unrelated error whose text happens to carry
// the word "unsupported" from being mistaken for a missing feature. See issue #1173.
var s3CompatNotImplementedCodes = map[string]bool{
	"NotImplemented":   true,
	"MethodNotAllowed": true,
}

func isS3CompatNotSupported(client *S3MinioClient, err error) bool {
	if client == nil || !client.S3CompatMode || err == nil {
		return false
	}
	code, ok := s3CompatErrorCode(err)
	return ok && s3CompatNotImplementedCodes[code]
}

func s3CompatErrorCode(err error) (string, bool) {
	var minioErr minio.ErrorResponse
	if errors.As(err, &minioErr) {
		return minioErr.Code, minioErr.Code != ""
	}

	if pointerErr, ok := err.(*minio.ErrorResponse); ok {
		return pointerErr.Code, pointerErr.Code != ""
	}

	var madminErr madmin.ErrorResponse
	if errors.As(err, &madminErr) {
		return madminErr.Code, madminErr.Code != ""
	}

	return "", false
}

// s3CompatReadUnsupported absorbs a "this backend does not implement the feature"
// error raised while reading, when s3_compat_mode is on. The resource stays in
// state with the attributes the backend cannot answer reset to their zero values:
// dropping it would plan a create that fails the same way, and keeping the stale
// values would hide that the backend never applied them. It reports whether the
// caller should return no diagnostics. See issue #1173.
func s3CompatReadUnsupported(ctx context.Context, client *S3MinioClient, d *schema.ResourceData, feature string, err error, attributes []string) (bool, diag.Diagnostics) {
	if !isS3CompatNotSupported(client, err) {
		return false, nil
	}

	tflog.Warn(ctx, fmt.Sprintf(
		"%s is not supported by this S3 backend; keeping resource %s in state with %s empty because s3_compat_mode is true. Set s3_compat_mode = false to make this an error instead.",
		feature, d.Id(), attributes,
	))

	return true, s3CompatZeroAttributes(d, attributes)
}

// s3CompatZeroAttributes resets each attribute to the zero value of its schema
// type, so a read against a backend that does not implement the feature reports an
// empty value instead of the last value the provider managed to write.
func s3CompatZeroAttributes(d *schema.ResourceData, attributes []string) diag.Diagnostics {
	var diags diag.Diagnostics

	for _, attribute := range attributes {
		current := d.Get(attribute)
		zero, ok := s3CompatZeroValue(current)
		if !ok {
			diags = append(diags, NewResourceError(
				"resetting an attribute the backend does not support",
				d.Id(),
				fmt.Errorf("cannot compute a zero value for %q of type %T", attribute, current),
			)...)
			continue
		}
		if err := d.Set(attribute, zero); err != nil {
			diags = append(diags, NewResourceError("resetting an attribute the backend does not support", d.Id(), err)...)
		}
	}

	return diags
}

func s3CompatZeroValue(value interface{}) (interface{}, bool) {
	switch value.(type) {
	case string:
		return "", true
	case bool:
		return false, true
	case int:
		return 0, true
	case float64:
		return float64(0), true
	case []interface{}:
		return []interface{}{}, true
	case map[string]interface{}:
		return map[string]interface{}{}, true
	}

	return nil, false
}

// s3CompatWriteError names the missing feature in a failed create or update, so
// that a backend answering 501 does not surface as its raw XML. s3_compat_mode
// never skips a write: a bucket policy or a bucket encryption rule that was
// silently dropped is worse than an error. With the flag off the error is
// returned untouched, so the reported failure is unchanged. See issue #1173.
func s3CompatWriteError(client *S3MinioClient, feature string, err error) error {
	if !isS3CompatNotSupported(client, err) {
		return err
	}

	return fmt.Errorf("%s is not supported by this S3 backend, and s3_compat_mode does not skip writes: %w", feature, err)
}
