package minio

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7/pkg/policy"
)

// mc, the MinIO Console and every other minio-go consumer classify a bucket policy client-side
// with policy.GetPolicy, and report a shape they do not recognize as `custom`. A canned policy
// that lands on `none` here is one no MinIO client can name.
func TestCannedPolicies_classifyForMinioClients(t *testing.T) {
	for accessType, want := range map[string]policy.BucketPolicy{
		"public-read":       policy.BucketPolicyReadOnly,
		"public-write":      policy.BucketPolicyWriteOnly,
		"public-read-write": policy.BucketPolicyReadWrite,
		"public":            policy.BucketPolicyReadWrite,
	} {
		t.Run(accessType, func(t *testing.T) {
			built, err := canonicalPolicyForAccessType(accessType, "test")
			if err != nil {
				t.Fatalf("building policy: %v", err)
			}

			var parsed policy.BucketAccessPolicy
			if err := json.Unmarshal([]byte(built), &parsed); err != nil {
				t.Fatalf("policy is not valid JSON: %v (%s)", err, built)
			}

			got := policy.GetPolicy(parsed.Statements, "test", "")
			if got == policy.BucketPolicyNone {
				t.Fatalf("mc would report this shape as `custom`: %s", built)
			}
			if got != want {
				t.Errorf("expected %q, got %q: %s", want, got, built)
			}
		})
	}
}

// MinIO has three anonymous shapes and the provider offers four access types, so classifying by
// shape alone cannot tell `public-read-write` and `public` apart and answers `public` for both.
func TestGetAccessTypeFromPolicy_roundTripsCannedShapes(t *testing.T) {
	for accessType, want := range map[string]string{
		"public-read":       "public-read",
		"public-write":      "public-write",
		"public":            "public",
		"public-read-write": "public",
	} {
		t.Run(accessType, func(t *testing.T) {
			built, err := canonicalPolicyForAccessType(accessType, "test")
			if err != nil {
				t.Fatalf("building policy: %v", err)
			}

			got, err := getAccessTypeFromPolicy(built, "test")
			if err != nil {
				t.Fatalf("classifying policy: %v", err)
			}
			if got != want {
				t.Errorf("expected %q, got %q: %s", want, got, built)
			}
		})
	}
}

// Without this the shared shape would rename `public-read-write` to `public` on every read and
// the resource would never reach a clean plan.
func TestConfirmConfiguredAccessType_keepsTheConfiguredTypeForASharedShape(t *testing.T) {
	built, err := canonicalPolicyForAccessType("public-read-write", "test")
	if err != nil {
		t.Fatalf("building policy: %v", err)
	}

	d := schema.TestResourceDataRaw(t, resourceMinioS3BucketAnonymousAccess().Schema, map[string]interface{}{
		"bucket":      "test",
		"access_type": "public-read-write",
	})

	got, err := confirmConfiguredAccessType(d, built, "test")
	if err != nil {
		t.Fatalf("confirming access_type: %v", err)
	}
	if got != "public-read-write" {
		t.Errorf("expected the configured access_type to be kept, got %q", got)
	}
}

func TestConfirmConfiguredAccessType_rejectsAStaleAccessType(t *testing.T) {
	built, err := canonicalPolicyForAccessType("public", "test")
	if err != nil {
		t.Fatalf("building policy: %v", err)
	}

	d := schema.TestResourceDataRaw(t, resourceMinioS3BucketAnonymousAccess().Schema, map[string]interface{}{
		"bucket":      "test",
		"access_type": "public-read",
	})

	got, err := confirmConfiguredAccessType(d, built, "test")
	if err != nil {
		t.Fatalf("confirming access_type: %v", err)
	}
	if got != "" {
		t.Errorf("expected an access_type that no longer describes the bucket to be dropped, got %q", got)
	}
}

// The read only derives acl for a bucket whose acl is a canned public value, so
// an empty policy on the server means the bucket is private again for any of
// them.
func TestBucketACLFromPolicy_emptyPolicyIsPrivate(t *testing.T) {
	for current := range map[string]bool{"public": true, "public-read-write": true} {
		got, err := bucketACLFromPolicy("", "test", current)
		if err != nil {
			t.Fatalf("deriving acl: %v", err)
		}
		if got != "private" {
			t.Errorf("expected %q, got %q", "private", got)
		}
	}
}

// public and public-read-write write the same policy shape, so the value already
// held is kept when it is one of them, and any other held value reports the
// shape as public.
func TestBucketACLFromPolicy_keepsHeldValueForSharedShape(t *testing.T) {
	built, err := canonicalPolicyForAccessType("public", "test")
	if err != nil {
		t.Fatalf("building policy: %v", err)
	}

	for current, want := range map[string]string{
		"public":            "public",
		"public-read-write": "public-read-write",
		"public-read":       "public",
	} {
		t.Run(current, func(t *testing.T) {
			got, err := bucketACLFromPolicy(built, "test", current)
			if err != nil {
				t.Fatalf("deriving acl: %v", err)
			}
			if got != want {
				t.Errorf("expected %q, got %q", want, got)
			}
		})
	}
}

// The read-only and write-only shapes map onto their acl values regardless of
// the value held in state.
func TestBucketACLFromPolicy_mapsReadAndWriteShapes(t *testing.T) {
	for accessType, want := range map[string]string{
		"public-read":  "public-read",
		"public-write": "public-write",
	} {
		t.Run(accessType, func(t *testing.T) {
			built, err := canonicalPolicyForAccessType(accessType, "test")
			if err != nil {
				t.Fatalf("building policy: %v", err)
			}

			for current := range map[string]bool{"public": true, "public-read-write": true} {
				got, err := bucketACLFromPolicy(built, "test", current)
				if err != nil {
					t.Fatalf("deriving acl: %v", err)
				}
				if got != want {
					t.Errorf("expected %q, got %q", want, got)
				}
			}
		})
	}
}

// A policy that matches no canned shape is reported as private so the plan
// shows the drift and the next apply puts the configured ACL back over it.
func TestBucketACLFromPolicy_customPolicyReportsPrivate(t *testing.T) {
	custom := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["arn:aws:iam::123456789012:user/bob"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::test/*"]}]}`
	for current := range map[string]bool{"public": true, "public-read": true, "public-read-write": true} {
		got, err := bucketACLFromPolicy(custom, "test", current)
		if err != nil {
			t.Fatalf("deriving acl: %v", err)
		}
		if got != "private" {
			t.Errorf("expected acl %q to be reported as private, got %q", current, got)
		}
	}
}
