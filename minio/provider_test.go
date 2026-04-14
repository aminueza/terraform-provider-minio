package minio

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-mux/tf5to6server"
	"github.com/hashicorp/terraform-plugin-mux/tf6muxserver"
)

// testAccProtoV6ProviderFactories is the mux provider factory for acceptance
// tests. It combines the SDK provider (data sources) with the framework provider (resources).
var testAccProtoV6ProviderFactories map[string]func() (tfprotov6.ProviderServer, error)

// testAccProtoV6SecondProviderFactories is used by site-replication tests that
// spin up multiple independent MinIO endpoints.
var testAccProtoV6SecondProviderFactories map[string]func() (tfprotov6.ProviderServer, error)

func newMuxProviderServer(envPrefix string) func() (tfprotov6.ProviderServer, error) {
	return func() (tfprotov6.ProviderServer, error) {
		ctx := context.Background()

		upgradedSdkProvider, err := tf5to6server.UpgradeServer(
			ctx,
			Provider().GRPCProvider,
		)
		if err != nil {
			return nil, err
		}

		muxServer, err := tf6muxserver.NewMuxServer(ctx,
			func() tfprotov6.ProviderServer { return upgradedSdkProvider },
			providerserver.NewProtocol6(NewFrameworkProvider("test")()),
		)
		if err != nil {
			return nil, err
		}

		return muxServer.ProviderServer(), nil
	}
}

func init() {
	testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
		"minio":       newMuxProviderServer(""),
		"secondminio": newMuxProviderServer("SECOND_"),
		"thirdminio":  newMuxProviderServer("THIRD_"),
		"fourthminio": newMuxProviderServer("FOURTH_"),
		"kmsminio":    newMuxProviderServer("KMS_"),
		"ldapminio":   newMuxProviderServer("LDAP_"),
	}

	testAccProtoV6SecondProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
		"secondminio": newMuxProviderServer("SECOND_"),
		"thirdminio":  newMuxProviderServer("THIRD_"),
		"fourthminio": newMuxProviderServer("FOURTH_"),
	}
}

func TestProvider(t *testing.T) {
	// Framework provider validation is handled by the framework itself
}

func TestProvider_impl(t *testing.T) {
	// Framework provider implementation check
	var _ = NewFrameworkProvider("test")
}

var kEnvVarNeeded = []string{
	"MINIO_ENDPOINT",
	"MINIO_USER",
	"MINIO_PASSWORD",
	"MINIO_ENABLE_HTTPS",
	"SECOND_MINIO_ENDPOINT",
	"SECOND_MINIO_USER",
	"SECOND_MINIO_PASSWORD",
	"SECOND_MINIO_ENABLE_HTTPS",
	"THIRD_MINIO_ENDPOINT",
	"THIRD_MINIO_USER",
	"THIRD_MINIO_PASSWORD",
	"THIRD_MINIO_ENABLE_HTTPS",
	"FOURTH_MINIO_ENDPOINT",
	"FOURTH_MINIO_USER",
	"FOURTH_MINIO_PASSWORD",
	"FOURTH_MINIO_ENABLE_HTTPS",
}

func testAccPreCheck(t *testing.T) {
	valid := true

	if v, _ := os.LookupEnv("TF_ACC"); v == "" {
		valid = false
	}

	for _, envvar := range kEnvVarNeeded {
		if _, ok := os.LookupEnv(envvar); !ok {
			valid = false
			break
		}
	}

	if !valid {
		t.Fatal("you must to set env variables for integration tests!")
	}
}

// testMustGetMinioClient returns a MinIO client built from env vars.
func testMustGetMinioClient() *S3MinioClient {
	return testMustGetMinioClientWithPrefix("")
}

// testMustGetMinioClientWithPrefix returns a MinIO client for an endpoint
// identified by the given env var prefix.
func testMustGetMinioClientWithPrefix(prefix string) *S3MinioClient {
	cfg := &S3MinioConfig{
		S3HostPort:     os.Getenv(prefix + "MINIO_ENDPOINT"),
		S3UserAccess:   os.Getenv(prefix + "MINIO_USER"),
		S3UserSecret:   os.Getenv(prefix + "MINIO_PASSWORD"),
		S3Region:       "us-east-1",
		S3APISignature: "v4",
		S3SSL:          os.Getenv(prefix+"MINIO_ENABLE_HTTPS") == "true" || os.Getenv(prefix+"MINIO_ENABLE_HTTPS") == "1",
	}
	raw, err := cfg.NewClient()
	if err != nil {
		panic("failed to create test MinIO client (prefix=" + prefix + "): " + err.Error())
	}
	return raw.(*S3MinioClient)
}
