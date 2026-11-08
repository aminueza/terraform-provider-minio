package minio

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func sseErrorHandler(status int, code string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>error</Message></Error>`, code)
	}
}

func TestBucketServerSideEncryptionReadErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   string
		gone   bool
	}{
		{name: "encryption removed", status: http.StatusNotFound, code: "ServerSideEncryptionConfigurationNotFoundError", gone: true},
		{name: "bucket deleted", status: http.StatusNotFound, code: "NoSuchBucket", gone: true},
		{name: "access denied", status: http.StatusForbidden, code: "AccessDenied", gone: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, sseErrorHandler(tc.status, tc.code))

			d := resourceMinioBucketServerSideEncryption().TestResourceData()
			d.SetId("test-bucket")
			if err := d.Set("bucket", "test-bucket"); err != nil {
				t.Fatal(err)
			}
			if err := d.Set("encryption_type", "AES256"); err != nil {
				t.Fatal(err)
			}

			diags := minioReadBucketServerSideEncryption(context.Background(), d, &S3MinioClient{S3Client: client})

			if tc.gone {
				if diags.HasError() {
					t.Fatalf("expected no error, got %v", diags)
				}
				if d.Id() != "" {
					t.Fatalf("expected the resource to be removed from state, id is %q", d.Id())
				}
				return
			}
			if !diags.HasError() {
				t.Fatalf("expected an error for %s", tc.code)
			}
			if d.Id() == "" {
				t.Fatalf("expected the resource to stay in state on %s", tc.code)
			}
		})
	}
}

func TestBucketServerSideEncryptionReadAfterCreateReportsMissingConfig(t *testing.T) {
	client := lifecycleStubClient(t, sseErrorHandler(http.StatusNotFound, "ServerSideEncryptionConfigurationNotFoundError"))

	d := resourceMinioBucketServerSideEncryption().TestResourceData()
	d.MarkNewResource()
	d.SetId("test-bucket")
	if err := d.Set("bucket", "test-bucket"); err != nil {
		t.Fatal(err)
	}

	diags := minioReadBucketServerSideEncryption(context.Background(), d, &S3MinioClient{S3Client: client})

	if !diags.HasError() {
		t.Fatalf("expected an error when the configuration is missing right after it was written")
	}
}

func TestBucketServerSideEncryptionDeleteWhenGone(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		code    string
		wantErr bool
	}{
		{name: "encryption already removed", status: http.StatusNotFound, code: "ServerSideEncryptionConfigurationNotFoundError"},
		{name: "bucket deleted", status: http.StatusNotFound, code: "NoSuchBucket"},
		{name: "access denied", status: http.StatusForbidden, code: "AccessDenied", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, sseErrorHandler(tc.status, tc.code))

			d := resourceMinioBucketServerSideEncryption().TestResourceData()
			d.SetId("test-bucket")
			if err := d.Set("bucket", "test-bucket"); err != nil {
				t.Fatal(err)
			}

			diags := minioDeleteBucketServerSideEncryption(context.Background(), d, &S3MinioClient{S3Client: client})

			if diags.HasError() != tc.wantErr {
				t.Fatalf("wantErr %v, got %v", tc.wantErr, diags)
			}
		})
	}
}
