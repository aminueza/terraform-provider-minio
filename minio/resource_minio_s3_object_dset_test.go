package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestSetResourceFieldErrorPropagation(t *testing.T) {
	d := resourceMinioKMSKey().TestResourceData()

	if diags := setResourceField(d, "key_id", "kms-1", "kms-1"); diags != nil {
		t.Fatalf("expected nil diags for valid schema key, got %v", diags)
	}
	if got := d.Get("key_id").(string); got != "kms-1" {
		t.Fatalf("key_id = %q, want %q", got, "kms-1")
	}

	diags := setResourceField(d, "nonexistent_field", "val", "kms-1")
	if !diags.HasError() {
		t.Fatal("expected error diagnostic when setting nonexistent schema key, got nil")
	}
	msg := fmt.Sprint(diags)
	if !strings.Contains(msg, ErrorSeverityFatal) || !strings.Contains(msg, "setting nonexistent_field (kms-1)") {
		t.Fatalf("unexpected diagnostic message: %q", msg)
	}
}

func TestObjectReadAndImportChecksStatePopulation(t *testing.T) {
	client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("unexpected method %s for object read", r.Method)
		}
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Last-Modified", "Thu, 01 Jan 2026 00:00:00 GMT")
		w.Header().Set("Content-Length", "12")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Cache-Control", "max-age=300")
		w.Header().Set("Content-Disposition", `attachment; filename="hello.txt"`)
		w.Header().Set("Expires", "Fri, 01 Jan 2027 00:00:00 GMT")
		w.Header().Set("X-Amz-Meta-Env", "prod")
		w.WriteHeader(http.StatusOK)
	})
	meta := &S3MinioClient{S3Client: client}

	d := resourceMinioObject().TestResourceData()
	d.SetId("my-bucket/dir/hello.txt")
	if err := d.Set("bucket_name", "my-bucket"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("object_name", "dir/hello.txt"); err != nil {
		t.Fatal(err)
	}

	if diags := minioReadObject(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("minioReadObject failed: %v", diags)
	}
	for k, want := range map[string]string{
		"etag":                "abc123",
		"content_type":        "text/plain",
		"content_encoding":    "gzip",
		"storage_class":       "STANDARD",
		"cache_control":       "max-age=300",
		"content_disposition": `attachment; filename="hello.txt"`,
		"expires":             "2027-01-01T00:00:00Z",
	} {
		if got := d.Get(k).(string); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	gotMeta := d.Get("metadata").(map[string]interface{})
	if gotMeta["env"] != "prod" {
		t.Errorf("metadata[env] = %v, want %q", gotMeta["env"], "prod")
	}

	importData := resourceMinioObject().TestResourceData()
	importData.SetId("my-bucket/nested/key.txt")
	imported, err := minioImportObject(context.Background(), importData, meta)
	if err != nil {
		t.Fatalf("minioImportObject failed: %v", err)
	}
	if len(imported) != 1 {
		t.Fatalf("imported len = %d, want 1", len(imported))
	}
	if got := imported[0].Get("bucket_name").(string); got != "my-bucket" {
		t.Errorf("imported bucket_name = %q, want %q", got, "my-bucket")
	}
	if got := imported[0].Get("object_name").(string); got != "nested/key.txt" {
		t.Errorf("imported object_name = %q, want %q", got, "nested/key.txt")
	}
}

func TestObjectRetentionReadSetsOptionalFields(t *testing.T) {
	client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `<Retention xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Mode>GOVERNANCE</Mode><RetainUntilDate>2027-01-01T00:00:00Z</RetainUntilDate></Retention>`)
	})

	d := resourceMinioObjectRetention().TestResourceData()
	d.SetId("worm-bucket/obj.txt/v1")
	if err := d.Set("bucket", "worm-bucket"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("key", "obj.txt"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("version_id", "v1"); err != nil {
		t.Fatal(err)
	}

	if diags := minioReadObjectRetention(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
		t.Fatalf("minioReadObjectRetention failed: %v", diags)
	}
	for k, want := range map[string]string{
		"bucket":            "worm-bucket",
		"key":               "obj.txt",
		"version_id":        "v1",
		"mode":              "GOVERNANCE",
		"retain_until_date": "2027-01-01T00:00:00Z",
	} {
		if got := d.Get(k).(string); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestObjectTagsNoSuchTagSetSetsEmptyTags(t *testing.T) {
	client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchTagSet</Code><Message>The TagSet does not exist</Message></Error>`)
	})

	d := resourceMinioObjectTags().TestResourceData()
	d.SetId("my-bucket/obj.txt")
	if err := d.Set("bucket", "my-bucket"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("key", "obj.txt"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("tags", map[string]string{"old": "val"}); err != nil {
		t.Fatal(err)
	}

	if diags := minioReadObjectTags(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
		t.Fatalf("minioReadObjectTags failed: %v", diags)
	}
	if tags := d.Get("tags").(map[string]interface{}); len(tags) != 0 {
		t.Fatalf("tags = %v, want empty map", tags)
	}
}

func TestKMSKeyLifecycleSetsKeyID(t *testing.T) {
	var deleteStatus int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "create"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "status"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(madmin.KMSKeyStatus{KeyID: r.URL.Query().Get("key-id")})
		case strings.Contains(r.URL.Path, "delete"):
			if deleteStatus == http.StatusNotImplemented {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = fmt.Fprint(w, `{"Code":"NotImplemented","Message":"not implemented"}`)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected madmin KMS request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer srv.Close()

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("access", "secret", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}
	meta := &S3MinioClient{S3Admin: admin}

	d := resourceMinioKMSKey().TestResourceData()
	if err := d.Set("key_id", "tf-kms-key"); err != nil {
		t.Fatal(err)
	}
	if diags := minioCreateKMSKey(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("minioCreateKMSKey failed: %v", diags)
	}
	if got := d.Get("key_id").(string); got != "tf-kms-key" {
		t.Fatalf("key_id after create = %q, want %q", got, "tf-kms-key")
	}

	deleteStatus = http.StatusOK
	if diags := minioDeleteKMSKey(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("minioDeleteKMSKey (200) failed: %v", diags)
	}
	// Delete does not write key_id: the SDK discards the resource state
	// after a successful delete, so the local value is irrelevant.

	d2 := resourceMinioKMSKey().TestResourceData()
	d2.SetId("ext-kms-key")
	if err := d2.Set("key_id", "ext-kms-key"); err != nil {
		t.Fatal(err)
	}
	deleteStatus = http.StatusNotImplemented
	if diags := minioDeleteKMSKey(context.Background(), d2, meta); diags.HasError() {
		t.Fatalf("minioDeleteKMSKey (NotImplemented) failed: %v", diags)
	}
	if d2.Id() != "" {
		t.Fatalf("Id after external KMS delete = %q, want empty", d2.Id())
	}
}
