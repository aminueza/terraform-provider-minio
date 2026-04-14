package main

import (
	"context"
	"flag"
	"log"

	"github.com/aminueza/terraform-provider-minio/v3/minio"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6/tf6server"
	"github.com/hashicorp/terraform-plugin-mux/tf5to6server"
	"github.com/hashicorp/terraform-plugin-mux/tf6muxserver"
)

var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers")
	flag.Parse()

	ctx := context.Background()

	// Upgrade the SDK provider (Protocol 5) to Protocol 6 so it can be muxed
	upgradedSdkProvider, err := tf5to6server.UpgradeServer(
		ctx,
		minio.Provider().GRPCProvider,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Mux: SDK provider (data sources only) + framework provider (all resources)
	muxServer, err := tf6muxserver.NewMuxServer(ctx,
		func() tfprotov6.ProviderServer { return upgradedSdkProvider },
		providerserver.NewProtocol6(minio.NewFrameworkProvider(version)()),
	)
	if err != nil {
		log.Fatal(err)
	}

	var serveOpts []tf6server.ServeOpt
	if debug {
		serveOpts = append(serveOpts, tf6server.WithManagedDebug())
	}

	err = tf6server.Serve(
		"registry.terraform.io/aminueza/minio",
		muxServer.ProviderServer,
		serveOpts...,
	)
	if err != nil {
		log.Fatal(err)
	}
}
