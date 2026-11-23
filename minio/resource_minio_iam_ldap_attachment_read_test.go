package minio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// newLDAPPolicyEntitiesStub answers policy-entities (and policy/attach) the way MinIO does:
// UserMappings and GroupMappings only hold the queried entities, while
// PolicyMappings lists every LDAP user and group that has the queried policy.
func newLDAPPolicyEntitiesStub(t *testing.T, users, groups map[string][]string) *madmin.AdminClient {
	t.Helper()

	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/idp/ldap/policy/attach") {
			plain, err := madmin.DecryptData("secretkey", r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			var req madmin.PolicyAssociationReq
			if err := json.Unmarshal(plain, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.User != "" {
				for _, p := range req.Policies {
					if !slices.Contains(users[req.User], p) {
						users[req.User] = append(users[req.User], p)
					}
				}
			}
			if req.Group != "" {
				for _, p := range req.Policies {
					if !slices.Contains(groups[req.Group], p) {
						groups[req.Group] = append(groups[req.Group], p)
					}
				}
			}
			respPlain, err := json.Marshal(madmin.PolicyAssociationResp{PoliciesAttached: req.Policies})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			body, err := madmin.EncryptData("secretkey", respPlain)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
			return
		}

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
		wantID   string
		dnKey    string
		dn       string
		policy   string
	}{
		{
			name:     "user still attached",
			res:      func() *terraform.InstanceState { return ldapAttachmentState("user_dn", alice, "readonly") },
			wantGone: false,
			wantID:   alice + "/readonly",
			dnKey:    "user_dn",
			dn:       alice,
			policy:   "readonly",
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
			wantID:   devs + "/readonly",
			dnKey:    "group_dn",
			dn:       devs,
			policy:   "readonly",
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
			if !tc.wantGone {
				if newState.ID != tc.wantID {
					t.Fatalf("refreshed ID = %q, want %q", newState.ID, tc.wantID)
				}
				cfg := terraform.NewResourceConfigRaw(map[string]interface{}{
					tc.dnKey:      tc.dn,
					"policy_name": tc.policy,
				})
				diff, err := res.Diff(ctx, newState, cfg, meta)
				if err != nil {
					t.Fatalf("diff after refresh: %v", err)
				}
				if diff != nil && !diff.Empty() {
					t.Fatalf("expected empty plan after refreshing legacy ID, got %#v", diff)
				}
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

func TestLDAPPolicyAttachmentIDAndMessages(t *testing.T) {
	const (
		userDN  = "cn=alice,ou=users,dc=example,dc=com"
		groupDN = "cn=devs,ou=groups,dc=example,dc=com"
	)
	ctx := context.Background()

	t.Run("user Create sets dn/policy ID", func(t *testing.T) {
		client := newLDAPPolicyEntitiesStub(t, map[string][]string{}, map[string][]string{})
		res := resourceMinioIAMLDAPUserPolicyAttachment()
		d := res.TestResourceData()
		if err := d.Set("user_dn", userDN); err != nil {
			t.Fatal(err)
		}
		if err := d.Set("policy_name", "readonly"); err != nil {
			t.Fatal(err)
		}

		diags := res.CreateContext(ctx, d, &S3MinioClient{S3Admin: client})
		if diags.HasError() {
			t.Fatalf("CreateContext failed: %v", diags)
		}
		want := userDN + "/readonly"
		if d.Id() != want {
			t.Fatalf("CreateContext ID = %q, want %q", d.Id(), want)
		}
	})

	t.Run("group Create sets dn/policy ID", func(t *testing.T) {
		client := newLDAPPolicyEntitiesStub(t, map[string][]string{}, map[string][]string{})
		res := resourceMinioIAMLDAPGroupPolicyAttachment()
		d := res.TestResourceData()
		if err := d.Set("group_dn", groupDN); err != nil {
			t.Fatal(err)
		}
		if err := d.Set("policy_name", "readonly"); err != nil {
			t.Fatal(err)
		}

		diags := res.CreateContext(ctx, d, &S3MinioClient{S3Admin: client})
		if diags.HasError() {
			t.Fatalf("CreateContext failed: %v", diags)
		}
		want := groupDN + "/readonly"
		if d.Id() != want {
			t.Fatalf("CreateContext ID = %q, want %q", d.Id(), want)
		}
	})

	t.Run("importers parse dn/policy and reject bad formats", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			dnKey   string
			dn      string
			isGroup bool
		}{
			{name: "user", dnKey: "user_dn", dn: userDN, isGroup: false},
			{name: "group", dnKey: "group_dn", dn: groupDN, isGroup: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				res := resourceMinioIAMLDAPUserPolicyAttachment()
				if tc.isGroup {
					res = resourceMinioIAMLDAPGroupPolicyAttachment()
				}

				for _, policy := range []string{"readonly", "team/readonly"} {
					d := res.TestResourceData()
					importID := tc.dn + "/" + policy
					d.SetId(importID)
					imported, err := res.Importer.StateContext(ctx, d, &S3MinioClient{})
					if err != nil {
						t.Fatalf("Importer(%q) failed: %v", importID, err)
					}
					if len(imported) != 1 {
						t.Fatalf("expected 1 imported resource, got %d", len(imported))
					}
					if imported[0].Id() != importID {
						t.Fatalf("imported ID = %q, want %q", imported[0].Id(), importID)
					}
					if got := imported[0].Get(tc.dnKey).(string); got != tc.dn {
						t.Fatalf("imported %s = %q, want %q", tc.dnKey, got, tc.dn)
					}
					if got := imported[0].Get("policy_name").(string); got != policy {
						t.Fatalf("imported policy_name = %q, want %q", got, policy)
					}
				}

				for _, badID := range []string{"", "readonly", tc.dn + "/", "/readonly"} {
					d := res.TestResourceData()
					d.SetId(badID)
					_, err := res.Importer.StateContext(ctx, d, &S3MinioClient{})
					if err == nil {
						t.Fatalf("expected error for invalid import ID %q", badID)
					}
					if !strings.Contains(err.Error(), "<dn>/<policy-name>") {
						t.Fatalf("expected error for %q to name <dn>/<policy-name>, got %q", badID, err.Error())
					}
					if tc.isGroup && strings.Contains(strings.ToLower(err.Error()), "user") {
						t.Fatalf("group importer error unexpectedly mentioned user: %q", err.Error())
					}
				}
			})
		}
	})

	t.Run("group duplicate mapping error does not mention user", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res := madmin.PolicyEntitiesResult{
				GroupMappings: []madmin.GroupPolicyEntities{
					{Group: groupDN, Policies: []string{"readonly"}},
					{Group: groupDN, Policies: []string{"writeonly"}},
				},
			}
			plain, err := json.Marshal(res)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			body, err := madmin.EncryptData("secretkey", plain)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		client, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
			Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
			Secure: false,
		})
		if err != nil {
			t.Fatal(err)
		}

		_, diags := minioReadLDAPGroupPolicies(ctx, client, groupDN)
		if !diags.HasError() {
			t.Fatal("expected error for duplicate group mappings")
		}
		for _, diag := range diags {
			if strings.Contains(strings.ToLower(diag.Summary), "user") || strings.Contains(strings.ToLower(diag.Detail), "user") {
				t.Fatalf("group read diagnostic unexpectedly mentioned user: summary=%q detail=%q", diag.Summary, diag.Detail)
			}
		}
	})
}
