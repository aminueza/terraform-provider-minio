package minio

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func dataSourceMinioNotifyRedis() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_redis",
		readFields: readNotifyRedisFields,
	}
	return dataSourceNotify(nrc, resourceMinioNotifyRedis().Schema)
}
