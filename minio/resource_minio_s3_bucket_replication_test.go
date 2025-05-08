package minio

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/replication"
)

const kOneWayComplexResource = `
resource "minio_s3_bucket_replication" "replication_in_all" {
  bucket     = minio_s3_bucket.my_bucket_in_a.bucket

  rule {
    enabled = false

    delete_replication = true
    delete_marker_replication = false
    existing_object_replication = false
    metadata_sync = false

    priority = 10
    prefix = "bar/"

    target {
        bucket = minio_s3_bucket.my_bucket_in_b.bucket
        host = local.second_minio_host
        region = "eu-west-1"
        secure = false
        access_key = minio_iam_service_account.replication_in_b.access_key
        secret_key = minio_iam_service_account.replication_in_b.secret_key
    }
  }

  rule {
    delete_replication = false
    delete_marker_replication = true
    existing_object_replication = true
    metadata_sync = false

    priority = 100
    prefix = "foo/"

    target {
        bucket = minio_s3_bucket.my_bucket_in_c.bucket
        host = local.third_minio_host
        region = "ap-south-1"
        secure = false
        access_key = minio_iam_service_account.replication_in_c.access_key
        secret_key = minio_iam_service_account.replication_in_c.secret_key
        health_check_period = "60s"
    }
  }

  rule {
    delete_replication = true
    delete_marker_replication = false
    existing_object_replication = true
    metadata_sync = false

    priority = 200
    tags = {
      "foo" = "bar"
    }

    target {
        bucket = minio_s3_bucket.my_bucket_in_d.bucket
        host = local.fourth_minio_host
        region = "us-west-2"
        secure = false
        bandwidth_limt = "1G"
        access_key = minio_iam_service_account.replication_in_d.access_key
        secret_key = minio_iam_service_account.replication_in_d.secret_key
    }
  }

  depends_on = [
    minio_s3_bucket_versioning.my_bucket_in_a,
    minio_s3_bucket_versioning.my_bucket_in_b,
    minio_s3_bucket_versioning.my_bucket_in_c,
    minio_s3_bucket_versioning.my_bucket_in_d,
  ]
}`

// (
//
//	resourceName,
//	minioIdentidier,
//	minioProvider,
//	ruleOneMinioIdentidier,
//	ruleOneMinioHost,
//	ruleOneMinioRegion,
//	ruleOneMinioIdentidier,
//	ruleOneMinioIdentidier,
//	ruleTwoMinioIdentidier,
//	ruleTwoMinioHost,
//	ruleTwoMinioRegion,
//	ruleTwoMinioIdentidier,
//	ruleTwoMinioIdentidier,
//	ruleThreeMinioIdentidier,
//	ruleThreeMinioHost,
//	ruleThreeMinioRegion,
//	ruleThreeMinioIdentidier,
//	ruleThreeMinioIdentidier,
//
// )
const kTemplateComplexResource = `
resource "minio_s3_bucket_replication" "%s" {
  bucket     = minio_s3_bucket.my_bucket_in_%s.bucket
  provider = %s

  rule {
    enabled = false

    delete_replication = true
    delete_marker_replication = true
    existing_object_replication = true
    metadata_sync = true

    prefix = "bar/"

    target {
        bucket = minio_s3_bucket.my_bucket_in_%s.bucket
        host = local.%s_minio_host
        region = %q
        secure = false
        access_key = minio_iam_service_account.replication_in_%s.access_key
        secret_key = minio_iam_service_account.replication_in_%s.secret_key
    }
  }

  rule {
    delete_replication = true
    delete_marker_replication = true
    existing_object_replication = true
    metadata_sync = true

    prefix = "foo/"

    target {
        bucket = minio_s3_bucket.my_bucket_in_%s.bucket
        host = local.%s_minio_host
        region = %q
        secure = false
        access_key = minio_iam_service_account.replication_in_%s.access_key
        secret_key = minio_iam_service_account.replication_in_%s.secret_key
        health_check_period = "60s"
    }
  }

  rule {
    delete_replication = true
    delete_marker_replication = false
    existing_object_replication = true
    metadata_sync = true

    tags = {
      "foo" = "bar"
    }

    target {
        bucket = minio_s3_bucket.my_bucket_in_%s.bucket
        host = local.%s_minio_host
        region = %q
        secure = false
        access_key = minio_iam_service_account.replication_in_%s.access_key
        secret_key = minio_iam_service_account.replication_in_%s.secret_key
        bandwidth_limt = "1G"
    }
  }

  depends_on = [
    minio_s3_bucket_versioning.my_bucket_in_a,
    minio_s3_bucket_versioning.my_bucket_in_b,
    minio_s3_bucket_versioning.my_bucket_in_c,
    minio_s3_bucket_versioning.my_bucket_in_d,
  ]
}
`

// Rule 1 ring is a -> b -> c -> d
// Rule 2 ring is a -> c -> d -> b
// Rule 3 ring is a -> d -> b -> c
// a -> eu-central-1
// b -> eu-west-1
// c -> ap-south-1
// d -> us-west-2
var kTwoWayComplexResource = fmt.Sprintf(kTemplateComplexResource,
	"replication_in_bcd",
	"a",
	"minio",
	// Rule 1
	"b",
	"second",
	"eu-west-1",
	"b",
	"b",
	// Rule 2
	"c",
	"third",
	"ap-south-1",
	"c",
	"c",
	// Rule 3
	"d",
	"fourth",
	"us-west-2",
	"d",
	"d",
) +
	fmt.Sprintf(kTemplateComplexResource,
		"replication_in_acd",
		"b",
		"secondminio",
		// Rule 1
		"c",
		"third",
		"ap-south-1",
		"c",
		"c",
		// Rule 2
		"d",
		"fourth",
		"us-west-2",
		"d",
		"d",
		// Rule 3
		"a",
		"primary",
		"eu-central-1",
		"a",
		"a",
	) +
	fmt.Sprintf(kTemplateComplexResource,
		"replication_in_abd",
		"c",
		"thirdminio",
		// Rule 1
		"d",
		"fourth",
		"us-west-2",
		"d",
		"d",
		// Rule 2
		"a",
		"primary",
		"eu-central-1",
		"a",
		"a",
		// Rule 3
		"b",
		"second",
		"eu-west-1",
		"b",
		"b",
	) +
	fmt.Sprintf(kTemplateComplexResource,
		"replication_in_abc",
		"d",
		"fourthminio",
		// Rule 1
		"a",
		"primary",
		"eu-central-1",
		"a",
		"a",
		// Rule 2
		"b",
		"second",
		"eu-west-1",
		"b",
		"b",
		// Rule 3
		"c",
		"third",
		"ap-south-1",
		"c",
		"c",
	)

const kOneWaySimpleResource = `
resource "minio_s3_bucket_replication" "replication_in_b" {
  bucket     = minio_s3_bucket.my_bucket_in_a.bucket

  rule {
    delete_replication = true
    delete_marker_replication = true
    existing_object_replication = true
    metadata_sync = false

    target {
        bucket = minio_s3_bucket.my_bucket_in_b.bucket
        host = local.second_minio_host
        secure = false
        bandwidth_limt = "100M"
        access_key = minio_iam_service_account.replication_in_b.access_key
        secret_key = minio_iam_service_account.replication_in_b.secret_key
    }
  }

  depends_on = [
    minio_s3_bucket_versioning.my_bucket_in_a,
    minio_s3_bucket_versioning.my_bucket_in_b
  ]
}`

const kTwoWaySimpleResource = `
resource "minio_s3_bucket_replication" "replication_in_b" {
    bucket     = minio_s3_bucket.my_bucket_in_a.bucket
    
    rule {
        priority = 100

        delete_replication = true
        delete_marker_replication = true
        existing_object_replication = true
        metadata_sync = true

        target {
            bucket = minio_s3_bucket.my_bucket_in_b.bucket
            host = local.second_minio_host
            secure = false
            region = "eu-west-1"
            syncronous = true
            bandwidth_limt = "100M"
            access_key = minio_iam_service_account.replication_in_b.access_key
            secret_key = minio_iam_service_account.replication_in_b.secret_key
        }
    }
    
    depends_on = [
        minio_s3_bucket_versioning.my_bucket_in_a,
        minio_s3_bucket_versioning.my_bucket_in_b
    ]
}

resource "minio_s3_bucket_replication" "replication_in_a" {
    bucket     = minio_s3_bucket.my_bucket_in_b.bucket
    provider = secondminio

    rule {
        priority = 100

        delete_replication = true
        delete_marker_replication = true
        existing_object_replication = true
        metadata_sync = true

        target {
            bucket = minio_s3_bucket.my_bucket_in_a.bucket
            host = local.primary_minio_host
            region = "eu-north-1"
            secure = false
            bandwidth_limt = "800M"
            health_check_period = "2m"
            access_key = minio_iam_service_account.replication_in_a.access_key
            secret_key = minio_iam_service_account.replication_in_a.secret_key
        }
    }

    depends_on = [
      minio_s3_bucket_versioning.my_bucket_in_a,
      minio_s3_bucket_versioning.my_bucket_in_b
    ]
}`

func TestAccS3BucketReplication_oneway_simple(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test-a")
	secondBucketName := acctest.RandomWithPrefix("tf-acc-test-b")
	username := acctest.RandomWithPrefix("tf-acc-usr")

	primaryMinioEndpoint := os.Getenv("MINIO_ENDPOINT")
	secondaryMinioEndpoint := os.Getenv("SECOND_MINIO_ENDPOINT")

	// Test in parallel cannot work as remote target endpoint would conflict
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint) +
					testAccBucketReplicationConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccBucketReplicationConfigPolicy(bucketName, secondBucketName) +
					testAccBucketReplicationConfigServiceAccount(username, 2) +
					kOneWaySimpleResource,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_b",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  true,
								Priority: 1,

								Prefix: "",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              false,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            secondBucketName,
									StorageClass:      "",
									Host:              secondaryMinioEndpoint,
									Path:              "/",
									Region:            "",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    100000000,
								},
							},
						},
					),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_replication.replication_in_b",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"rule.0.target.0.secret_key",
					"rule.0.priority", // This is ommited in our test case, so it gets automatically generated and thus mismatch
				},
				Config: kOneWaySimpleResource,
			},
		},
	})
}

func TestAccS3BucketReplication_oneway_simple_update(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test-a")
	secondBucketName := acctest.RandomWithPrefix("tf-acc-test-b")
	username := acctest.RandomWithPrefix("tf-acc-usr")

	primaryMinioEndpoint := os.Getenv("MINIO_ENDPOINT")
	secondaryMinioEndpoint := os.Getenv("SECOND_MINIO_ENDPOINT")

	// Test in parallel cannot work as remote target endpoint would conflict
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint) +
					testAccBucketReplicationConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccBucketReplicationConfigPolicy(bucketName, secondBucketName) +
					testAccBucketReplicationConfigServiceAccount(username, 2) +
					kOneWaySimpleResource,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_b",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  true,
								Priority: 1,

								Prefix: "",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              false,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            secondBucketName,
									StorageClass:      "",
									Host:              secondaryMinioEndpoint,
									Path:              "/",
									Region:            "",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    100000000,
								},
							},
						},
					),
				),
			},
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint) +
					testAccBucketReplicationConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccBucketReplicationConfigPolicy(bucketName, secondBucketName) +
					testAccBucketReplicationConfigServiceAccount(username, 2) + `
resource "minio_s3_bucket_replication" "replication_in_b" {
  bucket     = minio_s3_bucket.my_bucket_in_a.bucket

  rule {
    priority = 50 
    delete_replication = false
    delete_marker_replication = false
    existing_object_replication = true
    metadata_sync = false

    target {
        bucket = minio_s3_bucket.my_bucket_in_b.bucket
        host = local.second_minio_host
        secure = false
        bandwidth_limt = "150M"
        health_check_period = "5m"
        access_key = minio_iam_service_account.replication_in_b.access_key
        secret_key = minio_iam_service_account.replication_in_b.secret_key
    }
  }

  depends_on = [
    minio_s3_bucket_versioning.my_bucket_in_a,
    minio_s3_bucket_versioning.my_bucket_in_b
  ]
}`,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_b",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  true,
								Priority: 50,

								Prefix: "",
								Tags:   map[string]string{},

								DeleteReplication:         false,
								DeleteMarkerReplication:   false,
								ExistingObjectReplication: true,
								MetadataSync:              false,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            secondBucketName,
									StorageClass:      "",
									Host:              secondaryMinioEndpoint,
									Path:              "/",
									Region:            "",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Minute * 5,
									BandwidthLimit:    150000000,
								},
							},
						},
					),
				),
			},
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint) +
					testAccBucketReplicationConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccBucketReplicationConfigPolicy(bucketName, secondBucketName) +
					testAccBucketReplicationConfigServiceAccount(username, 2) +
					`
resource "minio_s3_bucket_replication" "replication_in_b" {
  bucket     = minio_s3_bucket.my_bucket_in_a.bucket

  rule {
    enabled = false

    delete_replication = false
    delete_marker_replication = false
    existing_object_replication = true
    metadata_sync = false

    target {
        bucket = minio_s3_bucket.my_bucket_in_b.bucket
        host = local.second_minio_host
        secure = false
        bandwidth_limt = "150M"
        health_check_period = "5m"
        access_key = minio_iam_service_account.replication_in_b.access_key
        secret_key = minio_iam_service_account.replication_in_b.secret_key
    }
  }

  depends_on = [
    minio_s3_bucket_versioning.my_bucket_in_a,
    minio_s3_bucket_versioning.my_bucket_in_b
  ]
}`,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_b",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  false,
								Priority: 1,

								Prefix: "",
								Tags:   map[string]string{},

								DeleteReplication:         false,
								DeleteMarkerReplication:   false,
								ExistingObjectReplication: true,
								MetadataSync:              false,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            secondBucketName,
									StorageClass:      "",
									Host:              secondaryMinioEndpoint,
									Path:              "/",
									Region:            "",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Minute * 5,
									BandwidthLimit:    150000000,
								},
							},
						},
					),
				),
			},
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint) +
					testAccBucketReplicationConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccBucketReplicationConfigPolicy(bucketName, secondBucketName) +
					testAccBucketReplicationConfigServiceAccount(username, 2) +
					kOneWaySimpleResource,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_b",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  true,
								Priority: 1,

								Prefix: "",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              false,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            secondBucketName,
									StorageClass:      "",
									Host:              secondaryMinioEndpoint,
									Path:              "/",
									Region:            "",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    100000000,
								},
							},
						},
					),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_replication.replication_in_b",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"rule.0.target.0.secret_key",
					"rule.0.priority", // This is ommited in our test case, so it gets automatically generated and thus mismatch
				},
				Config: kOneWaySimpleResource,
			},
		},
	})
}
func TestAccS3BucketReplication_oneway_complex(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test-a")
	secondBucketName := acctest.RandomWithPrefix("tf-acc-test-b")
	thirdBucketName := acctest.RandomWithPrefix("tf-acc-test-c")
	fourthBucketName := acctest.RandomWithPrefix("tf-acc-test-d")
	username := acctest.RandomWithPrefix("tf-acc-usr")

	primaryMinioEndpoint := os.Getenv("MINIO_ENDPOINT")
	secondaryMinioEndpoint := os.Getenv("SECOND_MINIO_ENDPOINT")
	thirdMinioEndpoint := os.Getenv("THIRD_MINIO_ENDPOINT")
	fourthMinioEndpoint := os.Getenv("FOURTH_MINIO_ENDPOINT")

	// Test in parallel cannot work as remote target endpoint would conflict
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint, thirdMinioEndpoint, fourthMinioEndpoint) +
					testAccBucketReplicationConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_c", "thirdminio", thirdBucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_d", "fourthminio", fourthBucketName) +
					testAccBucketReplicationConfigPolicy(bucketName, secondBucketName, thirdBucketName, fourthBucketName) +
					testAccBucketReplicationConfigServiceAccount(username, 4) +
					kOneWayComplexResource,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_all",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  false,
								Priority: 10,

								Prefix: "bar/",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   false,
								ExistingObjectReplication: false,
								MetadataSync:              false,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            secondBucketName,
									StorageClass:      "",
									Host:              secondaryMinioEndpoint,
									Path:              "/",
									Region:            "eu-west-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    0,
								},
							},
							{
								Enabled:  true,
								Priority: 100,

								Prefix: "foo/",
								Tags:   map[string]string{},

								DeleteReplication:         false,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              false,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            thirdBucketName,
									StorageClass:      "",
									Host:              thirdMinioEndpoint,
									Path:              "/",
									Region:            "ap-south-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 60,
									BandwidthLimit:    0,
								},
							},
							{
								Enabled:  true,
								Priority: 200,

								Prefix: "",
								Tags: map[string]string{
									"foo": "bar",
								},

								DeleteReplication:         true,
								DeleteMarkerReplication:   false,
								ExistingObjectReplication: true,
								MetadataSync:              false,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            fourthBucketName,
									StorageClass:      "",
									Host:              fourthMinioEndpoint,
									Path:              "/",
									Region:            "us-west-2",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    1 * humanize.BigGByte.Int64(),
								},
							},
						},
					),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_replication.replication_in_all",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"rule.0.target.0.secret_key",
					"rule.1.target.0.secret_key",
					"rule.2.target.0.secret_key",
				},
				Config: kOneWayComplexResource,
			},
		},
	})
}

func TestAccS3BucketReplication_twoway_simple(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test-a")
	secondBucketName := acctest.RandomWithPrefix("tf-acc-test-b")
	username := acctest.RandomWithPrefix("tf-acc-usr")

	primaryMinioEndpoint := os.Getenv("MINIO_ENDPOINT")
	secondaryMinioEndpoint := os.Getenv("SECOND_MINIO_ENDPOINT")

	// Test in parallel cannot work as remote target endpoint would conflict
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint) +
					testAccBucketReplicationConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccBucketReplicationConfigPolicy(bucketName, secondBucketName) +
					testAccBucketReplicationConfigServiceAccount(username, 2) +
					kTwoWaySimpleResource,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_b",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  true,
								Priority: 100,

								Prefix: "",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            secondBucketName,
									StorageClass:      "",
									Host:              secondaryMinioEndpoint,
									Region:            "eu-west-1",
									Syncronous:        true,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    100000000,
								},
							},
						},
					),
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_a",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  true,
								Priority: 100,

								Prefix: "",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            bucketName,
									StorageClass:      "",
									Host:              primaryMinioEndpoint,
									Region:            "eu-north-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 120,
									BandwidthLimit:    800000000,
								},
							},
						},
					),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_replication.replication_in_b",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"rule.0.target.0.secret_key",
				},
				Config: kTwoWaySimpleResource,
			},
			{
				ResourceName:      "minio_s3_bucket_replication.replication_in_a",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"rule.0.target.0.secret_key",
				},
				Config: kTwoWaySimpleResource,
			},
		},
	})
}
func TestAccS3BucketReplication_twoway_complex(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test-a")
	secondBucketName := acctest.RandomWithPrefix("tf-acc-test-b")
	thirdBucketName := acctest.RandomWithPrefix("tf-acc-test-c")
	fourthBucketName := acctest.RandomWithPrefix("tf-acc-test-d")
	username := acctest.RandomWithPrefix("tf-acc-usr")

	primaryMinioEndpoint := os.Getenv("MINIO_ENDPOINT")
	secondaryMinioEndpoint := os.Getenv("SECOND_MINIO_ENDPOINT")
	thirdMinioEndpoint := os.Getenv("THIRD_MINIO_ENDPOINT")
	fourthMinioEndpoint := os.Getenv("FOURTH_MINIO_ENDPOINT")

	// Test in parallel cannot work as remote target endpoint would conflict
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint, thirdMinioEndpoint, fourthMinioEndpoint) +
					testAccBucketReplicationConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_c", "thirdminio", thirdBucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_d", "fourthminio", fourthBucketName) +
					testAccBucketReplicationConfigPolicy(bucketName, secondBucketName, thirdBucketName, fourthBucketName) +
					testAccBucketReplicationConfigServiceAccount(username, 4) +
					kTwoWayComplexResource,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_bcd",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  false,
								Priority: 3,

								Prefix: "bar/",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            secondBucketName,
									StorageClass:      "",
									Host:              secondaryMinioEndpoint,
									Path:              "/",
									Region:            "eu-west-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    0,
								},
							},
							{
								Enabled:  true,
								Priority: 2,

								Prefix: "foo/",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            thirdBucketName,
									StorageClass:      "",
									Host:              thirdMinioEndpoint,
									Path:              "/",
									Region:            "ap-south-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 60,
									BandwidthLimit:    0,
								},
							},
							{
								Enabled:  true,
								Priority: 1,

								Prefix: "",
								Tags: map[string]string{
									"foo": "bar",
								},

								DeleteReplication:         true,
								DeleteMarkerReplication:   false,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            fourthBucketName,
									StorageClass:      "",
									Host:              fourthMinioEndpoint,
									Path:              "/",
									Region:            "us-west-2",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    1 * humanize.BigGByte.Int64(),
								},
							},
						},
					),
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_acd",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  false,
								Priority: 3,

								Prefix: "bar/",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            thirdBucketName,
									StorageClass:      "",
									Host:              thirdMinioEndpoint,
									Path:              "/",
									Region:            "ap-south-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    0,
								},
							},
							{
								Enabled:  true,
								Priority: 2,

								Prefix: "foo/",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            fourthBucketName,
									StorageClass:      "",
									Host:              fourthMinioEndpoint,
									Path:              "/",
									Region:            "us-west-2",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 60,
									BandwidthLimit:    0,
								},
							},
							{
								Enabled:  true,
								Priority: 1,

								Prefix: "",
								Tags: map[string]string{
									"foo": "bar",
								},

								DeleteReplication:         true,
								DeleteMarkerReplication:   false,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            bucketName,
									StorageClass:      "",
									Host:              primaryMinioEndpoint,
									Path:              "/",
									Region:            "eu-central-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    1 * humanize.BigGByte.Int64(),
								},
							},
						},
					),
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_abd",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  false,
								Priority: 3,

								Prefix: "bar/",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            fourthBucketName,
									StorageClass:      "",
									Host:              fourthMinioEndpoint,
									Path:              "/",
									Region:            "us-west-2",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    0,
								},
							},
							{
								Enabled:  true,
								Priority: 2,

								Prefix: "foo/",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            bucketName,
									StorageClass:      "",
									Host:              primaryMinioEndpoint,
									Path:              "/",
									Region:            "eu-central-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 60,
									BandwidthLimit:    0,
								},
							},
							{
								Enabled:  true,
								Priority: 1,

								Prefix: "",
								Tags: map[string]string{
									"foo": "bar",
								},

								DeleteReplication:         true,
								DeleteMarkerReplication:   false,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            secondBucketName,
									StorageClass:      "",
									Host:              secondaryMinioEndpoint,
									Path:              "/",
									Region:            "eu-west-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    1 * humanize.BigGByte.Int64(),
								},
							},
						},
					),
					testAccCheckBucketHasReplication(
						"minio_s3_bucket_replication.replication_in_abc",
						[]S3MinioBucketReplicationRule{
							{
								Enabled:  false,
								Priority: 3,

								Prefix: "bar/",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            bucketName,
									StorageClass:      "",
									Host:              primaryMinioEndpoint,
									Path:              "/",
									Region:            "eu-central-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    0,
								},
							},
							{
								Enabled:  true,
								Priority: 2,

								Prefix: "foo/",
								Tags:   map[string]string{},

								DeleteReplication:         true,
								DeleteMarkerReplication:   true,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            secondBucketName,
									StorageClass:      "",
									Host:              secondaryMinioEndpoint,
									Path:              "/",
									Region:            "eu-west-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 60,
									BandwidthLimit:    0,
								},
							},
							{
								Enabled:  true,
								Priority: 1,

								Prefix: "",
								Tags: map[string]string{
									"foo": "bar",
								},

								DeleteReplication:         true,
								DeleteMarkerReplication:   false,
								ExistingObjectReplication: true,
								MetadataSync:              true,

								Target: S3MinioBucketReplicationRuleTarget{
									Bucket:            thirdBucketName,
									StorageClass:      "",
									Host:              thirdMinioEndpoint,
									Path:              "/",
									Region:            "ap-south-1",
									Syncronous:        false,
									Secure:            false,
									PathStyle:         S3PathSyleAuto,
									HealthCheckPeriod: time.Second * 30,
									BandwidthLimit:    1 * humanize.BigGByte.Int64(),
								},
							},
						},
					),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_replication.replication_in_bcd",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"rule.0.target.0.secret_key",
					"rule.1.target.0.secret_key",
					"rule.2.target.0.secret_key",
					// Prorities are ignored in this test case, as it gets automatically generated and thus mismatch
					"rule.0.priority",
					"rule.1.priority",
					"rule.2.priority",
				},
				Config: kTwoWayComplexResource,
			},
			{
				ResourceName:      "minio_s3_bucket_replication.replication_in_acd",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"rule.0.target.0.secret_key",
					"rule.1.target.0.secret_key",
					"rule.2.target.0.secret_key",
					// Prorities are ignored in this test case, as it gets automatically generated and thus mismatch
					"rule.0.priority",
					"rule.1.priority",
					"rule.2.priority",
				},
				Config: kTwoWayComplexResource,
			},
			{
				ResourceName:      "minio_s3_bucket_replication.replication_in_abd",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"rule.0.target.0.secret_key",
					"rule.1.target.0.secret_key",
					"rule.2.target.0.secret_key",
					// Prorities are ignored in this test case, as it gets automatically generated and thus mismatch
					"rule.0.priority",
					"rule.1.priority",
					"rule.2.priority",
				},
				Config: kTwoWayComplexResource,
			},
			{
				ResourceName:      "minio_s3_bucket_replication.replication_in_abc",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"rule.0.target.0.secret_key",
					"rule.1.target.0.secret_key",
					"rule.2.target.0.secret_key",
					// Prorities are ignored in this test case, as it gets automatically generated and thus mismatch
					"rule.0.priority",
					"rule.1.priority",
					"rule.2.priority",
				},
				Config: kTwoWayComplexResource,
			},
		},
	})
}

var kMinioHostIdentifier = []string{
	"primary",
	"second",
	"third",
	"fourth",
}

var kMinioHostLetter = []string{
	"a",
	"b",
	"c",
	"d",
}

func testAccBucketReplicationConfigLocals(minioHost ...string) string {
	var varBlock string
	for i, val := range minioHost {
		varBlock = varBlock + fmt.Sprintf("	%s_minio_host = %q\n", kMinioHostIdentifier[i], val)
	}
	return fmt.Sprintf(`
locals {
  %s
}
`, varBlock)
}

func testAccBucketReplicationConfigBucket(resourceName string, provider string, bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" %q {
  provider = %s
  bucket = %q
}

resource "minio_s3_bucket_versioning" %q {
  provider = %s
  bucket     = %q

  versioning_configuration {
    status = "Enabled"
  }

  depends_on = [
    minio_s3_bucket.%s
  ]
}
`, resourceName, provider, bucketName, resourceName, provider, bucketName, resourceName)
}

func testAccBucketReplicationConfigServiceAccount(username string, count int) (varBlock string) {
	for i := 0; i < count; i++ {
		indentifier := kMinioHostIdentifier[i]
		if i == 0 {
			indentifier = "minio"
		} else {
			indentifier = indentifier + "minio"
		}
		letter := kMinioHostLetter[i]
		varBlock = varBlock + fmt.Sprintf(`
resource "minio_iam_policy" "replication_in_%s" {
  provider = %s
  name   = "ReplicationToMyBucketPolicy"
  policy = data.minio_iam_policy_document.replication_policy.json
}

resource "minio_iam_user" "replication_in_%s" {
  provider = %s
  name = %q
  force_destroy = true
} 

resource "minio_iam_user_policy_attachment" "replication_in_%s" {
  provider = %s
  user_name   = minio_iam_user.replication_in_%s.name
  policy_name = minio_iam_policy.replication_in_%s.id
}

resource "minio_iam_service_account" "replication_in_%s" {
  provider = %s
  target_user = minio_iam_user.replication_in_%s.name

  depends_on = [
    minio_iam_user_policy_attachment.replication_in_%s,
    minio_iam_policy.replication_in_%s,
  ]
}

`, letter, indentifier, letter, indentifier, username, letter, indentifier, letter, letter, letter, indentifier, letter, letter, letter)
	}
	return varBlock
}

func testAccBucketReplicationConfigPolicy(bucketArn ...string) string {
	bucketObjectArn := make([]string, len(bucketArn))
	for i, bucket := range bucketArn {
		bucketArn[i] = fmt.Sprintf("\"arn:aws:s3:::%s\"", bucket)
		bucketObjectArn[i] = fmt.Sprintf("\"arn:aws:s3:::%s/*\"", bucket)
	}
	return fmt.Sprintf(`
data "minio_iam_policy_document" "replication_policy" {
  statement {
    sid       = "ReadBuckets"
    effect    = "Allow"
    resources = ["arn:aws:s3:::*"]

    actions = [
      "s3:ListBucket",
    ]
  }

  statement {
    sid       = "EnableReplicationOnBucket"
    effect    = "Allow"
    resources = [%s]

    actions = [
      "s3:GetReplicationConfiguration",
      "s3:ListBucket",
      "s3:ListBucketMultipartUploads",
      "s3:GetBucketLocation",
      "s3:GetBucketVersioning",
      "s3:GetBucketObjectLockConfiguration",
      "s3:GetEncryptionConfiguration",
    ]
  }

  statement {
    sid       = "EnableReplicatingDataIntoBucket"
    effect    = "Allow"
    resources = [%s]

    actions = [
      "s3:GetReplicationConfiguration",
      "s3:ReplicateTags",
      "s3:AbortMultipartUpload",
      "s3:GetObject",
      "s3:GetObjectVersion",
      "s3:GetObjectVersionTagging",
      "s3:PutObject",
      "s3:PutObjectRetention",
      "s3:PutBucketObjectLockConfiguration",
      "s3:PutObjectLegalHold",
      "s3:DeleteObject",
      "s3:ReplicateObject",
      "s3:ReplicateDelete",
    ]
  }
}
`, strings.Join(bucketArn, ","), strings.Join(bucketObjectArn, ","))
}

func testAccCheckBucketHasReplication(n string, config []S3MinioBucketReplicationRule) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		var provider *S3MinioClient
		switch rs.Provider {
		case "registry.terraform.io/hashicorp/minio":
			provider = testAccProvider.Meta().(*S3MinioClient)
		case "registry.terraform.io/hashicorp/secondminio":
			provider = testAccSecondProvider.Meta().(*S3MinioClient)
		case "registry.terraform.io/hashicorp/thirdminio":
			provider = testAccThirdProvider.Meta().(*S3MinioClient)
		case "registry.terraform.io/hashicorp/fourthminio":
			provider = testAccFourthProvider.Meta().(*S3MinioClient)
		default:
			return fmt.Errorf("Provider %q unknown", rs.Provider)
		}

		minioC := provider.S3Client
		minioadm := provider.S3Admin
		actualConfig, err := minioC.GetBucketReplication(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error on GetBucketReplication: %v", err)
		}

		if len(actualConfig.Rules) != len(config) {
			return fmt.Errorf("non-equivalent status error:\n\nexpected: %d\n\ngot: %d", len(actualConfig.Rules), len(config))
		}

		// Check computed fields
		// for i, rule := range config {
		// 	if id, ok := rs.Primary.Attributes[fmt.Sprintf("rule.%d.id", i)]; !ok || len(id) != 20 {
		// 		return fmt.Errorf("Rule#%d doesn't have a valid ID: %q", i, id)
		// 	}
		// 	if arn, ok := rs.Primary.Attributes[fmt.Sprintf("rule.%d.arn", i)]; !ok || len(arn) != len(fmt.Sprintf("arn:minio:replication::xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx:%s", rule.Target.Bucket)) {
		// 		return fmt.Errorf("Rule#%d doesn't have a valid ARN:\n\nexpected: arn:minio:replication::xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx:%s\n\ngot: %v", i, rule.Target.Bucket, arn)
		// 	}
		// }

		// Check bucket replication
		actualReplicationConfigByPriority := map[int]replication.Rule{}
		for _, rule := range actualConfig.Rules {
			actualReplicationConfigByPriority[rule.Priority] = rule
		}
		for i, rule := range config {
			existingRule, ok := actualReplicationConfigByPriority[rule.Priority]
			if !ok {
				return fmt.Errorf("Rule with priority %d not found. Available: %v", rule.Priority, actualReplicationConfigByPriority)
			}
			if (existingRule.Status == replication.Enabled) != rule.Enabled {
				return fmt.Errorf("Mismatch status on res %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, (existingRule.Status == replication.Enabled), rule.Enabled)
			}
			if existingRule.Priority != rule.Priority {
				return fmt.Errorf("Mismatch priority on res %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingRule.Priority, rule.Priority)
			}
			if (existingRule.DeleteMarkerReplication.Status == replication.Enabled) != rule.DeleteMarkerReplication {
				return fmt.Errorf("Mismatch DeleteMarkerReplication on res %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, (existingRule.DeleteMarkerReplication.Status == replication.Enabled), rule.DeleteMarkerReplication)
			}
			if (existingRule.DeleteReplication.Status == replication.Enabled) != rule.DeleteReplication {
				return fmt.Errorf("Mismatch DeleteReplication on res %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, (existingRule.DeleteReplication.Status == replication.Enabled), rule.DeleteReplication)
			}
			if (existingRule.SourceSelectionCriteria.ReplicaModifications.Status == replication.Enabled) != rule.MetadataSync {
				return fmt.Errorf("Mismatch SourceSelectionCriteria on res %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, (existingRule.SourceSelectionCriteria.ReplicaModifications.Status == replication.Enabled), rule.MetadataSync)
			}
			if (existingRule.ExistingObjectReplication.Status == replication.Enabled) != rule.ExistingObjectReplication {
				return fmt.Errorf("Mismatch ExistingObjectReplication on res %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, (existingRule.ExistingObjectReplication.Status == replication.Enabled), rule.ExistingObjectReplication)
			}
			if !strings.HasPrefix(existingRule.Destination.Bucket, fmt.Sprintf("arn:minio:replication:%s:", rule.Target.Region)) {
				return fmt.Errorf("Mismatch ARN bucket prefix on res %q, rule#%d:\n\nexpected: arn:minio:replication:%s:\n\ngot: %v", n, i, rule.Target.Region, existingRule.Destination.Bucket)
			}
			if !strings.HasSuffix(existingRule.Destination.Bucket, ":"+rule.Target.Bucket) {
				return fmt.Errorf("Mismatch Target bucket name on res %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingRule.Destination.Bucket, rule.Target.Bucket)
			}
			if existingRule.Destination.StorageClass != rule.Target.StorageClass {
				return fmt.Errorf("Mismatch Target StorageClass on res %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingRule.Destination.StorageClass, rule.Target.StorageClass)
			}
			if existingRule.Prefix() != rule.Prefix {
				return fmt.Errorf("Mismatch Prefix on res %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingRule.Prefix(), rule.Prefix)
			}
			tags := strings.Split(existingRule.Tags(), "&")
			for i, v := range tags {
				if v != "" {
					continue
				}
				tags = append(tags[:i], tags[i+1:]...)
			}
			if len(tags) != len(rule.Tags) {
				return fmt.Errorf("Mismatch tags %q, rule#%d:\n\nexpected: %v (size %d)\n\ngot: %v (size %d)", n, i, tags, len(tags), rule.Tags, len(rule.Tags))
			}
			for _, kv := range tags {
				val := strings.SplitN(kv, "=", 2)
				k := val[0]
				v := val[1]
				if cv, ok := rule.Tags[k]; !ok || v != cv {
					return fmt.Errorf("Mismatch tags %q, rule#%d:\n\nexpected: %s=%q\n\ngot: %s=%q (found: %t)", n, i, k, v, k, cv, ok)
				}
			}
		}

		// Check remote target
		actualTargets, err := minioadm.ListRemoteTargets(context.Background(), rs.Primary.ID, "")
		if err != nil {
			return fmt.Errorf("error on ListRemoteTargets: %v", err)
		}

		if len(actualTargets) != len(config) {
			return fmt.Errorf("non-equivalent status error:\n\nexpected: %d\n\ngot: %d", len(actualTargets), len(config))
		}
		actualRemoteTargetByArn := map[string]madmin.BucketTarget{}
		for _, target := range actualTargets {
			actualRemoteTargetByArn[target.Arn] = target
		}
		for i, rule := range config {
			existingRule, ok := actualReplicationConfigByPriority[rule.Priority]
			if !ok {
				return fmt.Errorf("Rule with priority %d not found. Available: %v", rule.Priority, actualReplicationConfigByPriority)
			}
			existingTarget, ok := actualRemoteTargetByArn[existingRule.Destination.Bucket]
			if !ok {
				return fmt.Errorf("Target with ARN %q not found. Available: %v", existingRule.Destination.Bucket, actualRemoteTargetByArn)

			}

			if existingTarget.Endpoint != rule.Target.Host {
				return fmt.Errorf("Mismatch endpoint %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingTarget.Endpoint, rule.Target.Host)
			}
			if existingTarget.Secure != rule.Target.Secure {
				return fmt.Errorf("Mismatch Secure %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingTarget.Secure, rule.Target.Secure)
			}
			if existingTarget.BandwidthLimit != rule.Target.BandwidthLimit {
				return fmt.Errorf("Mismatch BandwidthLimit %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingTarget.BandwidthLimit, rule.Target.BandwidthLimit)
			}
			if existingTarget.HealthCheckDuration != rule.Target.HealthCheckPeriod {
				return fmt.Errorf("Mismatch HealthCheckDuration %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingTarget.HealthCheckDuration, rule.Target.HealthCheckPeriod)
			}
			if existingTarget.Secure != rule.Target.Secure {
				return fmt.Errorf("Mismatch Secure %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingTarget.Secure, rule.Target.Secure)
			}
			bucket := rule.Target.Bucket
			cleanPath := strings.TrimPrefix(strings.TrimPrefix(rule.Target.Path, "/"), ".")
			if cleanPath != "" {
				bucket = cleanPath + "/" + rule.Target.Bucket
			}
			if existingTarget.TargetBucket != bucket {
				return fmt.Errorf("Mismatch TargetBucket %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingTarget.TargetBucket, bucket)
			}
			if existingTarget.ReplicationSync != rule.Target.Syncronous {
				return fmt.Errorf("Mismatch synchronous mode %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingTarget.ReplicationSync, rule.Target.Syncronous)
			}
			if existingTarget.Region != rule.Target.Region {
				return fmt.Errorf("Mismatch region %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingTarget.Region, rule.Target.Region)
			}
			if existingTarget.Path != rule.Target.PathStyle.String() {
				return fmt.Errorf("Mismatch path style %q, rule#%d:\n\nexpected: %v\n\ngot: %v", n, i, existingTarget.Path, rule.Target.PathStyle.String())
			}
			// Asserting exact AccessKey value is too painful. Furthermore, since MinIO assert the credential validity before accepting the new remote target, the value is very low
			if len(existingTarget.Credentials.AccessKey) != 20 {
				return fmt.Errorf("Mismatch AccessKey %q, rule#%d:\n\nexpected: 20-char string\n\ngot: %v", n, i, existingTarget.Credentials.AccessKey)
			}
		}

		return nil
	}
}
