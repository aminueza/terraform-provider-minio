package minio

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestIsAIStorClient_nilSafe(t *testing.T) {
	if isAIStorClient(nil) {
		t.Fatal("nil client must not be reported as AIStor")
	}
	if isAIStorClient(&S3MinioClient{}) {
		t.Fatal("empty Edition must not be reported as AIStor")
	}
	if !isAIStorClient(&S3MinioClient{Edition: aistorEdition}) {
		t.Fatal("AIStor edition must be detected")
	}
}

func TestCanonicalPolicyForAccessType_aistorPublicRead(t *testing.T) {
	client := &S3MinioClient{Edition: aistorEdition}
	got, err := canonicalPolicyForAccessType("public-read", "any-bucket", client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []interface{}{
			map[string]interface{}{
				"Effect":   "Allow",
				"Action":   []interface{}{"s3:GetBucketLocation", "s3:GetObject"},
				"Resource": []interface{}{"arn:aws:s3:::*"},
			},
			map[string]interface{}{
				"Effect":   "Deny",
				"Action":   []interface{}{"admin:CreateUser"},
				"Resource": []interface{}{"arn:aws:s3:::*"},
			},
		},
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("policy is not valid JSON: %v (%s)", err, got)
	}
	if !reflect.DeepEqual(decoded, expected) {
		t.Errorf("AIStor public-read policy mismatch\nwant: %v\n got: %v", expected, decoded)
	}
}

func TestCanonicalPolicyForAccessType_openSourceUnchanged(t *testing.T) {
	client := &S3MinioClient{}
	got, err := canonicalPolicyForAccessType("public-read", "mybucket", client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "s3:ListAllMyBuckets") {
		t.Errorf("open-source public-read should keep legacy actions; got: %s", got)
	}
	if !strings.Contains(got, "arn:aws:s3:::mybucket") {
		t.Errorf("open-source public-read should scope to bucket; got: %s", got)
	}
}

func TestCanonicalPolicyForAccessType_aistorFallsBackForUnmappedTypes(t *testing.T) {
	client := &S3MinioClient{Edition: aistorEdition}
	for _, at := range []string{"public", "public-read-write", "public-write"} {
		got, err := canonicalPolicyForAccessType(at, "b", client)
		if err != nil {
			t.Fatalf("%s: %v", at, err)
		}
		if !strings.Contains(got, `"Principal"`) {
			t.Errorf("%s: expected legacy shape until AIStor templates are confirmed; got: %s", at, got)
		}
	}
}

func TestGetAccessTypeFromPolicy_aistorReadOnlyRoundTrip(t *testing.T) {
	client := &S3MinioClient{Edition: aistorEdition}
	at, err := getAccessTypeFromPolicy(aistorPolicyReadOnly, "any-bucket", client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if at != "public-read" {
		t.Errorf("expected public-read for AIStor readonly JSON, got %q", at)
	}
}

func TestGetAccessTypeFromPolicy_openSourceReadOnlyStillWorks(t *testing.T) {
	client := &S3MinioClient{}
	policy, _ := marshalPolicy(ReadOnlyPolicy(&S3MinioBucket{MinioBucket: "b"}))
	at, err := getAccessTypeFromPolicy(policy, "b", client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if at != "public-read" {
		t.Errorf("expected public-read for OSS readonly, got %q", at)
	}
}
