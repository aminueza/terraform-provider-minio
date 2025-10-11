package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7/pkg/notification"
)

func TestS3BucketNotification_queue(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-notification-test")

	config := notification.Configuration{}
	arn, _ := notification.NewArnFromString("arn:minio:sqs::primary:webhook")
	qc := notification.NewConfig(arn)
	qc.ID = "notification-queue"
	qc.AddEvents(notification.ObjectCreatedAll, notification.ObjectRemovedDelete)
	qc.AddFilterPrefix("tf-acc-test/")
	qc.AddFilterSuffix(".png")
	config.AddQueue(qc)

	updateConfig := notification.Configuration{}
	updateQc := notification.NewConfig(arn)
	updateQc.ID = "notification-queue"
	updateQc.AddEvents(notification.ObjectCreatedAll, notification.ObjectRemovedDelete)
	updateQc.AddFilterPrefix("tf-acc-test/")
	updateQc.AddFilterSuffix(".mp4")
	updateConfig.AddQueue(updateQc)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketNotificationConfig_queue(name, ".png"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBucketHasNotification(
						"minio_s3_bucket_notification.notification",
						config,
					),
				),
			},
			{
				Config: testAccBucketNotificationConfig_queue(name, ".mp4"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBucketHasNotification(
						"minio_s3_bucket_notification.notification",
						updateConfig,
					),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_notification.notification",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccBucketNotificationConfig_queue(name string, suffix string) string {
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
    filter_suffix = %[2]q
  }
}
`, name, suffix)
}

func testAccCheckBucketHasNotification(n string, config notification.Configuration) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		actualConfig, err := minioC.GetBucketNotification(context.Background(), rs.Primary.ID)

		if err != nil {
			return fmt.Errorf("error on GetBucketNotification: %v", err)
		}

		if len(actualConfig.QueueConfigs) != len(config.QueueConfigs) {
			return fmt.Errorf("non-equivalent queue configuration error:\n\nexpected len: %v\n\ngot: %v", len(actualConfig.QueueConfigs), len(config.QueueConfigs))
		}

		for _, actualQueueConfig := range actualConfig.QueueConfigs {
			for _, queueConfig := range config.QueueConfigs {
				if actualQueueConfig.Queue != queueConfig.Config.Arn.String() {
					return fmt.Errorf("non-equivalent queue configuration error:\n\nexpected %s\n\ngot: %s", actualQueueConfig.Queue, queueConfig.Config.Arn.String())
				}
				if !notificationConfigsEqual(actualQueueConfig.Config, queueConfig.Config) {
					return fmt.Errorf("non-equivalent queue configuration error:\n\nexpected: %v\n\ngot: %v", queueConfig.Config, actualQueueConfig.Config)
				}
			}
		}

		return nil
	}
}

func notificationConfigsEqual(a notification.Config, b notification.Config) bool {
	return a.ID == b.ID && notification.EqualEventTypeList(a.Events, b.Events) && notification.EqualFilterRuleList(a.Filter.S3Key.FilterRules, b.Filter.S3Key.FilterRules)
}

func TestAccMinioS3BucketNotification_disappearsBucket(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-notification-test")
	config := notification.Configuration{}
	arn, _ := notification.NewArnFromString("arn:minio:sqs::primary:webhook")
	qc := notification.NewConfig(arn)
	qc.ID = "notification-queue"
	qc.AddEvents(notification.ObjectCreatedAll, notification.ObjectRemovedDelete)
	qc.AddFilterPrefix("tf-acc-test/")
	qc.AddFilterSuffix(".png")
	config.AddQueue(qc)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketNotificationConfig_queue(name, ".png"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBucketHasNotification(
						"minio_s3_bucket_notification.notification",
						config,
					),
					testAccCheckMinioS3DestroyBucket("minio_s3_bucket.bucket"),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
