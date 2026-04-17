package minio

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)


func TestAccMinioS3BucketLifecycle_basic(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleBasic(bucket),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucket),
					resource.TestCheckResourceAttr(resourceName, "rule.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.id", "expire-90d"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.status", "Enabled"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.expiration.0.days", "90"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_dateExpiration(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleDateExpiration(bucket, "2030-01-01"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.expiration.0.date", "2030-01-01"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_filterSingleTag(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleFilterSingleTag(bucket),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.filter.0.tag.0.key", "retain"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.filter.0.tag.0.value", "short"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_filterPrefix(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleFilterPrefix(bucket),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.filter.0.prefix", "logs/"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.expiration.0.days", "30"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_filterAndComposite(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleFilterAnd(bucket),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.filter.0.and.0.prefix", "reports/"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.filter.0.and.0.tags.env", "prod"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.filter.0.and.0.tags.team", "platform"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_objectSizeFilter(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleObjectSize(bucket),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.filter.0.object_size_greater_than", "1048576"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_noncurrentVersion(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleNoncurrent(bucket),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.noncurrent_version_expiration.0.noncurrent_days", "30"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.noncurrent_version_expiration.0.newer_noncurrent_versions", "3"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_abortIncompleteMultipart(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleAbortMultipart(bucket),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.abort_incomplete_multipart_upload.0.days_after_initiation", "7"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.expiration.0.days", "365"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_multipleRules(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleMultipleRules(bucket),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.#", "3"),
					resource.TestCheckResourceAttr(resourceName, "rule.0.id", "expire-logs"),
					resource.TestCheckResourceAttr(resourceName, "rule.1.id", "archive-reports"),
					resource.TestCheckResourceAttr(resourceName, "rule.2.id", "cleanup-uploads"),
					resource.TestCheckResourceAttr(resourceName, "rule.1.status", "Disabled"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_updateRule(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleExpirationDays(bucket, 30),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.expiration.0.days", "30"),
				),
			},
			{
				Config: testAccS3BucketLifecycleExpirationDays(bucket, 60),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.expiration.0.days", "60"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_addAndRemoveRule(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleExpirationDays(bucket, 30),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "rule.#", "1"),
				),
			},
			{
				Config: testAccS3BucketLifecycleMultipleRules(bucket),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "rule.#", "3"),
				),
			},
			{
				Config: testAccS3BucketLifecycleExpirationDays(bucket, 30),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "rule.#", "1"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_validation_duplicateRuleID(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccS3BucketLifecycleDuplicateID(bucket),
				ExpectError: regexp.MustCompile(`duplicate rule id`),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_validation_noAction(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccS3BucketLifecycleNoAction(bucket),
				ExpectError: regexp.MustCompile(`at least one of expiration, transition`),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_validation_expirationMutuallyExclusive(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccS3BucketLifecycleExpirationConflict(bucket),
				ExpectError: regexp.MustCompile(`mutually exclusive`),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_validation_filterAndTopLevelConflict(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccS3BucketLifecycleFilterConflict(bucket),
				ExpectError: regexp.MustCompile(`filter.and is mutually exclusive`),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_validation_objectSizeBounds(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccS3BucketLifecycleSizeInvalid(bucket),
				ExpectError: regexp.MustCompile(`object_size_greater_than .* must be less than object_size_less_than`),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_expiredObjectDeleteMarker(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleDeleteMarker(bucket),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.expiration.0.expired_object_delete_marker", "true"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_transition(t *testing.T) {
	resourceName := "minio_s3_bucket_lifecycle.test"
	bucketA := acctest.RandomWithPrefix("tfacc-lc-a")
	bucketB := acctest.RandomWithPrefix("tfacc-lc-b")
	username := acctest.RandomWithPrefix("tfacc-lc-usr")
	tierName := acctest.RandomWithPrefix("COLD")

	primary := os.Getenv("MINIO_ENDPOINT")
	secondary := os.Getenv("SECOND_MINIO_ENDPOINT")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketReplicationConfigLocals(primary, secondary) +
					testAccMinioBucketTransitionConfigBucket("my_bucket_in_a", "minio", bucketA) +
					testAccMinioBucketTransitionConfigBucket("my_bucket_in_b", "secondminio", bucketB) +
					testAccMinioILMPolicyTransitionServiceAccount(username) +
					testAccMinioRemoteTierConfig(tierName, secondary) +
					testAccS3BucketLifecycleTransitionDays(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.transition.0.days", "30"),
					resource.TestCheckResourceAttrSet(resourceName, "rule.0.transition.0.storage_class"),
				),
			},
			{
				Config: testAccBucketReplicationConfigLocals(primary, secondary) +
					testAccMinioBucketTransitionConfigBucket("my_bucket_in_a", "minio", bucketA) +
					testAccMinioBucketTransitionConfigBucket("my_bucket_in_b", "secondminio", bucketB) +
					testAccMinioILMPolicyTransitionServiceAccount(username) +
					testAccMinioRemoteTierConfig(tierName, secondary) +
					testAccS3BucketLifecycleTransitionDate(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.transition.0.date", "2030-06-01"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{},
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_noncurrentVersionTransition(t *testing.T) {
	resourceName := "minio_s3_bucket_lifecycle.test"
	bucketA := acctest.RandomWithPrefix("tfacc-lc-a")
	bucketB := acctest.RandomWithPrefix("tfacc-lc-b")
	username := acctest.RandomWithPrefix("tfacc-lc-usr")
	tierName := acctest.RandomWithPrefix("COLD")

	primary := os.Getenv("MINIO_ENDPOINT")
	secondary := os.Getenv("SECOND_MINIO_ENDPOINT")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketReplicationConfigLocals(primary, secondary) +
					testAccMinioBucketTransitionConfigBucket("my_bucket_in_a", "minio", bucketA) +
					testAccMinioBucketTransitionConfigBucket("my_bucket_in_b", "secondminio", bucketB) +
					testAccMinioILMPolicyTransitionServiceAccount(username) +
					testAccMinioRemoteTierConfig(tierName, secondary) +
					testAccS3BucketLifecycleNoncurrentVersionTransition(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.0.noncurrent_version_transition.0.noncurrent_days", "30"),
					resource.TestCheckResourceAttrSet(resourceName, "rule.0.noncurrent_version_transition.0.storage_class"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketLifecycle_richConfigImport(t *testing.T) {
	bucket := acctest.RandomWithPrefix("tfacc-lc")
	resourceName := "minio_s3_bucket_lifecycle.test"

	// MinIO's GetBucketLifecycle does not round-trip abort_incomplete_multipart_upload
	// when combined with expiration in the same rule, so import cannot reconstruct it.
	// Refresh on an already-managed resource preserves the value from prior state.
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckS3BucketLifecycleDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3BucketLifecycleMultipleRules(bucket),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckS3BucketLifecycleExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "rule.#", "3"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"rule.2.abort_incomplete_multipart_upload",
					"rule.2.abort_incomplete_multipart_upload.#",
					"rule.2.abort_incomplete_multipart_upload.0.%",
					"rule.2.abort_incomplete_multipart_upload.0.days_after_initiation",
				},
			},
		},
	})
}

func TestBuildLifecycleRule_basic(t *testing.T) {
	rule := map[string]interface{}{
		"id":     "r1",
		"status": "Enabled",
		"expiration": []interface{}{map[string]interface{}{
			"days": 90,
		}},
	}
	got, err := buildLifecycleRule(rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "r1" || got.Status != "Enabled" {
		t.Errorf("unexpected id/status: %+v", got)
	}
	if got.Expiration.Days != 90 {
		t.Errorf("expected 90 days, got %d", got.Expiration.Days)
	}
}

func TestBuildLifecycleRule_filterAnd(t *testing.T) {
	rule := map[string]interface{}{
		"id": "r1",
		"expiration": []interface{}{map[string]interface{}{
			"days": 30,
		}},
		"filter": []interface{}{map[string]interface{}{
			"and": []interface{}{map[string]interface{}{
				"prefix": "logs/",
				"tags": map[string]interface{}{
					"env":  "prod",
					"team": "platform",
				},
			}},
		}},
	}
	got, err := buildLifecycleRule(rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RuleFilter.And.Prefix != "logs/" {
		t.Errorf("expected prefix logs/, got %q", got.RuleFilter.And.Prefix)
	}
	if len(got.RuleFilter.And.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(got.RuleFilter.And.Tags))
	}
}

func TestValidateLifecycleRule_noAction(t *testing.T) {
	err := validateLifecycleRule("r1", map[string]interface{}{"id": "r1"})
	if err == nil {
		t.Fatal("expected error for rule with no action")
	}
	if !strings.Contains(err.Error(), "at least one of expiration") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateLifecycleRule_expirationConflict(t *testing.T) {
	rule := map[string]interface{}{
		"id": "r1",
		"expiration": []interface{}{map[string]interface{}{
			"days": 10,
			"date": "2030-01-01",
		}},
	}
	err := validateLifecycleRule("r1", rule)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutually exclusive error, got %v", err)
	}
}

func TestValidateLifecycleRule_filterConflict(t *testing.T) {
	rule := map[string]interface{}{
		"id": "r1",
		"expiration": []interface{}{map[string]interface{}{
			"days": 10,
		}},
		"filter": []interface{}{map[string]interface{}{
			"prefix": "logs/",
			"and": []interface{}{map[string]interface{}{
				"prefix": "other/",
			}},
		}},
	}
	err := validateLifecycleRule("r1", rule)
	if err == nil || !strings.Contains(err.Error(), "filter.and is mutually exclusive") {
		t.Errorf("expected filter.and error, got %v", err)
	}
}

func TestValidateLifecycleRule_sizeBoundsInAnd(t *testing.T) {
	rule := map[string]interface{}{
		"id": "r1",
		"expiration": []interface{}{map[string]interface{}{
			"days": 10,
		}},
		"filter": []interface{}{map[string]interface{}{
			"and": []interface{}{map[string]interface{}{
				"object_size_greater_than": 2000,
				"object_size_less_than":    1000,
			}},
		}},
	}
	err := validateLifecycleRule("r1", rule)
	if err == nil || !strings.Contains(err.Error(), "must be less than") {
		t.Errorf("expected size-bounds error, got %v", err)
	}
}

func TestFlattenLifecycleRule_roundtrip(t *testing.T) {
	in := lifecycle.Rule{
		ID:     "r1",
		Status: "Enabled",
		Expiration: lifecycle.Expiration{
			Days: 30,
		},
		RuleFilter: lifecycle.Filter{
			Prefix: "logs/",
		},
	}
	out := flattenLifecycleRule(in)
	if out["id"] != "r1" {
		t.Errorf("expected id r1, got %v", out["id"])
	}
	expList, ok := out["expiration"].([]map[string]interface{})
	if !ok || len(expList) != 1 {
		t.Fatalf("expected expiration list, got %T %v", out["expiration"], out["expiration"])
	}
	if expList[0]["days"] != 30 {
		t.Errorf("expected days=30, got %v", expList[0]["days"])
	}
}

func TestIsLifecycleNotFoundErr(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("NoSuchLifecycleConfiguration: not found"), true},
		{fmt.Errorf("The lifecycle configuration does not exist"), true},
		{fmt.Errorf("some other error"), false},
	}
	for _, tc := range cases {
		if got := isLifecycleNotFoundError(tc.err); got != tc.want {
			t.Errorf("isLifecycleNotFoundError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func testAccCheckS3BucketLifecycleExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}
		c := testAccProvider.Meta().(*S3MinioClient).S3Client
		config, err := c.GetBucketLifecycle(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("lifecycle for %s not found: %w", rs.Primary.ID, err)
		}
		if len(config.Rules) == 0 {
			return fmt.Errorf("lifecycle for %s has no rules", rs.Primary.ID)
		}
		return nil
	}
}

func testAccCheckS3BucketLifecycleDestroy(s *terraform.State) error {
	c := testAccProvider.Meta().(*S3MinioClient).S3Client
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_bucket_lifecycle" {
			continue
		}
		config, err := c.GetBucketLifecycle(context.Background(), rs.Primary.ID)
		if err != nil {
			if isLifecycleNotFoundError(err) {
				return nil
			}
			continue
		}
		if config != nil && len(config.Rules) > 0 {
			return fmt.Errorf("lifecycle rules still present on %s after destroy", rs.Primary.ID)
		}
	}
	return nil
}

func testAccS3BucketLifecycleBasic(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id     = "expire-90d"
    status = "Enabled"
    expiration {
      days = 90
    }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleFilterPrefix(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "expire-logs"
    filter { prefix = "logs/" }
    expiration { days = 30 }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleFilterAnd(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "archive-reports"
    filter {
      and {
        prefix = "reports/"
        tags = {
          env  = "prod"
          team = "platform"
        }
      }
    }
    expiration { days = 180 }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleObjectSize(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "expire-large"
    filter { object_size_greater_than = 1048576 }
    expiration { days = 60 }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleNoncurrent(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_versioning" "v" {
  bucket = minio_s3_bucket.bucket.bucket
  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket_versioning.v.bucket
  rule {
    id = "expire-old-versions"
    noncurrent_version_expiration {
      noncurrent_days           = 30
      newer_noncurrent_versions = 3
    }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleAbortMultipart(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "cleanup-and-expire"
    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
    expiration { days = 365 }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleMultipleRules(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket

  rule {
    id = "expire-logs"
    filter { prefix = "logs/" }
    expiration { days = 30 }
  }

  rule {
    id     = "archive-reports"
    status = "Disabled"
    filter { prefix = "reports/" }
    expiration { days = 180 }
  }

  rule {
    id = "cleanup-uploads"
    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
    expiration { days = 365 }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleExpirationDays(bucket string, days int) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "expire"
    expiration { days = %[2]d }
  }
}
`, bucket, days)
}

func testAccS3BucketLifecycleDeleteMarker(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_versioning" "v" {
  bucket = minio_s3_bucket.bucket.bucket
  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket_versioning.v.bucket
  rule {
    id = "remove-delete-markers"
    expiration {
      expired_object_delete_marker = true
    }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleDuplicateID(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "same"
    expiration { days = 30 }
  }
  rule {
    id = "same"
    expiration { days = 60 }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleNoAction(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "r1"
  }
}
`, bucket)
}

func testAccS3BucketLifecycleExpirationConflict(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "r1"
    expiration {
      days = 30
      date = "2030-01-01"
    }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleFilterConflict(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "r1"
    expiration { days = 30 }
    filter {
      prefix = "logs/"
      and {
        prefix = "reports/"
      }
    }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleDateExpiration(bucket, date string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "expire-on-date"
    expiration {
      date = %[2]q
    }
  }
}
`, bucket, date)
}

func testAccS3BucketLifecycleFilterSingleTag(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "expire-tagged"
    filter {
      tag {
        key   = "retain"
        value = "short"
      }
    }
    expiration {
      days = 14
    }
  }
}
`, bucket)
}

func testAccS3BucketLifecycleTransitionDays() string {
	return `
resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.my_bucket_in_a.bucket
  rule {
    id = "transition-days"
    transition {
      days          = 30
      storage_class = minio_ilm_tier.remote_tier.name
    }
  }
}
`
}

func testAccS3BucketLifecycleTransitionDate() string {
	return `
resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.my_bucket_in_a.bucket
  rule {
    id = "transition-date"
    transition {
      date          = "2030-06-01"
      storage_class = minio_ilm_tier.remote_tier.name
    }
  }
}
`
}

func testAccS3BucketLifecycleNoncurrentVersionTransition() string {
	return `
resource "minio_s3_bucket_versioning" "test" {
  bucket = minio_s3_bucket.my_bucket_in_a.bucket
  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket_versioning.test.bucket
  rule {
    id = "archive-noncurrent"
    noncurrent_version_transition {
      noncurrent_days = 30
      storage_class   = minio_ilm_tier.remote_tier.name
    }
  }
}
`
}

func testAccS3BucketLifecycleSizeInvalid(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl    = "public"
}

resource "minio_s3_bucket_lifecycle" "test" {
  bucket = minio_s3_bucket.bucket.bucket
  rule {
    id = "r1"
    expiration { days = 30 }
    filter {
      and {
        object_size_greater_than = 2000
        object_size_less_than    = 1000
      }
    }
  }
}
`, bucket)
}
