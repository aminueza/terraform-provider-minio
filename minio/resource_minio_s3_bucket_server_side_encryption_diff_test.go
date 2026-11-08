package minio

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestBucketServerSideEncryptionKMSKeyIDDiff(t *testing.T) {
	cases := []struct {
		name        string
		stateType   string
		stateKey    string
		configType  string
		configKey   string
		wantKeyDiff bool
	}{
		{name: "AES256 with a kms_key_id in config", stateType: "AES256", stateKey: "", configType: "AES256", configKey: "my-key", wantKeyDiff: false},
		{name: "aws:kms key changed", stateType: "aws:kms", stateKey: "old-key", configType: "aws:kms", configKey: "new-key", wantKeyDiff: true},
		{name: "AES256 to aws:kms", stateType: "AES256", stateKey: "", configType: "aws:kms", configKey: "my-key", wantKeyDiff: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := resourceMinioBucketServerSideEncryption()
			state := &terraform.InstanceState{
				ID: "test-bucket",
				Attributes: map[string]string{
					"id":              "test-bucket",
					"bucket":          "test-bucket",
					"encryption_type": tc.stateType,
					"kms_key_id":      tc.stateKey,
				},
			}
			config := terraform.NewResourceConfigRaw(map[string]interface{}{
				"bucket":          "test-bucket",
				"encryption_type": tc.configType,
				"kms_key_id":      tc.configKey,
			})

			diff, err := r.Diff(context.Background(), state, config, nil)
			if err != nil {
				t.Fatalf("diff: %v", err)
			}

			gotKeyDiff := false
			if diff != nil {
				_, gotKeyDiff = diff.GetAttribute("kms_key_id")
			}
			if gotKeyDiff != tc.wantKeyDiff {
				t.Fatalf("kms_key_id in diff = %v, want %v (diff: %#v)", gotKeyDiff, tc.wantKeyDiff, diff)
			}
		})
	}
}
