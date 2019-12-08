package main

import (
	"github.com/aminueza/terraform-minio-provider/pconfig"
	"github.com/hashicorp/terraform/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: pconfig.Provider,
	})
}
