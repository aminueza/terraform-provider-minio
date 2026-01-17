package minio

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

func TestAccILMPolicy_basic(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	name := fmt.Sprintf("test-ilm-rule-%d", acctest.RandInt())
	resourceName := "minio_ilm_policy.rule"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMPolicyConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					resource.TestCheckResourceAttr(resourceName, "bucket", name),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
		},
	})
}

func testAccMinioILMPolicyConfigAbortOnly(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
    bucket = "%s"
    acl    = "public"
}

resource "minio_ilm_policy" "abort_only" {
    bucket = minio_s3_bucket.bucket.bucket
    rule {
        id = "abort-only-rule"
        abort_incomplete_multipart_upload {
            days_after_initiation = "7d"
        }
    }
}
`, randInt)
}

func TestAccILMPolicy_abortOnlyNoExpiration(t *testing.T) {
	name := fmt.Sprintf("test-ilm-abort-only-%d", acctest.RandInt())
	resourceName := "minio_ilm_policy.abort_only"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMPolicyConfigAbortOnly(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.abort_incomplete_multipart_upload.0.days_after_initiation", "7d"),
				),
			},
		},
	})
}

func TestAccILMPolicy_deleteMarkerDays(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	name := fmt.Sprintf("test-ilm-rule2-%d", acctest.RandInt())
	resourceName := "minio_ilm_policy.rule2"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMPolicyConfigDeleteMarker(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
			{
				Config: testAccMinioILMPolicyConfigDays(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
		},
	})
}

func TestAccILMPolicy_filterTags(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	name := fmt.Sprintf("test-ilm-rule3-%d", acctest.RandInt())
	resourceName := "minio_ilm_policy.rule3"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMPolicyFilterWithPrefix(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
			{
				Config: testAccMinioILMPolicyFilterWithPrefixAndTags(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
		},
	})
}

func TestAccILMPolicy_expireNoncurrentVersion(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	name := fmt.Sprintf("test-ilm-rule4-%d", acctest.RandInt())
	resourceName := "minio_ilm_policy.rule4"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMPolicyExpireNoncurrentVersion(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
					resource.TestCheckResourceAttr(
						resourceName, "rule.0.expiration", ""),
					resource.TestCheckResourceAttr(
						resourceName, "rule.0.noncurrent_expiration.0.days", "5d"),
				),
			},
		},
	})
}

func TestAccILMPolicy_transition(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	resourceName := "minio_ilm_policy.rule_transition"

	bucketName := acctest.RandomWithPrefix("tf-acc-test-a")
	secondBucketName := acctest.RandomWithPrefix("tf-acc-test-b")
	username := acctest.RandomWithPrefix("tf-acc-usr")

	primaryMinioEndpoint := os.Getenv("MINIO_ENDPOINT")
	secondaryMinioEndpoint := os.Getenv("SECOND_MINIO_ENDPOINT")

	remoteTierName := acctest.RandomWithPrefix("COLD")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint) +
					testAccMinioBucketTransitionConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccMinioBucketTransitionConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccMinioILMPolicyTransitionServiceAccount(username) +
					testAccMinioRemoteTierConfig(remoteTierName, secondaryMinioEndpoint) +
					testAccMinioILMPolicyTransitionConfig(),

				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.my_bucket_in_a"),
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
					resource.TestCheckResourceAttr(
						resourceName, "rule.0.transition.0.days", "1d"),
				),
			},
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint) +
					testAccMinioBucketTransitionConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccMinioBucketTransitionConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccMinioILMPolicyTransitionServiceAccount(username) +
					testAccMinioRemoteTierConfig(remoteTierName, secondaryMinioEndpoint) +
					testAccMinioILMPolicyTransitionDateConfig(),

				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.my_bucket_in_a"),
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
					resource.TestCheckResourceAttr(
						resourceName, "rule.0.transition.0.date", "2024-06-06"),
				),
			},
		},
	})
}

func TestAccILMPolicy_ruleStatus(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	name := fmt.Sprintf("test-ilm-rule-status-%d", acctest.RandInt())
	resourceName := "minio_ilm_policy.rule_status"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				// Test default status (should be "Enabled")
				Config: testAccMinioILMPolicyConfigDefaultStatus(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					resource.TestCheckResourceAttr(resourceName, "rule.0.status", "Enabled"),
				),
			},
			{
				// Test explicitly setting status to "Disabled"
				Config: testAccMinioILMPolicyConfigDisabledStatus(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					resource.TestCheckResourceAttr(resourceName, "rule.0.status", "Disabled"),
				),
			},
			{
				// Test updating back to "Enabled"
				Config: testAccMinioILMPolicyConfigEnabledStatus(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					resource.TestCheckResourceAttr(resourceName, "rule.0.status", "Enabled"),
				),
			},
		},
	})
}

func TestAccILMPolicy_abortIncompleteMultipartUpload(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	name := fmt.Sprintf("test-ilm-abort-mpu-%d", acctest.RandInt())
	resourceName := "minio_ilm_policy.abort_mpu"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMPolicyConfigAbortIncompleteMultipartUpload(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					resource.TestCheckResourceAttr(resourceName, "rule.0.abort_incomplete_multipart_upload.0.days_after_initiation", "7d"),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
		},
	})
}

func testAccCheckMinioLifecycleConfigurationValid(config *lifecycle.Configuration) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if config.Empty() || len(config.Rules) == 0 {
			return fmt.Errorf("lifecycle configuration is empty")
		}
		return nil
	}
}

func testAccCheckMinioILMPolicyExists(n string, config *lifecycle.Configuration) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		bucketLifecycle, _ := minioC.GetBucketLifecycle(context.Background(), rs.Primary.ID)
		if bucketLifecycle == nil {
			return fmt.Errorf("bucket lifecycle not found")
		}
		*config = *bucketLifecycle

		return nil
	}
}

func testAccMinioILMPolicyConfig(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "rule" {
  bucket = "${minio_s3_bucket.bucket.id}"
  rule {
	id = "asdf"
	expiration = "2022-01-01"
	filter = "temp/"
  }
}
`, randInt)
}

func testAccMinioILMPolicyConfigDays(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket2" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "rule2" {
  bucket = "${minio_s3_bucket.bucket2.id}"
  rule {
	id = "asdf"
	expiration = "5d"
	filter = "temp/"
  }
}
`, randInt)
}

func testAccMinioILMPolicyConfigDeleteMarker(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket2" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "rule2" {
  bucket = "${minio_s3_bucket.bucket2.id}"
  rule {
	id = "asdf"
	expiration = "DeleteMarker"
  }
}
`, randInt)
}

func testAccMinioILMPolicyFilterWithPrefix(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket3" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "rule3" {
  bucket = "${minio_s3_bucket.bucket3.id}"
  rule {
	id = "withPrefix"
	expiration = "5d"
	filter = "temp/"
  }
}
`, randInt)
}

func testAccMinioILMPolicyFilterWithPrefixAndTags(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket3" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "rule3" {
  bucket = "${minio_s3_bucket.bucket3.id}"
  rule {
	id = "withPrefixAndTags"
	expiration = "5d"
	filter = "temp/"
	tags = {
		key1 = "value1"
		key2 = "value2"
	}
  }
}
`, randInt)
}

func testAccMinioILMPolicyExpireNoncurrentVersion(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket4" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "rule4" {
  bucket = "${minio_s3_bucket.bucket4.id}"
  rule {
	id = "expireNoncurrentVersion"
	noncurrent_expiration {
	  days = "5d"
	}
  }
}
`, randInt)
}

func testAccMinioRemoteTierConfig(remoteTier, endpoint string) string {
	return fmt.Sprintf(`
resource "minio_ilm_tier" "remote_tier"{
	name = "%s"
	type = "minio"
	endpoint = "http://%s"
	bucket = "${minio_s3_bucket.my_bucket_in_b.bucket}"
	minio_config {
		access_key = "${minio_iam_service_account.remote_storage.access_key}"
		secret_key = "${minio_iam_service_account.remote_storage.secret_key}"
	}
}
`, remoteTier, endpoint)
}

func testAccMinioILMPolicyTransitionConfig() string {
	return `
resource "minio_ilm_policy" "rule_transition" {
  bucket = "${minio_s3_bucket.my_bucket_in_a.bucket}"
  rule {
	id = "asdf"
	transition {
	  days = "1d"
	  storage_class = "${minio_ilm_tier.remote_tier.name}"
	}
  }
}
`
}

func testAccMinioILMPolicyTransitionDateConfig() string {
	return `
resource "minio_ilm_policy" "rule_transition" {
  bucket = "${minio_s3_bucket.my_bucket_in_a.bucket}"
  rule {
	id = "asdf"
	transition {
	  date = "2024-06-06"
	  storage_class = "${minio_ilm_tier.remote_tier.name}"
	}
  }
}
`
}

func testAccMinioILMPolicyTransitionServiceAccount(username string) (varBlock string) {
	return fmt.Sprintf(`
resource "minio_iam_user" "remote_storage" {
  provider = "secondminio"
  name = %q
  force_destroy = true
} 

resource "minio_iam_user_policy_attachment" "remote_storage" {
  provider = "secondminio"
  user_name   = "${minio_iam_user.remote_storage.name}"
  policy_name = "consoleAdmin"
}

resource "minio_iam_service_account" "remote_storage" {
  provider = "secondminio"
  target_user = "${minio_iam_user.remote_storage.name}"

  depends_on = [
    minio_iam_user_policy_attachment.remote_storage,
  ]
}

`, username)

}

func testAccMinioBucketTransitionConfigBucket(resourceName string, provider string, bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" %q {
  provider = %s
  bucket = %q
}`, resourceName, provider, bucketName)
}

func testAccMinioILMPolicyConfigDefaultStatus(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
	bucket = "%s"
	acl    = "public"
}

resource "minio_ilm_policy" "rule_status" {
	bucket = minio_s3_bucket.bucket.bucket
	rule {
		id         = "rule-default-status"
		expiration = "7d"
	}
}
`, randInt)
}

func testAccMinioILMPolicyConfigDisabledStatus(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
	bucket = "%s"
	acl    = "public"
}

resource "minio_ilm_policy" "rule_status" {
	bucket = minio_s3_bucket.bucket.bucket
	rule {
		id         = "rule-disabled-status"
		status     = "Disabled"
		expiration = "7d"
	}
}
`, randInt)
}

func testAccMinioILMPolicyConfigEnabledStatus(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
	bucket = "%s"
	acl    = "public"
}

resource "minio_ilm_policy" "rule_status" {
	bucket = minio_s3_bucket.bucket.bucket
	rule {
		id         = "rule-enabled-status"
		status     = "Enabled"
		expiration = "7d"
	}
}
`, randInt)
}

func testAccMinioILMPolicyConfigAbortIncompleteMultipartUpload(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
    bucket = "%s"
    acl    = "public"
}

resource "minio_ilm_policy" "abort_mpu" {
    bucket = minio_s3_bucket.bucket.bucket
    rule {
        id = "abort-mpu-rule"
        # Include a supported action so that MinIO accepts the lifecycle configuration
        expiration = "30d"
        abort_incomplete_multipart_upload {
            days_after_initiation = "7d"
        }
    }
}
`, randInt)
}

func TestAccILMPolicy_expiredObjectDeleteMarker(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	name := fmt.Sprintf("test-ilm-expired-dm-%d", acctest.RandInt())
	resourceName := "minio_ilm_policy.expired_dm"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMPolicyConfigExpiredObjectDeleteMarker(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
					resource.TestCheckResourceAttr(resourceName, "rule.0.expired_object_delete_marker", "true"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.noncurrent_expiration.0.days", "30d"),
				),
			},
		},
	})
}

func testAccMinioILMPolicyConfigExpiredObjectDeleteMarker(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "expired_dm" {
  bucket = "${minio_s3_bucket.bucket.id}"
  rule {
	id = "expire-dm"
	expired_object_delete_marker = true
	noncurrent_expiration {
	  days = "30d"
	}
  }
}
`, randInt)
}
