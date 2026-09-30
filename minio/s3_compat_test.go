package minio

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7"
)

func s3CompatTestClient(compat bool) *S3MinioClient {
	return &S3MinioClient{S3CompatMode: compat}
}

func TestIsS3CompatNotSupported(t *testing.T) {
	notImplemented := minio.ErrorResponse{
		StatusCode: 501,
		Code:       "NotImplemented",
		Message:    "A header you provided implies functionality that is not implemented",
	}
	methodNotAllowed := minio.ErrorResponse{
		StatusCode: 405,
		Code:       "MethodNotAllowed",
		Message:    "The specified method is not allowed against this resource",
	}
	madminNotImplemented := madmin.ErrorResponse{
		Code:    "NotImplemented",
		Message: "Tiering is not implemented on this server",
	}

	testCases := []struct {
		name      string
		compat    bool
		nilClient bool
		// methodNotAllowed is what a 405 means at the call site; it defaults to
		// the bucket sub-resource case, which matches both codes.
		methodNotAllowed string
		err              error
		expected         bool
	}{
		{
			name:     "compat on, 501 NotImplemented from the S3 API matches",
			compat:   true,
			err:      notImplemented,
			expected: true,
		},
		{
			name:     "compat on, 405 MethodNotAllowed from the S3 API matches on a bucket read",
			compat:   true,
			err:      methodNotAllowed,
			expected: true,
		},
		{
			name:             "compat on, 405 on an object read does not match, it is an answer about the object",
			compat:           true,
			methodNotAllowed: s3Compat405IsAnAnswer,
			err:              methodNotAllowed,
		},
		{
			name:     "compat on, 501 NotImplemented from the admin API matches",
			compat:   true,
			err:      madminNotImplemented,
			expected: true,
		},
		{
			name:     "compat on, NotImplemented wrapped in a retry error matches",
			compat:   true,
			err:      fmt.Errorf("putting bucket policy: %w", notImplemented),
			expected: true,
		},
		{
			name:     "compat on, NotImplemented returned as a pointer matches",
			compat:   true,
			err:      &minio.ErrorResponse{Code: "NotImplemented", Message: "not implemented"},
			expected: true,
		},
		{
			name:   "compat on, a 501 whose body is not S3 XML matches",
			compat: true,
			// minio-go cannot decode a non-XML error body, so it falls back to
			// the HTTP status line and Code becomes "501 Not Implemented"
			// rather than "NotImplemented". Gateways in front of a backend do
			// this routinely, so the status code is the only reliable signal.
			err:      minio.ErrorResponse{StatusCode: 501, Code: "501 Not Implemented", Message: "Not Implemented"},
			expected: true,
		},
		{
			name:     "compat on, a 405 whose body is not S3 XML matches on a bucket read",
			compat:   true,
			err:      minio.ErrorResponse{StatusCode: 405, Code: "405 Method Not Allowed", Message: "Method Not Allowed"},
			expected: true,
		},
		{
			name:             "compat on, a body-less 405 on an object read does not match",
			compat:           true,
			methodNotAllowed: s3Compat405IsAnAnswer,
			err:              minio.ErrorResponse{StatusCode: 405, Code: "405 Method Not Allowed", Message: "Method Not Allowed"},
		},
		{
			name:   "compat off, a 501 whose body is not S3 XML does not match",
			compat: false,
			err:    minio.ErrorResponse{StatusCode: 501, Code: "501 Not Implemented", Message: "Not Implemented"},
		},
		{
			name:   "compat off, 501 NotImplemented does not match",
			compat: false,
			err:    notImplemented,
		},
		{
			name:   "an unrelated error whose text says unsupported does not match",
			compat: true,
			err:    errors.New("unsupported compression algorithm for the object metadata"),
		},
		{
			name:   "an unrelated error whose text says not supported does not match",
			compat: true,
			err:    errors.New("bucket is not supported by this configuration"),
		},
		{
			name:   "an untyped error carrying the 501 status text does not match",
			compat: true,
			err:    errors.New("unexpected status 501 Not Implemented"),
		},
		{
			name:   "an untyped error carrying the 405 status text does not match",
			compat: true,
			err:    errors.New("unexpected status 405 Method Not Allowed"),
		},
		{
			name:   "a real backend error that is not a missing feature does not match",
			compat: true,
			err:    minio.ErrorResponse{StatusCode: 403, Code: "AccessDenied", Message: "Access Denied"},
		},
		{
			name:   "a no error does not match",
			compat: true,
		},
		{
			name:      "a nil client does not match",
			compat:    true,
			nilClient: true,
			err:       notImplemented,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := s3CompatTestClient(testCase.compat)
			methodNotAllowed := testCase.methodNotAllowed
			if methodNotAllowed == "" {
				methodNotAllowed = s3Compat405IsMissingFeature
			}
			if testCase.nilClient {
				client = nil
			}

			actual := isS3CompatNotSupported(client, methodNotAllowed, testCase.err)
			if actual != testCase.expected {
				t.Fatalf("isS3CompatNotSupported() = %v, expected %v", actual, testCase.expected)
			}
		})
	}
}

func s3CompatTestResource(t *testing.T) (*schema.ResourceData, *schema.Resource) {
	resource := &schema.Resource{
		Schema: map[string]*schema.Schema{
			"bucket":  {Type: schema.TypeString, Optional: true},
			"rule":    {Type: schema.TypeList, Optional: true, Elem: &schema.Resource{Schema: map[string]*schema.Schema{"id": {Type: schema.TypeString, Optional: true}}}},
			"quota":   {Type: schema.TypeInt, Optional: true},
			"enabled": {Type: schema.TypeBool, Optional: true},
			"tags":    {Type: schema.TypeMap, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}},
			"ratio":   {Type: schema.TypeFloat, Optional: true},
		},
	}

	return schema.TestResourceDataRaw(t, resource.Schema, map[string]interface{}{
		"bucket":  "compat-bucket",
		"rule":    []interface{}{map[string]interface{}{"id": "rule-1"}},
		"quota":   4096,
		"enabled": true,
		"tags":    map[string]interface{}{"env": "prod"},
		"ratio":   1.5,
	}), resource
}

func TestS3CompatReadUnsupported(t *testing.T) {
	notImplemented := minio.ErrorResponse{StatusCode: 501, Code: "NotImplemented", Message: "not implemented"}
	methodNotAllowed := minio.ErrorResponse{StatusCode: 405, Code: "MethodNotAllowed", Message: "method not allowed"}

	t.Run("keeps the id and every attribute on 501", func(t *testing.T) {
		d, _ := s3CompatTestResource(t)
		d.SetId("compat-bucket")

		absorbed := s3CompatReadUnsupported(context.Background(), s3CompatTestClient(true), s3Compat405IsMissingFeature, d, "bucket versioning", notImplemented)

		if !absorbed {
			t.Fatal("s3CompatReadUnsupported() must absorb a 501 while s3_compat_mode is on")
		}
		if d.Id() != "compat-bucket" {
			t.Fatalf("resource id = %q, expected it to be kept in state", d.Id())
		}
		// A 501 on the read says nothing about what the write stored, so an
		// emptied attribute would show a diff the next successful read closes.
		if got := d.Get("quota").(int); got != 4096 {
			t.Errorf("attribute quota = %d, expected the state left at 4096", got)
		}
		if got := d.Get("rule").([]interface{}); len(got) != 1 {
			t.Errorf("attribute rule = %#v, expected the single rule it held", got)
		}
	})

	t.Run("keeps the id and every attribute on 405 for a bucket read", func(t *testing.T) {
		d, _ := s3CompatTestResource(t)
		d.SetId("compat-bucket")

		absorbed := s3CompatReadUnsupported(context.Background(), s3CompatTestClient(true), s3Compat405IsMissingFeature, d, "bucket policy", methodNotAllowed)

		if !absorbed {
			t.Fatal("s3CompatReadUnsupported() must absorb a 405 on a bucket read while s3_compat_mode is on")
		}
		if got := d.Get("quota").(int); got != 4096 {
			t.Errorf("attribute quota = %d, expected the state left at 4096", got)
		}
	})

	t.Run("leaves the read to the caller with s3_compat_mode off", func(t *testing.T) {
		d, _ := s3CompatTestResource(t)
		d.SetId("compat-bucket")

		absorbed := s3CompatReadUnsupported(context.Background(), s3CompatTestClient(false), s3Compat405IsMissingFeature, d, "bucket policy", notImplemented)

		if absorbed {
			t.Fatal("s3CompatReadUnsupported() must not absorb the error with s3_compat_mode off")
		}
		if got := d.Get("quota").(int); got != 4096 {
			t.Errorf("attribute quota = %d, expected the state left at 4096", got)
		}
	})

	t.Run("leaves an unrelated error to the caller", func(t *testing.T) {
		d, _ := s3CompatTestResource(t)
		d.SetId("compat-bucket")

		absorbed := s3CompatReadUnsupported(context.Background(), s3CompatTestClient(true), s3Compat405IsMissingFeature, d, "bucket policy", errors.New("unsupported argument"))

		if absorbed {
			t.Fatal("s3CompatReadUnsupported() must not absorb an error that is not a missing feature")
		}
	})
}

func TestS3CompatWriteError(t *testing.T) {
	notImplemented := minio.ErrorResponse{StatusCode: 501, Code: "NotImplemented", Message: "<Error><Code>NotImplemented</Code></Error>"}

	t.Run("names the feature and points at s3_compat_mode", func(t *testing.T) {
		got := s3CompatWriteError(s3CompatTestClient(true), "bucket encryption", notImplemented)

		if got == nil {
			t.Fatal("s3CompatWriteError() must still fail the write")
		}
		if !strings.Contains(got.Error(), "bucket encryption is not supported by this S3 backend") {
			t.Errorf("error does not name the feature: %s", got.Error())
		}
		if !strings.Contains(got.Error(), "s3_compat_mode") {
			t.Errorf("error does not point at s3_compat_mode: %s", got.Error())
		}
	})

	t.Run("returns the error untouched with s3_compat_mode off", func(t *testing.T) {
		got := s3CompatWriteError(s3CompatTestClient(false), "bucket encryption", notImplemented)

		if got.Error() != notImplemented.Error() {
			t.Fatalf("error = %q, expected the backend error unchanged %q", got.Error(), notImplemented.Error())
		}
	})

	t.Run("returns the error untouched when the feature is not the problem", func(t *testing.T) {
		accessDenied := minio.ErrorResponse{StatusCode: 403, Code: "AccessDenied", Message: "Access Denied"}

		got := s3CompatWriteError(s3CompatTestClient(true), "bucket encryption", accessDenied)

		if got.Error() != accessDenied.Error() {
			t.Fatalf("error = %q, expected the backend error unchanged %q", got.Error(), accessDenied.Error())
		}
	})
}
