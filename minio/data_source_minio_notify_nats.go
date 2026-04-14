package minio

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func dataSourceMinioNotifyNats() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_nats",
		readFields: readNotifyNatsFields,
	}
	return dataSourceNotify(nrc, resourceMinioNotifyNats().Schema)
}
