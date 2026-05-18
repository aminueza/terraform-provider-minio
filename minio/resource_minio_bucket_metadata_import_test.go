package minio

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/minio/madmin-go/v3"
)

func TestAccMinioBucketMetadataImport_basic(t *testing.T) {
	bucketName := fmt.Sprintf("tfacc-import-%d", acctest.RandInt())
	resourceName := "minio_bucket_metadata_import.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketMetadataImportConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "imported_at"),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
				),
			},
		},
	})
}

func TestAccMinioBucketMetadataImport_invalidBase64(t *testing.T) {
	bucketName := fmt.Sprintf("tfacc-import-bad-%d", acctest.RandInt())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccMinioBucketMetadataImportInvalidBase64Config(bucketName),
				ExpectError: regexp.MustCompile(`decoding metadata`),
			},
		},
	})
}

func TestCheckBucketMetaImportErrs(t *testing.T) {
	tests := []struct {
		name      string
		bucket    string
		result    madmin.BucketMetaImportErrs
		wantCount int
		wantSev   []diag.Severity
		wantText  []string
	}{
		{
			name:      "empty result returns no diagnostics",
			bucket:    "b",
			result:    madmin.BucketMetaImportErrs{},
			wantCount: 0,
		},
		{
			name:   "exact match with no per-field errors returns no diagnostics",
			bucket: "b",
			result: madmin.BucketMetaImportErrs{
				Buckets: map[string]madmin.BucketStatus{
					"b": {},
				},
			},
			wantCount: 0,
		},
		{
			name:   "exact match with top-level error returns one error",
			bucket: "b",
			result: madmin.BucketMetaImportErrs{
				Buckets: map[string]madmin.BucketStatus{
					"b": {Err: "boom"},
				},
			},
			wantCount: 1,
			wantSev:   []diag.Severity{diag.Error},
			wantText:  []string{"boom"},
		},
		{
			name:   "exact match with per-field errors aggregates a single warning",
			bucket: "b",
			result: madmin.BucketMetaImportErrs{
				Buckets: map[string]madmin.BucketStatus{
					"b": {
						Policy:  madmin.MetaStatus{IsSet: true, Err: "policy failed"},
						Tagging: madmin.MetaStatus{IsSet: true, Err: "tagging failed"},
					},
				},
			},
			wantCount: 1,
			wantSev:   []diag.Severity{diag.Warning},
			wantText:  []string{"policy failed", "tagging failed"},
		},
		{
			name:   "key mismatch with single entry falls back to that entry",
			bucket: "target",
			result: madmin.BucketMetaImportErrs{
				Buckets: map[string]madmin.BucketStatus{
					"source-in-zip": {Err: "source side err"},
				},
			},
			wantCount: 1,
			wantSev:   []diag.Severity{diag.Error},
			wantText:  []string{"source side err"},
		},
		{
			name:   "key mismatch with multiple entries aggregates all",
			bucket: "target",
			result: madmin.BucketMetaImportErrs{
				Buckets: map[string]madmin.BucketStatus{
					"x": {Err: "x failed"},
					"y": {Err: "y failed"},
				},
			},
			wantCount: 2,
			wantSev:   []diag.Severity{diag.Error, diag.Error},
			wantText:  []string{"x failed", "y failed"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkBucketMetaImportErrs(tc.bucket, tc.result)
			if len(got) != tc.wantCount {
				t.Fatalf("expected %d diagnostics, got %d (%v)", tc.wantCount, len(got), got)
			}
			for _, want := range tc.wantText {
				found := false
				for _, d := range got {
					if regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(d.Detail + d.Summary) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected diagnostics to contain %q; got %+v", want, got)
				}
			}
		})
	}
}

func testAccMinioBucketMetadataImportConfig(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "source" {
  bucket = "%s-source"
}

data "minio_bucket_metadata_export" "source" {
  bucket = minio_s3_bucket.source.bucket
}

resource "minio_s3_bucket" "target" {
  bucket = "%s"
}

resource "minio_bucket_metadata_import" "test" {
  bucket   = minio_s3_bucket.target.bucket
  metadata = data.minio_bucket_metadata_export.source.metadata
}
`, bucketName, bucketName)
}

func testAccMinioBucketMetadataImportInvalidBase64Config(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "target" {
  bucket = "%s"
}

resource "minio_bucket_metadata_import" "test" {
  bucket   = minio_s3_bucket.target.bucket
  metadata = "this is not base64!!!"
}
`, bucketName)
}
