package main

import (
	"flag"
	"log"

	"github.com/aminueza/terraform-provider-minio/v3/minio"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6/tf6server"
)

var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers")
	flag.Parse()

	frameworkProvider := providerserver.NewProtocol6(minio.NewFrameworkProvider(version)())

	var serveOpts []tf6server.ServeOpt
	if debug {
		serveOpts = append(serveOpts, tf6server.WithManagedDebug())
	}

	err := tf6server.Serve(
		"registry.terraform.io/aminueza/minio",
		frameworkProvider,
		serveOpts...,
	)
	if err != nil {
		log.Fatal(err)
	}
}
