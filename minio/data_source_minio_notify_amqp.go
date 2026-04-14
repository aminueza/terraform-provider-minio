package minio

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func dataSourceMinioNotifyAmqp() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_amqp",
		readFields: readNotifyAmqpFields,
	}
	return dataSourceNotify(nrc, resourceMinioNotifyAmqp().Schema)
}
