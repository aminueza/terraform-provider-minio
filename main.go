package main

import (
	"context"
	"flag"
	"log"

	"github.com/aminueza/terraform-provider-minio/v3/minio"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6/tf6server"
)

const providerAddr = "registry.terraform.io/aminueza/minio"

func main() {
	var debugMode bool

	flag.BoolVar(&debugMode, "debuggable", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	ctx := context.Background()

	muxServer, err := minio.MuxProviderServer(ctx)
	if err != nil {
		log.Fatal(err)
	}

	var serveOpts []tf6server.ServeOpt
	if debugMode {
		serveOpts = append(serveOpts, tf6server.WithManagedDebug())
	}

	if err := tf6server.Serve(providerAddr, func() tfprotov6.ProviderServer { return muxServer }, serveOpts...); err != nil {
		log.Fatal(err)
	}
}
