package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func groupPolicyAdminStub(t *testing.T, infoStatus int, infoBody string, removed *bool) *madmin.AdminClient {
	return groupPolicyAdminStubWithPolicies(t, infoStatus, infoBody, []string{"test-policy"}, removed)
}

func groupPolicyAdminStubWithPolicies(t *testing.T, infoStatus int, infoBody string, attached []string, removed *bool) *madmin.AdminClient {
	t.Helper()
	var mu sync.Mutex
	groupPolicies := append([]string(nil), attached...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info-canned-policy"):
			w.WriteHeader(infoStatus)
			_, _ = fmt.Fprint(w, infoBody)
		case strings.HasSuffix(r.URL.Path, "/remove-canned-policy"):
			*removed = true
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/group"):
			groupName := r.URL.Query().Get("group")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"name":%q,"status":"enabled","members":[],"policy":%q}`, groupName, strings.Join(groupPolicies, ","))
		case strings.HasSuffix(r.URL.Path, "/idp/builtin/policy/detach"):
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("reading detach body: %v", err)
			}
			dec, err := madmin.DecryptData("secretkey", strings.NewReader(string(raw)))
			if err != nil {
				t.Fatalf("decrypting detach body: %v", err)
			}
			var req madmin.PolicyAssociationReq
			if err := json.Unmarshal(dec, &req); err != nil {
				t.Fatalf("unmarshaling detach request: %v", err)
			}
			var remaining []string
			for _, p := range groupPolicies {
				if !Contains(req.Policies, p) {
					remaining = append(remaining, p)
				}
			}
			groupPolicies = remaining
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
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

func TestIAMGroupPolicyReadAndDeleteErrors(t *testing.T) {
	const policyBody = `{"PolicyName":"test-policy","Policy":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bucket/*"]}]}}`

	cases := []struct {
		name          string
		status        int
		body          string
		readErr       bool
		readGone      bool
		deleteErr     bool
		deleteRemoves bool
	}{
		{name: "policy exists", status: http.StatusOK, body: policyBody, deleteRemoves: true},
		{name: "policy deleted", status: http.StatusNotFound, body: `{"Code":"XMinioAdminNoSuchPolicy","Message":"The canned policy does not exist"}`, readGone: true},
		{name: "access denied", status: http.StatusForbidden, body: `{"Code":"AccessDenied","Message":"Access Denied."}`, readErr: true, deleteErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ops := []struct {
				name string
				run  func(context.Context, *schema.ResourceData, interface{}) diag.Diagnostics
			}{
				{"read", minioReadGroupPolicy},
				{"delete", minioDeleteGroupPolicy},
			}
			for _, op := range ops {
				removed := false
				client := groupPolicyAdminStub(t, tc.status, tc.body, &removed)

				d := resourceMinioIAMGroupPolicy().TestResourceData()
				d.SetId("test-group:test-policy")

				diags := op.run(context.Background(), d, &S3MinioClient{S3Admin: client})

				if op.name == "read" {
					if diags.HasError() != tc.readErr {
						t.Fatalf("read: wantErr %v, got %v", tc.readErr, diags)
					}
					if gone := d.Id() == ""; gone != tc.readGone {
						t.Fatalf("read: wantGone %v, id is %q", tc.readGone, d.Id())
					}
					continue
				}
				if diags.HasError() != tc.deleteErr {
					t.Fatalf("delete: wantErr %v, got %v", tc.deleteErr, diags)
				}
				if removed != tc.deleteRemoves {
					t.Fatalf("delete: remove-canned-policy called = %v, want %v", removed, tc.deleteRemoves)
				}
			}
		})
	}
}

func TestIAMGroupPolicyReadAfterCreateReportsMissingPolicy(t *testing.T) {
	removed := false
	client := groupPolicyAdminStub(t, http.StatusNotFound, `{"Code":"XMinioAdminNoSuchPolicy","Message":"The canned policy does not exist"}`, &removed)

	d := resourceMinioIAMGroupPolicy().TestResourceData()
	d.MarkNewResource()
	d.SetId("test-group:test-policy")

	if diags := minioReadGroupPolicy(context.Background(), d, &S3MinioClient{S3Admin: client}); !diags.HasError() {
		t.Fatal("expected an error when the policy is missing right after create")
	}
}

func TestIAMPolicyReadErrors(t *testing.T) {
	const policyBody = `{"PolicyName":"test-policy","Policy":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bucket/*"]}]}}`

	cases := []struct {
		name    string
		status  int
		body    string
		wantErr bool
		gone    bool
	}{
		{name: "policy exists", status: http.StatusOK, body: policyBody},
		{name: "policy deleted", status: http.StatusNotFound, body: `{"Code":"XMinioAdminNoSuchPolicy","Message":"The canned policy does not exist"}`, gone: true},
		{name: "access denied", status: http.StatusForbidden, body: `{"Code":"AccessDenied","Message":"Access Denied."}`, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			removed := false
			client := groupPolicyAdminStub(t, tc.status, tc.body, &removed)

			d := resourceMinioIAMPolicy().TestResourceData()
			d.SetId("test-policy")

			diags := minioReadPolicy(context.Background(), d, &S3MinioClient{S3Admin: client})
			if diags.HasError() != tc.wantErr {
				t.Fatalf("wantErr %v, got %v", tc.wantErr, diags)
			}
			if gone := d.Id() == ""; gone != tc.gone {
				t.Fatalf("wantGone %v, id is %q", tc.gone, d.Id())
			}
		})
	}
}

func TestIAMPolicyAndGroupPolicyRecoverNamePrefixOnRead(t *testing.T) {
	const policyBody = `{"PolicyName":"test-policy","Policy":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bucket/*"]}]}}`

	for _, r := range []struct {
		name string
		res  *schema.Resource
	}{
		{"minio_iam_policy", resourceMinioIAMPolicy()},
		{"minio_iam_group_policy", resourceMinioIAMGroupPolicy()},
	} {
		s := r.res.Schema["name_prefix"]
		if !s.Optional || !s.Computed {
			t.Fatalf("%s name_prefix Optional=%v Computed=%v, want true/true", r.name, s.Optional, s.Computed)
		}
	}

	cases := []struct {
		name       string
		policyName string
		wantPrefix string
	}{
		{name: "generated unique suffix", policyName: "tfacc-20261123010026647900000001", wantPrefix: "tfacc-"},
		{name: "plain name", policyName: "mypolicy", wantPrefix: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			removed := false
			client := groupPolicyAdminStubWithPolicies(t, http.StatusOK, policyBody, []string{tc.policyName}, &removed)
			meta := &S3MinioClient{S3Admin: client}

			dPolicy := resourceMinioIAMPolicy().TestResourceData()
			dPolicy.SetId(tc.policyName)
			if diags := minioReadPolicy(context.Background(), dPolicy, meta); diags.HasError() {
				t.Fatalf("minioReadPolicy error: %v", diags)
			}
			if got := dPolicy.Get("name_prefix").(string); got != tc.wantPrefix {
				t.Fatalf("minioReadPolicy(%q) name_prefix = %q, want %q", tc.policyName, got, tc.wantPrefix)
			}

			dGroup := resourceMinioIAMGroupPolicy().TestResourceData()
			dGroup.SetId("devs:" + tc.policyName)
			if diags := minioReadGroupPolicy(context.Background(), dGroup, meta); diags.HasError() {
				t.Fatalf("minioReadGroupPolicy error: %v", diags)
			}
			if got := dGroup.Get("name_prefix").(string); got != tc.wantPrefix {
				t.Fatalf("minioReadGroupPolicy(%q) name_prefix = %q, want %q", tc.policyName, got, tc.wantPrefix)
			}
		})
	}
}

func TestResourceMinioIamGroupPolicyParseID(t *testing.T) {
	invalid := []string{"developer", "developer:", ":p", ""}
	for _, id := range invalid {
		t.Run("invalid_"+id, func(t *testing.T) {
			_, _, err := resourceMinioIamGroupPolicyParseID(id)
			if err == nil {
				t.Fatalf("resourceMinioIamGroupPolicyParseID(%q) succeeded, want error", id)
			}
			want := fmt.Sprintf("unexpected format of ID (%q), expected <group-name>:<policy-name>", id)
			if err.Error() != want {
				t.Fatalf("resourceMinioIamGroupPolicyParseID(%q) error = %q, want %q", id, err.Error(), want)
			}
		})
	}

	groupName, policyName, err := resourceMinioIamGroupPolicyParseID("dev:a:b")
	if err != nil {
		t.Fatalf("resourceMinioIamGroupPolicyParseID(\"dev:a:b\") unexpected error: %v", err)
	}
	if groupName != "dev" || policyName != "a:b" {
		t.Fatalf("resourceMinioIamGroupPolicyParseID(\"dev:a:b\") = (%q, %q), want (\"dev\", \"a:b\")", groupName, policyName)
	}
}

func TestIAMGroupPolicyImportAndGroupValidation(t *testing.T) {
	s := resourceMinioIAMGroupPolicy().Schema["group"]
	if s.ValidateFunc == nil {
		t.Fatal("group schema ValidateFunc is nil, want validateMinioIamGroupName")
	}
	if _, errs := s.ValidateFunc("a:b", "group"); len(errs) == 0 {
		t.Fatal("ValidateFunc(\"a:b\") returned no errors, want rejection")
	}
	if _, errs := s.ValidateFunc("developer", "group"); len(errs) != 0 {
		t.Fatalf("ValidateFunc(\"developer\") returned errors: %v", errs)
	}

	dBad := resourceMinioIAMGroupPolicy().TestResourceData()
	dBad.SetId("developer")
	if _, err := minioImportGroupPolicy(context.Background(), dBad, nil); err == nil {
		t.Fatal("minioImportGroupPolicy(\"developer\") succeeded, want error")
	} else if want := `unexpected format of ID ("developer"), expected <group-name>:<policy-name>`; err.Error() != want {
		t.Fatalf("minioImportGroupPolicy(\"developer\") error = %q, want %q", err.Error(), want)
	}

	dGood := resourceMinioIAMGroupPolicy().TestResourceData()
	dGood.SetId("devs:mypolicy")
	res, err := minioImportGroupPolicy(context.Background(), dGood, nil)
	if err != nil {
		t.Fatalf("minioImportGroupPolicy(\"devs:mypolicy\") unexpected error: %v", err)
	}
	if len(res) != 1 || res[0].Id() != "devs:mypolicy" {
		t.Fatalf("minioImportGroupPolicy(\"devs:mypolicy\") = %v, want ID devs:mypolicy", res)
	}
}

func TestIAMGroupPolicyCreateReadDeleteLifecycle(t *testing.T) {
	const policyJSON = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bucket/*"]}]}`

	type stubState struct {
		mu              sync.Mutex
		groups          map[string][]string
		canned          map[string]string
		failAttachError bool
		addCalls        int
		attachCalls     [][]string
		detachCalls     [][]string
		removeCalls     []string
	}

	newClient := func(st *stubState) *madmin.AdminClient {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			st.mu.Lock()
			defer st.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/group"):
				groupName := r.URL.Query().Get("group")
				policies, ok := st.groups[groupName]
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `{"Code":"XMinioAdminNoSuchGroup","Message":"The specified group does not exist."}`)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `{"name":%q,"status":"enabled","members":[],"policy":%q}`, groupName, strings.Join(policies, ","))
			case strings.HasSuffix(r.URL.Path, "/add-canned-policy"):
				st.addCalls++
				name := r.URL.Query().Get("name")
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("reading canned policy body: %v", err)
				}
				st.canned[name] = string(body)
				w.WriteHeader(http.StatusOK)
			case strings.HasSuffix(r.URL.Path, "/info-canned-policy"):
				name := r.URL.Query().Get("name")
				body, ok := st.canned[name]
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `{"Code":"XMinioAdminNoSuchPolicy","Message":"The canned policy does not exist"}`)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `{"PolicyName":%q,"Policy":%s}`, name, body)
			case strings.HasSuffix(r.URL.Path, "/remove-canned-policy"):
				name := r.URL.Query().Get("name")
				st.removeCalls = append(st.removeCalls, name)
				delete(st.canned, name)
				w.WriteHeader(http.StatusOK)
			case strings.HasSuffix(r.URL.Path, "/idp/builtin/policy/attach"):
				if st.failAttachError {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = fmt.Fprint(w, `{"Code":"InternalError","Message":"attach failed"}`)
					return
				}
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("reading attach body: %v", err)
				}
				dec, err := madmin.DecryptData("secretkey", strings.NewReader(string(raw)))
				if err != nil {
					t.Fatalf("decrypting attach body: %v", err)
				}
				var req madmin.PolicyAssociationReq
				if err := json.Unmarshal(dec, &req); err != nil {
					t.Fatalf("unmarshaling attach req: %v", err)
				}
				st.attachCalls = append(st.attachCalls, req.Policies)
				st.groups[req.Group] = append([]string(nil), req.Policies...)
				w.WriteHeader(http.StatusNoContent)
			case strings.HasSuffix(r.URL.Path, "/idp/builtin/policy/detach"):
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("reading detach body: %v", err)
				}
				dec, err := madmin.DecryptData("secretkey", strings.NewReader(string(raw)))
				if err != nil {
					t.Fatalf("decrypting detach body: %v", err)
				}
				var req madmin.PolicyAssociationReq
				if err := json.Unmarshal(dec, &req); err != nil {
					t.Fatalf("unmarshaling detach req: %v", err)
				}
				st.detachCalls = append(st.detachCalls, req.Policies)
				var remaining []string
				for _, p := range st.groups[req.Group] {
					if !Contains(req.Policies, p) {
						remaining = append(remaining, p)
					}
				}
				st.groups[req.Group] = remaining
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			}
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

	t.Run("create preserves existing attachment and delete leaves it attached", func(t *testing.T) {
		st := &stubState{
			groups: map[string][]string{"devs": {"other-policy"}},
			canned: map[string]string{},
		}
		meta := &S3MinioClient{S3Admin: newClient(st)}

		d := resourceMinioIAMGroupPolicy().TestResourceData()
		d.Set("group", "devs")
		d.Set("name", "group-pol")
		d.Set("policy", policyJSON)
		if diags := minioCreateGroupPolicy(context.Background(), d, meta); diags.HasError() {
			t.Fatalf("create failed: %v", diags)
		}
		if d.Id() != "devs:group-pol" {
			t.Fatalf("id = %q, want devs:group-pol", d.Id())
		}
		if len(st.attachCalls) != 1 || strings.Join(st.attachCalls[0], ",") != "other-policy,group-pol" {
			t.Fatalf("attachCalls = %v, want [[other-policy group-pol]]", st.attachCalls)
		}

		// Detached outside Terraform -> Read removes from state.
		st.mu.Lock()
		st.groups["devs"] = []string{"other-policy"}
		st.mu.Unlock()
		dDetached := resourceMinioIAMGroupPolicy().TestResourceData()
		dDetached.SetId("devs:group-pol")
		if diags := minioReadGroupPolicy(context.Background(), dDetached, meta); diags.HasError() {
			t.Fatalf("read after external detach failed: %v", diags)
		}
		if dDetached.Id() != "" {
			t.Fatalf("expected empty id when detached outside Terraform, got %q", dDetached.Id())
		}

		// Re-attach and test Delete detaches only group-pol while keeping other-policy.
		st.mu.Lock()
		st.groups["devs"] = []string{"other-policy", "group-pol"}
		st.mu.Unlock()
		dDel := resourceMinioIAMGroupPolicy().TestResourceData()
		dDel.SetId("devs:group-pol")
		if diags := minioDeleteGroupPolicy(context.Background(), dDel, meta); diags.HasError() {
			t.Fatalf("delete failed: %v", diags)
		}
		if len(st.detachCalls) != 1 || strings.Join(st.detachCalls[0], ",") != "group-pol" {
			t.Fatalf("detachCalls = %v, want [[group-pol]]", st.detachCalls)
		}
		if strings.Join(st.groups["devs"], ",") != "other-policy" {
			t.Fatalf("remaining group policies = %v, want [other-policy]", st.groups["devs"])
		}
		if len(st.removeCalls) != 1 || st.removeCalls[0] != "group-pol" {
			t.Fatalf("removeCalls = %v, want [group-pol]", st.removeCalls)
		}
	})

	t.Run("create on missing group fails naming group without adding canned policy", func(t *testing.T) {
		st := &stubState{groups: map[string][]string{}, canned: map[string]string{}}
		meta := &S3MinioClient{S3Admin: newClient(st)}

		d := resourceMinioIAMGroupPolicy().TestResourceData()
		d.Set("group", "does-not-exist")
		d.Set("name", "group-pol")
		d.Set("policy", policyJSON)
		diags := minioCreateGroupPolicy(context.Background(), d, meta)
		if !diags.HasError() || !strings.Contains(diags[0].Detail+diags[0].Summary, "does-not-exist") {
			t.Fatalf("diags = %v, want error naming does-not-exist", diags)
		}
		if st.addCalls != 0 {
			t.Fatalf("addCalls = %d, want 0 when group lookup fails", st.addCalls)
		}
	})

	t.Run("create cleans up canned policy if attach fails", func(t *testing.T) {
		st := &stubState{
			groups:          map[string][]string{"devs": {}},
			canned:          map[string]string{},
			failAttachError: true,
		}
		meta := &S3MinioClient{S3Admin: newClient(st)}

		d := resourceMinioIAMGroupPolicy().TestResourceData()
		d.Set("group", "devs")
		d.Set("name", "group-pol")
		d.Set("policy", policyJSON)
		diags := minioCreateGroupPolicy(context.Background(), d, meta)
		if !diags.HasError() {
			t.Fatal("expected error when attach fails")
		}
		if len(st.removeCalls) != 1 || st.removeCalls[0] != "group-pol" {
			t.Fatalf("removeCalls = %v, want rollback [group-pol]", st.removeCalls)
		}
	})
}
