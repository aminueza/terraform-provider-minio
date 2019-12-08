package main

import (
	s3minio "./s3minio"
	"github.com/hashicorp/terraform/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: s3minio.Provider,
	})
}
