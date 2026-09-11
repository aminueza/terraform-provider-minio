package minio

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

func TestMuxProviderServerStarts(t *testing.T) {
	if _, err := MuxProviderServer(context.Background()); err != nil {
		t.Fatalf("building the mux server: %s", err)
	}
}

func TestMuxServerSchemasMatch(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
	}{
		{name: "MINIO_ENDPOINT unset", endpoint: ""},
		{name: "MINIO_ENDPOINT set", endpoint: "localhost:9000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MINIO_ENDPOINT", tc.endpoint)

			ctx := context.Background()

			server, err := MuxProviderServer(ctx)
			if err != nil {
				t.Fatalf("building the mux server: %s", err)
			}

			resp, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
			if err != nil {
				t.Fatalf("reading the provider schema: %s", err)
			}

			for _, diagnostic := range resp.Diagnostics {
				if diagnostic.Severity == tfprotov6.DiagnosticSeverityError {
					t.Fatalf("the SDKv2 and framework provider schemas disagree: %s: %s", diagnostic.Summary, diagnostic.Detail)
				}
			}

			if resp.Provider == nil {
				t.Fatal("the mux server returned no provider schema")
			}
		})
	}
}

func TestMuxServerServesEphemeralResources(t *testing.T) {
	ctx := context.Background()

	server, err := MuxProviderServer(ctx)
	if err != nil {
		t.Fatalf("building the mux server: %s", err)
	}

	resp, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("reading the provider schema: %s", err)
	}

	if _, ok := resp.EphemeralResourceSchemas["minio_sts_credentials"]; !ok {
		t.Fatalf("minio_sts_credentials is not served; the mux advertises %d ephemeral resources", len(resp.EphemeralResourceSchemas))
	}
}

func TestMuxServerStillServesSDKResources(t *testing.T) {
	ctx := context.Background()

	server, err := MuxProviderServer(ctx)
	if err != nil {
		t.Fatalf("building the mux server: %s", err)
	}

	resp, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("reading the provider schema: %s", err)
	}

	sdkProvider := Provider()
	for name := range sdkProvider.ResourcesMap {
		if _, ok := resp.ResourceSchemas[name]; !ok {
			t.Errorf("resource %s is no longer served through the mux", name)
		}
	}
	for name := range sdkProvider.DataSourcesMap {
		if _, ok := resp.DataSourceSchemas[name]; !ok {
			t.Errorf("data source %s is no longer served through the mux", name)
		}
	}
}
