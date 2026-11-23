package minio

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestValidateIAMPolicyNamesAndPrefixLimits(t *testing.T) {
	if id.UniqueIDSuffixLength != 26 {
		t.Fatalf("UniqueIDSuffixLength = %d, want 26", id.UniqueIDSuffixLength)
	}

	strLen := func(n int) string {
		return strings.Repeat("a", n)
	}

	t.Run("validateIAMNamePolicy", func(t *testing.T) {
		valid := []struct {
			name  string
			value string
		}{
			{name: "100 chars", value: strLen(100)},
			{name: "229 chars", value: strLen(229)},
			{name: "230 chars", value: strLen(230)},
			{name: "255 chars", value: strLen(255)},
			{name: "allowed punctuation without comma", value: "policy_+=.@:/-1"},
		}
		for _, tc := range valid {
			_, errs := validateIAMNamePolicy(tc.value, "name")
			if len(errs) != 0 {
				t.Errorf("%s (%d chars): unexpected errors: %v", tc.name, len(tc.value), errs)
			}
		}

		invalid := []struct {
			name     string
			value    string
			wantText string
		}{
			{name: "256 chars", value: strLen(256), wantText: "255"},
			{name: "comma", value: "a,b", wantText: "must match"},
			{name: "space", value: "bad policy", wantText: "must match"},
		}
		for _, tc := range invalid {
			_, errs := validateIAMNamePolicy(tc.value, "name")
			if len(errs) == 0 {
				t.Errorf("%s: expected validation error, got nil", tc.name)
				continue
			}
			if !strings.Contains(errs[0].Error(), tc.wantText) {
				t.Errorf("%s: error = %q, want substring %q", tc.name, errs[0].Error(), tc.wantText)
			}
		}
	})

	t.Run("validateIAMNamePrefixPolicy", func(t *testing.T) {
		valid := []struct {
			name  string
			value string
		}{
			{name: "100 chars", value: strLen(100)},
			{name: "229 chars", value: strLen(229)},
			{name: "allowed punctuation without comma", value: "prefix_+=.@:/-"},
		}
		for _, tc := range valid {
			_, errs := validateIAMNamePrefixPolicy(tc.value, "name_prefix")
			if len(errs) != 0 {
				t.Errorf("%s (%d chars): unexpected errors: %v", tc.name, len(tc.value), errs)
			}
		}

		invalid := []struct {
			name     string
			value    string
			wantText string
		}{
			{name: "230 chars", value: strLen(230), wantText: "229"},
			{name: "comma", value: "a,b", wantText: "must match"},
		}
		for _, tc := range invalid {
			_, errs := validateIAMNamePrefixPolicy(tc.value, "name_prefix")
			if len(errs) == 0 {
				t.Errorf("%s: expected validation error, got nil", tc.name)
				continue
			}
			if !strings.Contains(errs[0].Error(), tc.wantText) {
				t.Errorf("%s: error = %q, want substring %q", tc.name, errs[0].Error(), tc.wantText)
			}
		}
	})
}

func TestValidateIAMResourceSchemasPolicyAndGroupNames(t *testing.T) {
	const policyDoc = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::*"]}]}`
	p255 := strings.Repeat("p", 255)
	p256 := strings.Repeat("p", 256)
	p229 := strings.Repeat("p", 229)
	p230 := strings.Repeat("p", 230)
	g100 := strings.Repeat("g", 100)

	cases := []struct {
		name      string
		res       *schema.Resource
		raw       map[string]interface{}
		wantError string
	}{
		{
			name: "iam_policy 255 name passes",
			res:  resourceMinioIAMPolicy(),
			raw:  map[string]interface{}{"name": p255, "policy": policyDoc},
		},
		{
			name:      "iam_policy 256 name fails with 255",
			res:       resourceMinioIAMPolicy(),
			raw:       map[string]interface{}{"name": p256, "policy": policyDoc},
			wantError: "255",
		},
		{
			name:      "iam_policy comma name fails",
			res:       resourceMinioIAMPolicy(),
			raw:       map[string]interface{}{"name": "a,b", "policy": policyDoc},
			wantError: "must match",
		},
		{
			name: "iam_policy 229 name_prefix passes",
			res:  resourceMinioIAMPolicy(),
			raw:  map[string]interface{}{"name_prefix": p229, "policy": policyDoc},
		},
		{
			name:      "iam_policy 230 name_prefix fails with 229",
			res:       resourceMinioIAMPolicy(),
			raw:       map[string]interface{}{"name_prefix": p230, "policy": policyDoc},
			wantError: "229",
		},
		{
			name: "iam_group_policy 255 name passes",
			res:  resourceMinioIAMGroupPolicy(),
			raw:  map[string]interface{}{"name": p255, "group": "grp", "policy": policyDoc},
		},
		{
			name:      "iam_group_policy 230 name_prefix fails",
			res:       resourceMinioIAMGroupPolicy(),
			raw:       map[string]interface{}{"name_prefix": p230, "group": "grp", "policy": policyDoc},
			wantError: "229",
		},
		{
			name: "iam_group_policy_attachment 255 policy_name passes",
			res:  resourceMinioIAMGroupPolicyAttachment(),
			raw:  map[string]interface{}{"policy_name": p255, "group_name": g100},
		},
		{
			name:      "iam_group_policy_attachment comma policy_name fails",
			res:       resourceMinioIAMGroupPolicyAttachment(),
			raw:       map[string]interface{}{"policy_name": "a,b", "group_name": "grp"},
			wantError: "must match",
		},
		{
			name: "iam_user_policy_attachment 255 policy_name passes",
			res:  resourceMinioIAMUserPolicyAttachment(),
			raw:  map[string]interface{}{"policy_name": p255, "user_name": "alice"},
		},
		{
			name:      "iam_user_policy_attachment comma policy_name fails",
			res:       resourceMinioIAMUserPolicyAttachment(),
			raw:       map[string]interface{}{"policy_name": "a,b", "user_name": "alice"},
			wantError: "must match",
		},
		{
			name: "iam_ldap_group_policy_attachment 255 policy_name passes",
			res:  resourceMinioIAMLDAPGroupPolicyAttachment(),
			raw:  map[string]interface{}{"policy_name": p255, "group_dn": "CN=admins,OU=Groups,DC=example,DC=com"},
		},
		{
			name:      "iam_ldap_group_policy_attachment comma policy_name fails",
			res:       resourceMinioIAMLDAPGroupPolicyAttachment(),
			raw:       map[string]interface{}{"policy_name": "a,b", "group_dn": "CN=admins,OU=Groups,DC=example,DC=com"},
			wantError: "must match",
		},
		{
			name: "iam_ldap_user_policy_attachment 255 policy_name passes",
			res:  resourceMinioIAMLDAPUserPolicyAttachment(),
			raw:  map[string]interface{}{"policy_name": p255, "user_dn": "CN=alice,OU=Users,DC=example,DC=com"},
		},
		{
			name:      "iam_ldap_user_policy_attachment comma policy_name fails",
			res:       resourceMinioIAMLDAPUserPolicyAttachment(),
			raw:       map[string]interface{}{"policy_name": "a,b", "user_dn": "CN=alice,OU=Users,DC=example,DC=com"},
			wantError: "must match",
		},
		{
			name: "iam_group_user_attachment accepts 100-char group_name",
			res:  resourceMinioIAMGroupUserAttachment(),
			raw:  map[string]interface{}{"group_name": g100, "user_name": "alice"},
		},
		{
			name:      "iam_group_user_attachment rejects colon in group_name",
			res:       resourceMinioIAMGroupUserAttachment(),
			raw:       map[string]interface{}{"group_name": "a:b", "user_name": "alice"},
			wantError: "group_name",
		},
		{
			name:      "iam_group_user_attachment rejects slash in group_name",
			res:       resourceMinioIAMGroupUserAttachment(),
			raw:       map[string]interface{}{"group_name": "a/b", "user_name": "alice"},
			wantError: "group_name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := tc.res.Validate(terraform.NewResourceConfigRaw(tc.raw))
			if tc.wantError == "" {
				if diags.HasError() {
					t.Fatalf("unexpected validation errors: %v", diags)
				}
				return
			}
			if !diags.HasError() {
				t.Fatalf("expected validation error containing %q, got none", tc.wantError)
			}
			var parts []string
			for _, d := range diags {
				parts = append(parts, d.Summary, d.Detail)
			}
			joined := strings.Join(parts, " ")
			if !strings.Contains(joined, tc.wantError) {
				t.Fatalf("diagnostics = %q, want substring %q", joined, tc.wantError)
			}
		})
	}
}
