package main

import (
	"github.com/aminueza/terraform-provider-minio/minio"
	"github.com/hashicorp/terraform-plugin-sdk/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: minio.Provider,
	})
}
