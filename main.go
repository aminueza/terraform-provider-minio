package main

import (
	minio "github.com/aminueza/terraform-provider-minio/minio"
	"github.com/hashicorp/terraform/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: minio.Provider,
	})
}
