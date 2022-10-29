package minio

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccS3BucketVersioning_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketVersioningConfig(name, "Enabled", []string{"foo/", "bar/"}, true),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasVersioning(
						"minio_s3_bucket_versioning.bucket",
						S3MinioBucketVersioningConfiguration{
							Status:           "Enabled",
							ExcludedPrefixes: []string{"foo/", "bar/"},
							ExcludeFolders:   true,
						},
					),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_versioning.bucket",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccS3BucketVersioning_update(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketVersioningConfig(name, "Enabled", []string{}, false),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasVersioning(
						"minio_s3_bucket_versioning.bucket",
						S3MinioBucketVersioningConfiguration{
							Status:           "Enabled",
							ExcludedPrefixes: []string{},
							ExcludeFolders:   false,
						},
					),
				),
			},
			{
				Config: testAccBucketVersioningConfig(name, "Suspended", []string{}, false),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasVersioning(
						"minio_s3_bucket_versioning.bucket",
						S3MinioBucketVersioningConfiguration{
							Status:           "Suspended",
							ExcludedPrefixes: []string{},
							ExcludeFolders:   false,
						},
					),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_versioning.bucket",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccBucketVersioningConfig(bucketName string, status string, prefixes []string, excludeFolders bool) string {
	prefixSlice := []string{}
	for _, v := range prefixes {
		v = strconv.Quote(v)
		prefixSlice = append(prefixSlice, v)
	}

	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
}

resource "minio_s3_bucket_versioning" "bucket" {
  bucket = minio_s3_bucket.bucket.bucket
  versioning_configuration {
    status = "%s"
	excluded_prefixes = [%s]
	exclude_folders = %v
  }
}
`, bucketName, status, strings.Join(prefixSlice, ", "), excludeFolders)
}

func testAccCheckBucketHasVersioning(n string, config S3MinioBucketVersioningConfiguration) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		actualConfig, err := minioC.GetBucketVersioning(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error on GetBucketVersioning: %v", err)
		}

		if actualConfig.Status != config.Status {
			return fmt.Errorf("non-equivalent status error:\n\nexpected: %s\n\ngot: %s", config.Status, actualConfig.Status)
		}

		if len(actualConfig.ExcludedPrefixes) != len(config.ExcludedPrefixes) {
			return fmt.Errorf("non-equivalent excluded_prefixes error:\n\nexpected len: %v\n\ngot: %v", len(config.ExcludedPrefixes), len(actualConfig.ExcludedPrefixes))
		}

		for i, v := range config.ExcludedPrefixes {
			if v != actualConfig.ExcludedPrefixes[i].Prefix {
				return fmt.Errorf("non-equivalent excluded_prefixes error at index %v:\n\nexpected %s\n\ngot: %s", i, v, actualConfig.ExcludedPrefixes[i].Prefix)
			}
		}

		if actualConfig.ExcludeFolders != config.ExcludeFolders {
			return fmt.Errorf("non-equivalent exclude_folders error:\n\nexpected: %v\n\ngot: %v", config.ExcludeFolders, actualConfig.ExcludeFolders)
		}

		return nil
	}
}
