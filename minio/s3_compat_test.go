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
		err       error
		expected  bool
	}{
		{
			name:     "compat on, 501 NotImplemented from the S3 API matches",
			compat:   true,
			err:      notImplemented,
			expected: true,
		},
		{
			name:     "compat on, 405 MethodNotAllowed from the S3 API matches",
			compat:   true,
			err:      methodNotAllowed,
			expected: true,
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
			name:     "compat on, a 405 whose body is not S3 XML matches",
			compat:   true,
			err:      minio.ErrorResponse{StatusCode: 405, Code: "405 Method Not Allowed", Message: "Method Not Allowed"},
			expected: true,
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
			if testCase.nilClient {
				client = nil
			}

			actual := isS3CompatNotSupported(client, testCase.err)
			if actual != testCase.expected {
				t.Fatalf("isS3CompatNotSupported() = %v, expected %v", actual, testCase.expected)
			}
		})
	}
}

func s3CompatZeroTestResource(t *testing.T) (*schema.ResourceData, *schema.Resource) {
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

func TestS3CompatZeroAttributes(t *testing.T) {
	d, _ := s3CompatZeroTestResource(t)
	d.SetId("compat-bucket")

	diags := s3CompatZeroAttributes(d, []string{"rule", "quota", "enabled", "tags", "ratio"})
	if diags.HasError() {
		t.Fatalf("s3CompatZeroAttributes() returned errors: %v", diags)
	}

	// Asserted against literals: the code under test must not be what computes the
	// expected value, or the test only proves that it agrees with itself.
	expected := map[string]interface{}{
		"rule":    []interface{}{},
		"quota":   0,
		"enabled": false,
		"tags":    map[string]interface{}{},
		"ratio":   float64(0),
	}
	for attribute, want := range expected {
		got := d.Get(attribute)
		if !s3CompatValuesEqual(got, want) {
			t.Errorf("attribute %q = %#v, expected %#v", attribute, got, want)
		}
	}

	// A resource must stay in state, otherwise the next plan recreates it forever.
	if d.Id() == "" {
		t.Fatal("s3CompatZeroAttributes() must not clear the resource id")
	}
}

func s3CompatValuesEqual(got interface{}, want interface{}) bool {
	gotList, gotIsList := got.([]interface{})
	wantList, wantIsList := want.([]interface{})
	if gotIsList != wantIsList {
		return false
	}
	if gotIsList {
		return len(gotList) == len(wantList)
	}

	gotMap, gotIsMap := got.(map[string]interface{})
	wantMap, wantIsMap := want.(map[string]interface{})
	if gotIsMap != wantIsMap {
		return false
	}
	if gotIsMap {
		return len(gotMap) == len(wantMap)
	}

	return got == want
}

func TestS3CompatZeroValue(t *testing.T) {
	supported := []struct {
		value interface{}
		want  interface{}
	}{
		{value: "a string", want: ""},
		{value: true, want: false},
		{value: 7, want: 0},
		{value: 1.5, want: float64(0)},
		{value: []interface{}{"a"}, want: []interface{}{}},
		{value: map[string]interface{}{"a": "b"}, want: map[string]interface{}{}},
	}

	for _, testCase := range supported {
		got, ok := s3CompatZeroValue(testCase.value)
		if !ok {
			t.Errorf("s3CompatZeroValue(%#v) reported no zero value", testCase.value)
			continue
		}
		if !s3CompatValuesEqual(got, testCase.want) {
			t.Errorf("s3CompatZeroValue(%#v) = %#v, expected %#v", testCase.value, got, testCase.want)
		}
	}

	if _, ok := s3CompatZeroValue(nil); ok {
		t.Error("s3CompatZeroValue(nil) must report no zero value, so an unknown type is never silently overwritten")
	}
	if _, ok := s3CompatZeroValue(map[string]string{"a": "b"}); ok {
		t.Error("s3CompatZeroValue(map[string]string) must report no zero value")
	}
}

func TestS3CompatZeroAttributesRejectsUnknownType(t *testing.T) {
	d, _ := s3CompatZeroTestResource(t)
	d.SetId("compat-bucket")

	// A type the zero-value table does not cover must raise, not silently leave a
	// stale value in state.
	diags := s3CompatZeroAttributes(d, []string{"unsupported_type"})

	if !diags.HasError() {
		t.Fatal("s3CompatZeroAttributes() must return an error for an attribute whose zero value is unknown")
	}
	if !strings.Contains(diags[0].Summary, "cannot compute a zero value") {
		t.Fatalf("unexpected diagnostic: %s", diags[0].Summary)
	}
}

func TestS3CompatReadUnsupported(t *testing.T) {
	notImplemented := minio.ErrorResponse{StatusCode: 501, Code: "NotImplemented", Message: "not implemented"}
	methodNotAllowed := minio.ErrorResponse{StatusCode: 405, Code: "MethodNotAllowed", Message: "method not allowed"}

	t.Run("keeps the resource in state and zeroes the attributes on 501", func(t *testing.T) {
		d, _ := s3CompatZeroTestResource(t)
		d.SetId("compat-bucket")

		absorbed, diags := s3CompatReadUnsupported(context.Background(), s3CompatTestClient(true), d, "bucket versioning", notImplemented, []string{"rule"})

		if !absorbed {
			t.Fatal("s3CompatReadUnsupported() must absorb a 501 while s3_compat_mode is on")
		}
		if diags.HasError() {
			t.Fatalf("s3CompatReadUnsupported() returned errors: %v", diags)
		}
		if d.Id() != "compat-bucket" {
			t.Fatalf("resource id = %q, expected it to be kept in state", d.Id())
		}
		if got := d.Get("rule").([]interface{}); len(got) != 0 {
			t.Fatalf("attribute rule = %#v, expected it to be emptied", got)
		}
	})

	t.Run("keeps the resource in state and zeroes the attributes on 405", func(t *testing.T) {
		d, _ := s3CompatZeroTestResource(t)
		d.SetId("compat-bucket")

		absorbed, diags := s3CompatReadUnsupported(context.Background(), s3CompatTestClient(true), d, "bucket policy", methodNotAllowed, []string{"quota"})

		if !absorbed {
			t.Fatal("s3CompatReadUnsupported() must absorb a 405 while s3_compat_mode is on")
		}
		if diags.HasError() {
			t.Fatalf("s3CompatReadUnsupported() returned errors: %v", diags)
		}
		if got := d.Get("quota").(int); got != 0 {
			t.Fatalf("attribute quota = %d, expected 0", got)
		}
	})

	t.Run("leaves the read to the caller with s3_compat_mode off", func(t *testing.T) {
		d, _ := s3CompatZeroTestResource(t)
		d.SetId("compat-bucket")

		absorbed, diags := s3CompatReadUnsupported(context.Background(), s3CompatTestClient(false), d, "bucket policy", notImplemented, []string{"quota"})

		if absorbed {
			t.Fatal("s3CompatReadUnsupported() must not absorb the error with s3_compat_mode off")
		}
		if diags.HasError() {
			t.Fatalf("s3CompatReadUnsupported() returned errors: %v", diags)
		}
		if got := d.Get("quota").(int); got != 4096 {
			t.Fatalf("attribute quota = %d, expected the state left at 4096", got)
		}
	})

	t.Run("leaves an unrelated error to the caller", func(t *testing.T) {
		d, _ := s3CompatZeroTestResource(t)
		d.SetId("compat-bucket")

		absorbed, _ := s3CompatReadUnsupported(context.Background(), s3CompatTestClient(true), d, "bucket policy", errors.New("unsupported argument"), []string{"quota"})

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
