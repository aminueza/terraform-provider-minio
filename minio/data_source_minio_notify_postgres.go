package minio

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func dataSourceMinioNotifyPostgres() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_postgres",
		readFields: readNotifyPostgresFields,
	}
	return dataSourceNotify(nrc, resourceMinioNotifyPostgres().Schema)
}
