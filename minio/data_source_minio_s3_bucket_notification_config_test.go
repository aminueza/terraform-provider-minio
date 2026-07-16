package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3BucketNotificationConfig_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-ds-notif")
	dataSourceName := "data.minio_s3_bucket_notification_config.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceBucketNotificationConfig(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "bucket", name),
					resource.TestCheckResourceAttr(dataSourceName, "queue.#", "1"),
					resource.TestCheckResourceAttr(dataSourceName, "queue.0.arn", "arn:minio:sqs::primary:webhook"),
					resource.TestCheckResourceAttr(dataSourceName, "queue.0.events.#", "2"),
					resource.TestCheckResourceAttr(dataSourceName, "queue.0.prefix", "tf-acc-test/"),
					resource.TestCheckResourceAttr(dataSourceName, "queue.0.suffix", ".png"),
				),
			},
		},
	})
}

func testAccDataSourceBucketNotificationConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
}

resource "minio_s3_bucket_notification" "notification" {
  bucket = minio_s3_bucket.bucket.id

  queue {
    id        = "notification-queue"
    queue_arn = "arn:minio:sqs::primary:webhook"

    events = [
      "s3:ObjectCreated:*",
      "s3:ObjectRemoved:Delete",
    ]

    filter_prefix = "tf-acc-test/"
    filter_suffix = ".png"
  }
}

data "minio_s3_bucket_notification_config" "test" {
  bucket = minio_s3_bucket_notification.notification.bucket
}
`, name)
}
