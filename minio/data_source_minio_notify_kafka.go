package minio

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func dataSourceMinioNotifyKafka() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_kafka",
		readFields: readNotifyKafkaFields,
	}
	return dataSourceNotify(nrc, resourceMinioNotifyKafka().Schema)
}
