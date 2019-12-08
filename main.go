package main

import (
	"github.com/aminueza/terraform-minio-provider/s3minio"
	"github.com/hashicorp/terraform/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: s3minio.Provider,
	})
}
