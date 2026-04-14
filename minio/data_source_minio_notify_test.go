package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioNotifyWebhook_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	dataSourceName := "data.minio_notify_webhook.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceNotifyWebhookConfig(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "name", name),
					resource.TestCheckResourceAttr(dataSourceName, "endpoint", "http://minio:9000"),
				),
			},
		},
	})
}

func TestDataSourceNotifySchemaValidation(t *testing.T) {
	tests := []struct {
		name string
		fn   func() interface{}
	}{
		{"webhook", func() interface{} { return dataSourceMinioNotifyWebhook() }},
		{"amqp", func() interface{} { return dataSourceMinioNotifyAmqp() }},
		{"kafka", func() interface{} { return dataSourceMinioNotifyKafka() }},
		{"mqtt", func() interface{} { return dataSourceMinioNotifyMqtt() }},
		{"nats", func() interface{} { return dataSourceMinioNotifyNats() }},
		{"nsq", func() interface{} { return dataSourceMinioNotifyNsq() }},
		{"mysql", func() interface{} { return dataSourceMinioNotifyMysql() }},
		{"postgres", func() interface{} { return dataSourceMinioNotifyPostgres() }},
		{"elasticsearch", func() interface{} { return dataSourceMinioNotifyElasticsearch() }},
		{"redis", func() interface{} { return dataSourceMinioNotifyRedis() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := tt.fn()
			if res == nil {
				t.Fatalf("data source %s returned nil", tt.name)
			}
		})
	}
}

func testAccDataSourceNotifyWebhookConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_webhook" "test" {
  name     = %[1]q
  endpoint = "http://minio:9000"
  enable   = false
}

data "minio_notify_webhook" "test" {
  name = minio_notify_webhook.test.name
}
`, name)
}
