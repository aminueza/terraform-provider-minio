package minio

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func dataSourceMinioNotifyWebhook() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_webhook",
		readFields: readNotifyWebhookFields,
	}
	return dataSourceNotify(nrc, resourceMinioNotifyWebhook().Schema)
}
