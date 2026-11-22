package minio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// newLDAPPolicyEntitiesStub answers policy-entities the way MinIO does:
// UserMappings and GroupMappings only hold the queried entities, while
// PolicyMappings lists every LDAP user and group that has the queried policy.
func newLDAPPolicyEntitiesStub(t *testing.T, users, groups map[string][]string) *madmin.AdminClient {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/idp/ldap/policy-entities") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
		q := r.URL.Query()
		var res madmin.PolicyEntitiesResult
		for _, u := range q["user"] {
			if p, ok := users[u]; ok {
				res.UserMappings = append(res.UserMappings, madmin.UserPolicyEntities{User: u, Policies: p})
			}
		}
		for _, g := range q["group"] {
			if p, ok := groups[g]; ok {
				res.GroupMappings = append(res.GroupMappings, madmin.GroupPolicyEntities{Group: g, Policies: p})
			}
		}
		for _, policy := range q["policy"] {
			entry := madmin.PolicyEntities{Policy: policy}
			for u, p := range users {
				if Contains(p, policy) {
					entry.Users = append(entry.Users, u)
				}
			}
			for g, p := range groups {
				if Contains(p, policy) {
					entry.Groups = append(entry.Groups, g)
				}
			}
			if len(entry.Users)+len(entry.Groups) > 0 {
				res.PolicyMappings = append(res.PolicyMappings, entry)
			}
		}
		plain, err := json.Marshal(res)
		if err != nil {
			t.Errorf("encoding policy entities: %v", err)
		}
		body, err := madmin.EncryptData("secretkey", plain)
		if err != nil {
			t.Errorf("encrypting policy entities: %v", err)
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	client, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}
	return client
}

func TestLDAPPolicyAttachmentReadChecksTheEntity(t *testing.T) {
	const (
		alice = "cn=alice,ou=users,dc=example,dc=com"
		bob   = "cn=bob,ou=users,dc=example,dc=com"
		carol = "cn=carol,ou=users,dc=example,dc=com"
		devs  = "cn=devs,ou=groups,dc=example,dc=com"
		ops   = "cn=ops,ou=groups,dc=example,dc=com"
	)
	client := newLDAPPolicyEntitiesStub(t,
		map[string][]string{alice: {"readonly"}, bob: {"diagnostics"}},
		map[string][]string{devs: {"readonly"}, ops: {"diagnostics"}},
	)
	meta := &S3MinioClient{S3Admin: client}
	ctx := context.Background()

	cases := []struct {
		name     string
		res      func() *terraform.InstanceState
		wantGone bool
	}{
		{
			name:     "user still attached",
			res:      func() *terraform.InstanceState { return ldapAttachmentState("user_dn", alice, "readonly") },
			wantGone: false,
		},
		{
			name:     "user detached while another user keeps the policy",
			res:      func() *terraform.InstanceState { return ldapAttachmentState("user_dn", bob, "readonly") },
			wantGone: true,
		},
		{
			name:     "user without any mapping while another user keeps the policy",
			res:      func() *terraform.InstanceState { return ldapAttachmentState("user_dn", carol, "readonly") },
			wantGone: true,
		},
		{
			name:     "group still attached",
			res:      func() *terraform.InstanceState { return ldapAttachmentState("group_dn", devs, "readonly") },
			wantGone: false,
		},
		{
			name:     "group detached while another group keeps the policy",
			res:      func() *terraform.InstanceState { return ldapAttachmentState("group_dn", ops, "readonly") },
			wantGone: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := tc.res()
			res := resourceMinioIAMLDAPUserPolicyAttachment()
			if _, ok := state.Attributes["group_dn"]; ok {
				res = resourceMinioIAMLDAPGroupPolicyAttachment()
			}

			newState, diags := res.RefreshWithoutUpgrade(ctx, state, meta)
			if diags.HasError() {
				t.Fatalf("read: %v", diags)
			}
			if gone := newState == nil || newState.ID == ""; gone != tc.wantGone {
				t.Fatalf("removed from state = %v, want %v", gone, tc.wantGone)
			}
		})
	}
}

func ldapAttachmentState(dnKey, dn, policy string) *terraform.InstanceState {
	return &terraform.InstanceState{
		ID: policy + "/" + dn,
		Attributes: map[string]string{
			"id":          policy + "/" + dn,
			dnKey:         dn,
			"policy_name": policy,
		},
	}
}
