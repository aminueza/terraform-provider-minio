package minio

import (
	"encoding/xml"
	"errors"
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestIsS3TaggingNotImplemented(t *testing.T) {
	t.Run("not implemented", func(t *testing.T) {
		err := &minio.ErrorResponse{Code: "NotImplemented"}
		if !IsS3TaggingNotImplemented(err) {
			t.Fatalf("expected tagging NotImplemented to be detected")
		}
	})

	t.Run("different s3 error", func(t *testing.T) {
		err := &minio.ErrorResponse{Code: "AccessDenied"}
		if IsS3TaggingNotImplemented(err) {
			t.Fatalf("expected AccessDenied to not be detected as NotImplemented")
		}
	})

	t.Run("non s3 error", func(t *testing.T) {
		err := errors.New("generic error")
		if IsS3TaggingNotImplemented(err) {
			t.Fatalf("expected generic error to not be detected as NotImplemented")
		}
	})

	t.Run("unexpected xml response", func(t *testing.T) {
		err := xml.UnmarshalError("expected element type <Tagging> but have <ListBucketResult>")
		if !IsS3TaggingNotImplemented(err) {
			t.Fatalf("expected unexpected XML response to be treated as NotImplemented")
		}
	})
}
