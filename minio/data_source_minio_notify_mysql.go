package minio

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func dataSourceMinioNotifyMysql() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_mysql",
		readFields: readNotifyMysqlFields,
	}
	return dataSourceNotify(nrc, resourceMinioNotifyMysql().Schema)
}
