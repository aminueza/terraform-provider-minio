package minio

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7"
)

const s3CompatMethodNotAllowed = "MethodNotAllowed"

// A backend that never implemented an S3 feature answers with 501 NotImplemented,
// and one that implements it on another method answers with 405 MethodNotAllowed.
// Matching the typed response keeps an unrelated error whose text happens to carry
// the word "unsupported" from being mistaken for a missing feature. See issue #1173.
var s3CompatNotImplementedCodes = map[string]bool{
	"NotImplemented": true,
}

var s3CompatNotImplementedStatuses = map[int]bool{
	http.StatusNotImplemented: true,
}

// What a 405 MethodNotAllowed means depends on what was asked, so the call site
// says which of the two it is. See isS3CompatNotSupported.
const (
	// s3Compat405IsMissingFeature covers the bucket sub-resources and every
	// write: a backend that implements the feature on another method answers
	// 405 to the request the provider made, which is a missing feature.
	s3Compat405IsMissingFeature = "method-not-allowed-is-missing-feature"
	// s3Compat405IsAnAnswer covers the object-level reads. S3 and MinIO answer
	// an object GET or HEAD whose versionId is a delete marker with 405, and
	// minio-go synthesises exactly that code in api-stat.go, so there a 405 is
	// an answer about the object rather than a missing feature.
	s3Compat405IsAnAnswer = "method-not-allowed-is-an-answer"
)

func isS3CompatNotSupported(client *S3MinioClient, methodNotAllowed string, err error) bool {
	if client == nil || !client.S3CompatMode || err == nil {
		return false
	}
	code, status := s3CompatErrorResponse(err)
	if s3CompatNotImplementedCodes[code] || s3CompatNotImplementedStatuses[status] {
		return true
	}
	if methodNotAllowed != s3Compat405IsMissingFeature {
		return false
	}
	return code == s3CompatMethodNotAllowed || status == http.StatusMethodNotAllowed
}

// s3CompatErrorResponse returns the S3 error code and the HTTP status of a typed
// backend error. Both are needed: minio-go fills Code with the S3 error code only
// when the error body is S3 XML, and falls back to the HTTP status line otherwise,
// so a backend behind a gateway that answers 501 with a plain-text or empty body
// reports Code "501 Not Implemented" and is recognisable only by its status.
// madmin.ErrorResponse carries no status, so only its code is available.
func s3CompatErrorResponse(err error) (string, int) {
	var minioErr minio.ErrorResponse
	if errors.As(err, &minioErr) {
		return minioErr.Code, minioErr.StatusCode
	}

	var minioErrPtr *minio.ErrorResponse
	if errors.As(err, &minioErrPtr) && minioErrPtr != nil {
		return minioErrPtr.Code, minioErrPtr.StatusCode
	}

	var madminErr madmin.ErrorResponse
	if errors.As(err, &madminErr) {
		return madminErr.Code, 0
	}

	return "", 0
}

// s3CompatReadUnsupported absorbs a "this backend does not implement the feature"
// error raised while reading, when s3_compat_mode is on. The resource keeps its id
// and the attributes the provider last managed to write: dropping it would plan a
// create that fails the same way, and the read says nothing about what the write
// stored, so emptying the attributes would only manufacture a diff the next
// successful read removes again. It reports whether the caller should return no
// diagnostics. See issue #1173.
func s3CompatReadUnsupported(ctx context.Context, client *S3MinioClient, methodNotAllowed string, d *schema.ResourceData, feature string, err error) bool {
	if !isS3CompatNotSupported(client, methodNotAllowed, err) {
		return false
	}

	tflog.Warn(ctx, fmt.Sprintf(
		"%s is not supported by this S3 backend; keeping resource %s in state with its last known attributes because s3_compat_mode is true. Set s3_compat_mode = false to make this an error instead.",
		feature, d.Id(),
	))

	return true
}

// s3CompatWriteError names the missing feature in a failed create or update, so
// that a backend answering 501 does not surface as its raw XML. s3_compat_mode
// never skips a write: a bucket policy or a bucket encryption rule that was
// silently dropped is worse than an error. With the flag off the error is
// returned untouched, so the reported failure is unchanged. See issue #1173.
func s3CompatWriteError(client *S3MinioClient, feature string, err error) error {
	if !isS3CompatNotSupported(client, s3Compat405IsMissingFeature, err) {
		return err
	}

	return fmt.Errorf("%s is not supported by this S3 backend, and s3_compat_mode does not skip writes: %w", feature, err)
}
