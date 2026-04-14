package minio

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func dataSourceMinioNotifyNsq() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_nsq",
		readFields: readNotifyNsqFields,
	}
	return dataSourceNotify(nrc, resourceMinioNotifyNsq().Schema)
}
