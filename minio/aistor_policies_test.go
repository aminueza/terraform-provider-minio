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

func TestCanonicalPolicyForAccessType_aistorReadWriteAndWriteOnly(t *testing.T) {
	client := &S3MinioClient{Edition: aistorEdition}

	cases := map[string]string{
		"public-read-write": aistorPolicyReadWrite,
		"public":            aistorPolicyReadWrite,
		"public-write":      aistorPolicyWriteOnly,
	}
	for at, want := range cases {
		got, err := canonicalPolicyForAccessType(at, "any-bucket", client)
		if err != nil {
			t.Fatalf("%s: %v", at, err)
		}
		if strings.Contains(got, `"Principal"`) {
			t.Errorf("%s: AIStor policy must not contain Principal; got: %s", at, got)
		}
		var gotMap, wantMap map[string]interface{}
		if err := json.Unmarshal([]byte(got), &gotMap); err != nil {
			t.Fatalf("%s: not valid JSON: %v (%s)", at, err, got)
		}
		if err := json.Unmarshal([]byte(want), &wantMap); err != nil {
			t.Fatalf("%s: want not valid JSON: %v", at, err)
		}
		if !reflect.DeepEqual(gotMap, wantMap) {
			t.Errorf("%s mismatch\nwant: %v\n got: %v", at, wantMap, gotMap)
		}
	}
}

func TestGetAccessTypeFromPolicy_aistorAllThreeRoundTrip(t *testing.T) {
	client := &S3MinioClient{Edition: aistorEdition}
	cases := map[string]string{
		"public-read":       aistorPolicyReadOnly,
		"public-read-write": aistorPolicyReadWrite,
		"public-write":      aistorPolicyWriteOnly,
	}
	for wantAT, policy := range cases {
		at, err := getAccessTypeFromPolicy(policy, "any-bucket", client)
		if err != nil {
			t.Fatalf("%s: %v", wantAT, err)
		}
		if at != wantAT {
			t.Errorf("expected %s for AIStor canned JSON, got %q", wantAT, at)
		}
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
