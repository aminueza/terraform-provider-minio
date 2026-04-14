package minio

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func dataSourceMinioNotifyElasticsearch() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_elasticsearch",
		readFields: readNotifyElasticsearchFields,
	}
	return dataSourceNotify(nrc, resourceMinioNotifyElasticsearch().Schema)
}
