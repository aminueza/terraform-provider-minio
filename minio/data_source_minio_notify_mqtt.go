package minio

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func dataSourceMinioNotifyMqtt() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_mqtt",
		readFields: readNotifyMqttFields,
	}
	return dataSourceNotify(nrc, resourceMinioNotifyMqtt().Schema)
}
