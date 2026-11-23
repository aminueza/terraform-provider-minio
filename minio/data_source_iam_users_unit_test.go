package minio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func iamUsersStubAdmin(t *testing.T, handler http.HandlerFunc) *madmin.AdminClient {
	t.Helper()
	srv := httptest.NewServer(handler)
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

func iamUsersEncryptedHandler(t *testing.T, users map[string]madmin.UserInfo) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/list-users") {
			http.NotFound(w, r)
			return
		}
		plain, err := json.Marshal(users)
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
	}
}

func toStringSlice(v interface{}) []string {
	raw, _ := v.([]interface{})
	out := make([]string, len(raw))
	for i, item := range raw {
		out[i], _ = item.(string)
	}
	return out
}

func TestDataSourceIAMUsersUnit(t *testing.T) {
	ctx := context.Background()

	t.Run("every schema field has a description", func(t *testing.T) {
		ds := dataSourceIAMUsers()
		for fieldName, field := range ds.Schema {
			if strings.TrimSpace(field.Description) == "" {
				t.Errorf("schema field %q has empty Description", fieldName)
			}
		}
		elemRes, ok := ds.Schema["users"].Elem.(*schema.Resource)
		if !ok {
			t.Fatalf("expected *schema.Resource elem for users, got %T", ds.Schema["users"].Elem)
		}
		for fieldName, field := range elemRes.Schema {
			if strings.TrimSpace(field.Description) == "" {
				t.Errorf("nested users schema field %q has empty Description", fieldName)
			}
		}
	})

	t.Run("populates policy_names, member_of_groups, sorts by name, and filters status", func(t *testing.T) {
		client := iamUsersStubAdmin(t, iamUsersEncryptedHandler(t, map[string]madmin.UserInfo{
			"team-charlie": {
				Status:     madmin.AccountEnabled,
				PolicyName: "a,,b",
				MemberOf:   []string{"grp-1"},
			},
			"team-alpha": {
				Status:     madmin.AccountEnabled,
				PolicyName: " a, b ",
				MemberOf:   []string{"devs", "ops"},
			},
			"team-bravo": {
				Status:     madmin.AccountEnabled,
				PolicyName: "",
			},
			"team-disabled": {
				Status:     madmin.AccountDisabled,
				PolicyName: "readonly",
			},
			"other-user": {
				Status:     madmin.AccountEnabled,
				PolicyName: "a,b",
			},
		}))

		ds := dataSourceIAMUsers()

		// enabled + name_prefix="team-"
		d := ds.TestResourceData()
		if err := d.Set("name_prefix", "team-"); err != nil {
			t.Fatal(err)
		}
		if err := d.Set("status", "enabled"); err != nil {
			t.Fatal(err)
		}
		if diags := ds.ReadContext(ctx, d, &S3MinioClient{S3Admin: client}); diags.HasError() {
			t.Fatalf("ReadContext(enabled) error: %v", diags)
		}

		users := d.Get("users").([]interface{})
		if len(users) != 3 {
			t.Fatalf("expected 3 enabled team- users, got %d", len(users))
		}
		u0 := users[0].(map[string]interface{})
		u1 := users[1].(map[string]interface{})
		u2 := users[2].(map[string]interface{})

		gotOrder := []string{u0["name"].(string), u1["name"].(string), u2["name"].(string)}
		if !slices.Equal(gotOrder, []string{"team-alpha", "team-bravo", "team-charlie"}) {
			t.Fatalf("expected sorted user names [team-alpha team-bravo team-charlie], got %v", gotOrder)
		}
		if got := toStringSlice(u0["policy_names"]); !slices.Equal(got, []string{"a", "b"}) {
			t.Fatalf("team-alpha policy_names = %v, want [a b]", got)
		}
		if got := toStringSlice(u0["member_of_groups"]); !slices.Equal(got, []string{"devs", "ops"}) {
			t.Fatalf("team-alpha member_of_groups = %v, want [devs ops]", got)
		}
		if got := toStringSlice(u1["policy_names"]); len(got) != 0 {
			t.Fatalf("team-bravo policy_names = %v, want empty", got)
		}
		if got := toStringSlice(u1["member_of_groups"]); len(got) != 0 {
			t.Fatalf("team-bravo member_of_groups = %v, want empty", got)
		}
		if got := toStringSlice(u2["policy_names"]); !slices.Equal(got, []string{"a", "b"}) {
			t.Fatalf("team-charlie policy_names = %v, want [a b]", got)
		}
		if got := toStringSlice(u2["member_of_groups"]); !slices.Equal(got, []string{"grp-1"}) {
			t.Fatalf("team-charlie member_of_groups = %v, want [grp-1]", got)
		}

		// status="disabled"
		dDisabled := ds.TestResourceData()
		if err := dDisabled.Set("name_prefix", "team-"); err != nil {
			t.Fatal(err)
		}
		if err := dDisabled.Set("status", "disabled"); err != nil {
			t.Fatal(err)
		}
		if diags := ds.ReadContext(ctx, dDisabled, &S3MinioClient{S3Admin: client}); diags.HasError() {
			t.Fatalf("ReadContext(disabled) error: %v", diags)
		}
		disUsers := dDisabled.Get("users").([]interface{})
		if len(disUsers) != 1 || disUsers[0].(map[string]interface{})["name"] != "team-disabled" {
			t.Fatalf("expected [team-disabled], got %v", disUsers)
		}

		// status="all"
		dAll := ds.TestResourceData()
		if err := dAll.Set("name_prefix", "team-"); err != nil {
			t.Fatal(err)
		}
		if err := dAll.Set("status", "all"); err != nil {
			t.Fatal(err)
		}
		if diags := ds.ReadContext(ctx, dAll, &S3MinioClient{S3Admin: client}); diags.HasError() {
			t.Fatalf("ReadContext(all) error: %v", diags)
		}
		if allUsers := dAll.Get("users").([]interface{}); len(allUsers) != 4 {
			t.Fatalf("expected 4 users with status=all, got %d", len(allUsers))
		}
	})

	t.Run("id is stable across repeated reads with same filters", func(t *testing.T) {
		client := iamUsersStubAdmin(t, iamUsersEncryptedHandler(t, map[string]madmin.UserInfo{
			"alice": {Status: madmin.AccountEnabled, PolicyName: "a,b"},
		}))
		ds := dataSourceIAMUsers()

		readOnce := func() string {
			d := ds.TestResourceData()
			if err := d.Set("name_prefix", "al"); err != nil {
				t.Fatal(err)
			}
			if err := d.Set("status", "enabled"); err != nil {
				t.Fatal(err)
			}
			if diags := ds.ReadContext(ctx, d, &S3MinioClient{S3Admin: client}); diags.HasError() {
				t.Fatalf("ReadContext error: %v", diags)
			}
			return d.Id()
		}

		id1 := readOnce()
		time.Sleep(1100 * time.Millisecond)
		id2 := readOnce()
		if id1 == "" || id1 != id2 {
			t.Fatalf("expected stable non-empty ID across reads, got %q then %q", id1, id2)
		}
	})

	t.Run("API error wrapped with NewResourceError", func(t *testing.T) {
		client := iamUsersStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"Code":    "InternalError",
				"Message": "boom",
			})
		})
		ds := dataSourceIAMUsers()
		d := ds.TestResourceData()
		diags := ds.ReadContext(ctx, d, &S3MinioClient{S3Admin: client})
		if !diags.HasError() {
			t.Fatal("expected error diagnostic on 500 response")
		}
		if !strings.Contains(diags[0].Summary, "[FATAL]") {
			t.Fatalf("expected NewResourceError [FATAL] diagnostic, got %q", diags[0].Summary)
		}
	})
}

func TestSplitIAMPolicyNames(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{raw: "a,b", want: []string{"a", "b"}},
		{raw: " a, b ", want: []string{"a", "b"}},
		{raw: "a,,b", want: []string{"a", "b"}},
		{raw: "", want: []string{}},
		{raw: " , , ", want: []string{}},
	}
	for _, tc := range cases {
		got := splitIAMPolicyNames(tc.raw)
		if !slices.Equal(got, tc.want) {
			t.Errorf("splitIAMPolicyNames(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}
