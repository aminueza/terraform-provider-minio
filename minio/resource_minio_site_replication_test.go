package minio

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const kBasicSiteReplicationResource = `
resource "minio_site_replication" "basic" {
  name = "%s"

  site {
    name       = "site1"
    endpoint   = "%s"
    access_key = "%s"
    secret_key = "%s"
  }

  site {
    name       = "site2"
    endpoint   = "%s"
    access_key = "%s"
    secret_key = "%s"
  }
}`

const kThreeSiteReplicationResource = `
resource "minio_site_replication" "three_site" {
  name = "%s"

  site {
    name       = "site1"
    endpoint   = "%s"
    access_key = "%s"
    secret_key = "%s"
  }

  site {
    name       = "site2"
    endpoint   = "%s"
    access_key = "%s"
    secret_key = "%s"
  }

  site {
    name       = "site3"
    endpoint   = "%s"
    access_key = "%s"
    secret_key = "%s"
  }
}`

func testAccPreCheckSiteReplication(t *testing.T) {
	if os.Getenv("MINIO_USER") == "" || os.Getenv("MINIO_PASSWORD") == "" {
		t.Skip("MINIO_USER or MINIO_PASSWORD not set for acceptance test")
	}

	if os.Getenv("MINIO_USER") == "" || os.Getenv("MINIO_PASSWORD") == "" {
		t.Skip("MINIO_USER or MINIO_PASSWORD not set for acceptance test")
	}
}

func cleanupAllBuckets(t *testing.T) {
	t.Helper()

	cleanupBucketsForEndpoint(t, "minio:9000", os.Getenv("MINIO_USER"), os.Getenv("MINIO_PASSWORD"))

	cleanupBucketsForEndpoint(t, "secondminio:9000", os.Getenv("SECOND_MINIO_USER"), os.Getenv("SECOND_MINIO_PASSWORD"))

	cleanupBucketsForEndpoint(t, "thirdminio:9000", os.Getenv("THIRD_MINIO_USER"), os.Getenv("THIRD_MINIO_PASSWORD"))
}

func cleanupBucketsForEndpoint(t *testing.T, endpoint, accessKey, secretKey string) {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	if err != nil {
		t.Logf("Warning: Failed to create MinIO client for %s: %v", endpoint, err)
		return
	}

	ctx := context.Background()
	buckets, err := minioClient.ListBuckets(ctx)
	if err != nil {
		t.Logf("Warning: Failed to list buckets for %s: %v", endpoint, err)
		return
	}

	for _, bucket := range buckets {
		objectsCh := make(chan minio.ObjectInfo)

		go func() {
			defer close(objectsCh)
			for object := range minioClient.ListObjects(ctx, bucket.Name, minio.ListObjectsOptions{
				Recursive:    true,
				WithVersions: true,
			}) {
				objectsCh <- object
			}
		}()

		removeErrors := minioClient.RemoveObjects(ctx, bucket.Name, objectsCh, minio.RemoveObjectsOptions{})
		for err := range removeErrors {
			t.Logf("Warning: Failed to remove object from bucket %s on %s: %v", bucket.Name, endpoint, err)
		}

		err = minioClient.RemoveBucket(ctx, bucket.Name)
		if err != nil {
			t.Logf("Warning: Failed to remove bucket %s from %s: %v", bucket.Name, endpoint, err)
		} else {
			t.Logf("Cleaned up bucket %s from %s", bucket.Name, endpoint)
		}
	}
}

func TestAccMinioSiteReplication_basic(t *testing.T) {
	cleanupAllBuckets(t)

	replicationName := acctest.RandomWithPrefix("tf-acc-site-repl")

	primaryMinioEndpoint := "http://minio:9000"
	primaryMinioUser := os.Getenv("MINIO_USER")
	primaryMinioPassword := os.Getenv("MINIO_PASSWORD")

	secondaryMinioEndpoint := "http://secondminio:9000"
	secondaryMinioUser := os.Getenv("SECOND_MINIO_USER")
	secondaryMinioPassword := os.Getenv("SECOND_MINIO_PASSWORD")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSiteReplication(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioSiteReplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(kBasicSiteReplicationResource,
					replicationName,
					primaryMinioEndpoint, primaryMinioUser, primaryMinioPassword,
					secondaryMinioEndpoint, secondaryMinioUser, secondaryMinioPassword,
				),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckSiteReplicationExists("minio_site_replication.basic"),
					resource.TestCheckResourceAttr("minio_site_replication.basic", "enabled", "true"),
					resource.TestCheckResourceAttr("minio_site_replication.basic", "site.#", "2"),
					resource.TestCheckResourceAttr("minio_site_replication.basic", "site.0.name", "site1"),
					resource.TestCheckResourceAttr("minio_site_replication.basic", "site.0.endpoint", primaryMinioEndpoint),
					resource.TestCheckResourceAttr("minio_site_replication.basic", "site.1.name", "site2"),
					resource.TestCheckResourceAttr("minio_site_replication.basic", "site.1.endpoint", secondaryMinioEndpoint),
				),
			},
			{
				ResourceName:      "minio_site_replication.basic",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"site.0.access_key",
					"site.0.secret_key",
					"site.1.access_key",
					"site.1.secret_key",
				},
				Check: testAccCheckImportSiteReplicationExists("minio_site_replication.basic", replicationName),
			},
		},
	})
}

func TestAccMinioSiteReplication_threeSites(t *testing.T) {
	cleanupAllBuckets(t)

	replicationName := acctest.RandomWithPrefix("tf-acc-site-repl-3")

	primaryMinioEndpoint := "http://minio:9000"
	primaryMinioUser := os.Getenv("MINIO_USER")
	primaryMinioPassword := os.Getenv("MINIO_PASSWORD")

	secondaryMinioEndpoint := "http://secondminio:9000"
	secondaryMinioUser := os.Getenv("SECOND_MINIO_USER")
	secondaryMinioPassword := os.Getenv("SECOND_MINIO_PASSWORD")

	thirdMinioEndpoint := "http://thirdminio:9000"
	thirdMinioUser := os.Getenv("THIRD_MINIO_USER")
	thirdMinioPassword := os.Getenv("THIRD_MINIO_PASSWORD")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSiteReplication(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioSiteReplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(kThreeSiteReplicationResource,
					replicationName,
					primaryMinioEndpoint, primaryMinioUser, primaryMinioPassword,
					secondaryMinioEndpoint, secondaryMinioUser, secondaryMinioPassword,
					thirdMinioEndpoint, thirdMinioUser, thirdMinioPassword,
				),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckSiteReplicationExists("minio_site_replication.three_site"),
					resource.TestCheckResourceAttr("minio_site_replication.three_site", "enabled", "true"),
					resource.TestCheckResourceAttr("minio_site_replication.three_site", "site.#", "3"),
					resource.TestCheckResourceAttr("minio_site_replication.three_site", "site.0.name", "site1"),
					resource.TestCheckResourceAttr("minio_site_replication.three_site", "site.0.endpoint", primaryMinioEndpoint),
					resource.TestCheckResourceAttr("minio_site_replication.three_site", "site.1.name", "site2"),
					resource.TestCheckResourceAttr("minio_site_replication.three_site", "site.1.endpoint", secondaryMinioEndpoint),
					resource.TestCheckResourceAttr("minio_site_replication.three_site", "site.2.name", "site3"),
					resource.TestCheckResourceAttr("minio_site_replication.three_site", "site.2.endpoint", thirdMinioEndpoint),
				),
			},
		},
	})
}

func TestAccMinioSiteReplication_update(t *testing.T) {
	cleanupAllBuckets(t)

	replicationName := acctest.RandomWithPrefix("tf-acc-site-repl-update")

	primaryMinioEndpoint := "http://minio:9000"
	primaryMinioUser := os.Getenv("MINIO_USER")
	primaryMinioPassword := os.Getenv("MINIO_PASSWORD")

	secondaryMinioEndpoint := "http://secondminio:9000"
	secondaryMinioUser := os.Getenv("SECOND_MINIO_USER")
	secondaryMinioPassword := os.Getenv("SECOND_MINIO_PASSWORD")

	thirdMinioEndpoint := "http://thirdminio:9000"
	thirdMinioUser := os.Getenv("THIRD_MINIO_USER")
	thirdMinioPassword := os.Getenv("THIRD_MINIO_PASSWORD")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSiteReplication(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioSiteReplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(kBasicSiteReplicationResource,
					replicationName,
					primaryMinioEndpoint, primaryMinioUser, primaryMinioPassword,
					secondaryMinioEndpoint, secondaryMinioUser, secondaryMinioPassword,
				),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckSiteReplicationExists("minio_site_replication.basic"),
					resource.TestCheckResourceAttr("minio_site_replication.basic", "site.#", "2"),
				),
			},
			{
				Config: fmt.Sprintf(kThreeSiteReplicationResource,
					replicationName,
					primaryMinioEndpoint, primaryMinioUser, primaryMinioPassword,
					secondaryMinioEndpoint, secondaryMinioUser, secondaryMinioPassword,
					thirdMinioEndpoint, thirdMinioUser, thirdMinioPassword,
				),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckSiteReplicationExists("minio_site_replication.three_site"),
					resource.TestCheckResourceAttr("minio_site_replication.three_site", "site.#", "3"),
				),
			},
			{
				Config: fmt.Sprintf(kBasicSiteReplicationResource,
					replicationName,
					primaryMinioEndpoint, primaryMinioUser, primaryMinioPassword,
					thirdMinioEndpoint, thirdMinioUser, thirdMinioPassword,
				),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckSiteReplicationExists("minio_site_replication.basic"),
					resource.TestCheckResourceAttr("minio_site_replication.basic", "site.#", "2"),
				),
			},
		},
	})
}

func testAccCheckSiteReplicationExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		provider := testAccProvider.Meta().(*S3MinioClient)
		minioadm := provider.S3Admin

		info, err := minioadm.SiteReplicationInfo(context.Background())
		if err != nil {
			return fmt.Errorf("error getting site replication info: %v", err)
		}

		if !info.Enabled {
			return fmt.Errorf("site replication is not enabled")
		}

		return nil
	}
}

func testAccCheckImportSiteReplicationExists(n string, expectedName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		if name, ok := rs.Primary.Attributes["name"]; ok && name != expectedName {
			return fmt.Errorf("expected name %s, got %s", expectedName, name)
		}

		if enabled, ok := rs.Primary.Attributes["enabled"]; ok && enabled != "true" {
			return fmt.Errorf("expected enabled to be true, got %s", enabled)
		}

		if siteCount, ok := rs.Primary.Attributes["site.#"]; ok {
			if siteCount < "2" {
				return fmt.Errorf("expected at least 2 sites, got %s", siteCount)
			}
		}

		provider := testAccProvider.Meta().(*S3MinioClient)
		minioadm := provider.S3Admin

		info, err := minioadm.SiteReplicationInfo(context.Background())
		if err != nil {
			return fmt.Errorf("error getting site replication info: %v", err)
		}

		if !info.Enabled {
			return fmt.Errorf("site replication is not enabled")
		}

		return nil
	}
}

func testAccCheckMinioSiteReplicationDestroy(s *terraform.State) error {
	provider := testAccProvider.Meta().(*S3MinioClient)
	minioadm := provider.S3Admin

	info, err := minioadm.SiteReplicationInfo(context.Background())
	if err != nil {
		if err.Error() != "site replication not configured" {
			return fmt.Errorf("error checking site replication info: %v", err)
		}
		return nil
	}

	if info.Enabled {
		siteNames := make([]string, len(info.Sites))
		for i, site := range info.Sites {
			siteNames[i] = site.Name
		}

		_, err := minioadm.SiteReplicationRemove(context.Background(), madmin.SRRemoveReq{
			SiteNames: siteNames,
			RemoveAll: true,
		})
		if err != nil {
			return fmt.Errorf("failed to clean up site replication during test cleanup: %v", err)
		}

		info, err := minioadm.SiteReplicationInfo(context.Background())
		if err == nil && info.Enabled {
			return fmt.Errorf("site replication is still enabled on the cluster after cleanup attempt")
		}
	}

	return nil
}

func TestAccMinioSiteReplication_errorConditions(t *testing.T) {
	if os.Getenv("MINIO_USER") == "" || os.Getenv("MINIO_PASSWORD") == "" {
		t.Skip("MINIO_USER or MINIO_PASSWORD not set for acceptance test")
	}

	replicationName := acctest.RandomWithPrefix("tf-acc-site-repl-error")

	primaryMinioEndpoint := "http://minio:9000"
	primaryMinioUser := os.Getenv("MINIO_USER")
	primaryMinioPassword := os.Getenv("MINIO_PASSWORD")

	invalidEndpoint := "http://nonexistent:9000"
	invalidUser := "invalid"
	invalidPassword := "invalid"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSiteReplication(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "minio_site_replication" "error_test" {
  name = "%s"

  site {
    name       = "valid-site"
    endpoint   = "%s"
    access_key = "%s"
    secret_key = "%s"
  }

  site {
    name       = "invalid-credentials"
    endpoint   = "%s"
    access_key = "%s"
    secret_key = "%s"
  }
}`, replicationName, primaryMinioEndpoint, primaryMinioUser, primaryMinioPassword,
					invalidEndpoint, invalidUser, invalidPassword),
				ExpectError: regexp.MustCompile("error.*replication|Unable to fetch server info"),
			},
		},
	})

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSiteReplication(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "minio_site_replication" "connectivity_test" {
  name = "%s-connectivity"

  site {
    name       = "site1"
    endpoint   = "%s"
    access_key = "%s"
    secret_key = "%s"
  }

  site {
    name       = "site2"
    endpoint   = "http://192.0.2.1:9000"  # Reserved IP for documentation/test purposes
    access_key = "test"
    secret_key = "test"
  }
}`, replicationName, primaryMinioEndpoint, primaryMinioUser, primaryMinioPassword),
				ExpectError: regexp.MustCompile("error.*replication|connection|Unable to fetch server info|dial tcp|network"),
			},
		},
	})

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSiteReplication(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "minio_site_replication" "single_site_test" {
  name = "%s-single-site"

  site {
    name       = "only-site"
    endpoint   = "%s"
    access_key = "%s"
    secret_key = "%s"
  }
}`, replicationName, primaryMinioEndpoint, primaryMinioUser, primaryMinioPassword),
				ExpectError: regexp.MustCompile("Insufficient site blocks|At least 2 site blocks"),
			},
		},
	})
}
