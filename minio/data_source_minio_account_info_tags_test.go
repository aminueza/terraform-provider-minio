package minio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
)

// accountInfoTaggedBucketJSON is trimmed from the accountinfo body MinIO
// RELEASE.2025-09-07T16-13-09Z returns when one bucket carries tags: the
// server encodes details.tags as a struct, not as a flat map, and names the
// list "Buckets" (issue #1460). The size of b2 is above 2^53 so a float round
// trip would change it.
const accountInfoTaggedBucketJSON = `{"AccountName":"svc-user","Server":{"Type":2},"Policy":{"Version":"2012-10-17"},"Buckets":[` +
	`{"name":"b1","size":100,"objects":2,"details":{"versioning":false,"tags":{"XMLName":{"Space":"","Local":"Tagging"},"TagSet":{}},"quota":{"quota":0}}},` +
	`{"name":"b2","size":9007199254740993,"objects":5,"details":{"versioning":true,"tags":null}}]}`

func newAccountInfoStubAdmin(t *testing.T, body string, transport http.RoundTripper) *madmin.AdminClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/accountinfo") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}
	return admin
}

func TestAccountInfoTaggedBucketDecode(t *testing.T) {
	t.Run("the unwrapped client fails as in the issue", func(t *testing.T) {
		admin := newAccountInfoStubAdmin(t, accountInfoTaggedBucketJSON, http.DefaultTransport)
		_, err := admin.AccountInfo(context.Background(), madmin.AccountOpts{})
		if err == nil || !strings.Contains(err.Error(), "cannot unmarshal object") {
			t.Fatalf("err = %v, want the details.tags decode error the stub is meant to reproduce", err)
		}
	})

	t.Run("the wrapped client reads every bucket", func(t *testing.T) {
		admin := newAccountInfoStubAdmin(t, accountInfoTaggedBucketJSON, accountInfoTagsTransport{base: http.DefaultTransport})
		d := schema.TestResourceDataRaw(t, dataSourceMinioAccountInfo().Schema, map[string]interface{}{})
		if diags := dataSourceMinioAccountInfoRead(context.Background(), d, &S3MinioClient{S3Admin: admin}); diags.HasError() {
			t.Fatalf("dataSourceMinioAccountInfoRead failed: %v", diags)
		}
		if got := d.Get("bucket_count"); got != 2 {
			t.Errorf("bucket_count = %v, want 2", got)
		}
		if got := d.Get("buckets.1.size"); got != "9007199254740993" {
			t.Errorf("buckets.1.size = %v, want 9007199254740993", got)
		}
		if got := d.Get("total_size"); got != "9007199254741093" {
			t.Errorf("total_size = %v, want 9007199254741093", got)
		}
		if got := d.Get("total_objects"); got != "7" {
			t.Errorf("total_objects = %v, want 7", got)
		}
	})
}

func TestNormalizeAccountInfoTags(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantChanged bool
		want        string
	}{
		{
			name:        "struct-shaped tags become an empty object",
			body:        `{"Buckets":[{"name":"b1","details":{"tags":{"XMLName":{"Space":"","Local":"Tagging"},"TagSet":{}}}}]}`,
			wantChanged: true,
			want:        `{"Buckets":[{"details":{"tags":{}},"name":"b1"}]}`,
		},
		{
			name:        "a lowercase buckets key and a TagSet without XMLName are handled too",
			body:        `{"buckets":[{"name":"b1","details":{"tags":{"TagSet":{}}}}]}`,
			wantChanged: true,
			want:        `{"buckets":[{"details":{"tags":{}},"name":"b1"}]}`,
		},
		{
			name: "flat tags are left alone",
			body: `{"buckets":[{"name":"b1","details":{"tags":{"team":"storage"}}}]}`,
		},
		{
			name: "null tags are left alone",
			body: `{"buckets":[{"name":"b1","details":{"tags":null}}]}`,
		},
		{
			name: "a body that is not JSON is left to madmin",
			body: `<Error><Code>AccessDenied</Code></Error>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := normalizeAccountInfoTags([]byte(tc.body))
			if changed != tc.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, tc.wantChanged)
			}
			if tc.wantChanged && string(got) != tc.want {
				t.Errorf("body = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestAccountInfoTagsTransportOnlyTouchesAccountInfo(t *testing.T) {
	body := `{"buckets":[{"name":"b1","details":{"tags":{"XMLName":{},"TagSet":{}}}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/minio/admin/v3/datausageinfo", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := accountInfoTagsTransport{base: http.DefaultTransport}.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Errorf("body = %s, want the original body for a path other than accountinfo", got)
	}
}
