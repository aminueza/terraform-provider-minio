package main

import (
	"context"
	"flag"
	"log"

	minio "github.com/aminueza/terraform-provider-minio/minio"
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
)

func main() {
	var debugMode bool

	flag.BoolVar(&debugMode, "debuggable", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	if debugMode {
		err := plugin.Debug(context.Background(), "registry.terraform.io/aminueza/minio",
			&plugin.ServeOpts{
				ProviderFunc: minio.Provider,
			})
		if err != nil {
			log.Println(err.Error())
		}
	} else {
		plugin.Serve(&plugin.ServeOpts{
			ProviderFunc: minio.Provider})
	}
}
