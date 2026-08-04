package minio

import (
	"encoding/json"
	"testing"

	"github.com/minio/minio-go/v7/pkg/policy"
	"gotest.tools/v3/assert"
)

func TestPublicPolicy(t *testing.T) {

	minio := &S3MinioBucket{
		MinioBucket: "test",
	}

	stringPolicy := `{"Version":"2012-10-17","Statement":[{"Sid":"ListBucketActions","Action":["s3:GetBucketLocation","s3:ListBucket","s3:ListBucketMultipartUploads"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::test"]},{"Sid":"AllObjectActions","Action":["s3:AbortMultipartUpload","s3:DeleteObject","s3:GetObject","s3:ListMultipartUploadParts","s3:PutObject"],"Effect":"Allow","Principal":"*","Resource":["arn:aws:s3:::test/*"]}]}`

	var expected BucketPolicy
	if err := json.Unmarshal([]byte(stringPolicy), &expected); err != nil {
		t.Error(err)
	}

	policy := PublicPolicy(minio)
	assert.DeepEqual(t, expected, policy)
}

// The `public` access type grants data access only: reading, writing and listing objects, and
// nothing that administers, discloses or reconfigures the bucket itself. #1089 reviewed every
// action #1085 removed and kept all of them out, so this list is deliberate and complete rather
// than a running tally of whatever has been noticed so far. Anyone who needs more writes it out
// in the resource's `policy` attribute, which takes precedence over `access_type`.
//
// s3:HeadBucket is here because it is redundant, not dangerous: anonymous HEAD on a bucket is
// already authorized under s3:ListBucket. s3:ListAllMyBuckets is a server-level action with no
// effect when scoped to a single bucket ARN.
func TestPublicPolicy_grantsNoAdministrativeActions(t *testing.T) {
	forbidden := []string{
		"s3:CreateBucket",
		"s3:DeleteBucket",
		"s3:DeleteBucketPolicy",
		"s3:PutBucketPolicy",
		"s3:GetBucketPolicy",
		"s3:GetBucketNotification",
		"s3:PutBucketNotification",
		"s3:ListenBucketNotification",
		"s3:HeadBucket",
		"s3:ListAllMyBuckets",
	}

	for _, statement := range PublicPolicy(&S3MinioBucket{MinioBucket: "test"}).Statements {
		for _, action := range forbidden {
			if statement.Actions.Contains(action) {
				t.Errorf("public policy grants %s to anonymous callers", action)
			}
		}
	}
}

func TestPublicPolicy_grantsFullObjectAccess(t *testing.T) {
	objectResource := awsResourcePrefix + "test/*"

	granted := map[string]bool{}
	for _, statement := range PublicPolicy(&S3MinioBucket{MinioBucket: "test"}).Statements {
		if !statement.Resources.Contains(objectResource) {
			continue
		}
		for action := range statement.Actions {
			granted[action] = true
		}
	}

	for _, action := range []string{"s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"} {
		if !granted[action] {
			t.Errorf("public policy does not grant %s on %s", action, objectResource)
		}
	}
}

// mc, the MinIO Console and every other minio-go consumer classify a bucket policy
// client-side with policy.GetPolicy, and report a shape they do not recognize as `custom`.
func TestPublicPolicy_classifiesAsReadWrite(t *testing.T) {
	built, err := marshalPolicy(PublicPolicy(&S3MinioBucket{MinioBucket: "test"}))
	if err != nil {
		t.Fatalf("marshalling policy: %v", err)
	}

	var parsed policy.BucketAccessPolicy
	if err := json.Unmarshal([]byte(built), &parsed); err != nil {
		t.Fatalf("policy is not valid JSON: %v (%s)", err, built)
	}

	if got := policy.GetPolicy(parsed.Statements, "test", ""); got != policy.BucketPolicyReadWrite {
		t.Errorf("expected %q, got %q (mc would report this as `custom`)", policy.BucketPolicyReadWrite, got)
	}
}
