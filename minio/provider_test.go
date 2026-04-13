package minio

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-mux/tf5muxserver"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var testAccProviders map[string]func() (*schema.Provider, error)
var testAccProvider *schema.Provider
var testAccSecondProvider *schema.Provider
var testAccThirdProvider *schema.Provider
var testAccFourthProvider *schema.Provider
var testAccKmsProvider *schema.Provider
var testAccLdapProvider *schema.Provider

// testAccProtoV5ProviderFactories is the muxed provider factory for acceptance
// tests. All resources migrated to the framework must use this.
var testAccProtoV5ProviderFactories map[string]func() (tfprotov5.ProviderServer, error)

// testAccProtoV5SecondProviderFactories is used by site-replication tests that
// spin up multiple independent MinIO endpoints.
var testAccProtoV5SecondProviderFactories map[string]func() (tfprotov5.ProviderServer, error)

func newMuxedProviderServer(envPrefix string) func() (tfprotov5.ProviderServer, error) {
	return func() (tfprotov5.ProviderServer, error) {
		ctx := context.Background()
		sdkProvider := newProvider(envPrefix).GRPCProvider
		frameworkServer := providerserver.NewProtocol5(NewFrameworkProvider("test")())
		mux, err := tf5muxserver.NewMuxServer(ctx, sdkProvider, frameworkServer)
		if err != nil {
			return nil, err
		}
		return mux.ProviderServer(), nil
	}
}

func init() {
	testAccProvider = newProvider()
	testAccSecondProvider = newProvider("SECOND_")
	testAccThirdProvider = newProvider("THIRD_")
	testAccFourthProvider = newProvider("FOURTH_")
	testAccKmsProvider = newProvider("KMS_")
	testAccLdapProvider = newProvider("LDAP_")
	testAccProviders = map[string]func() (*schema.Provider, error){
		"minio": func() (*schema.Provider, error) {
			return testAccProvider, nil
		},
		"secondminio": func() (*schema.Provider, error) {
			return testAccSecondProvider, nil
		},
		"thirdminio": func() (*schema.Provider, error) {
			return testAccThirdProvider, nil
		},
		"fourthminio": func() (*schema.Provider, error) {
			return testAccFourthProvider, nil
		},
		"kmsminio": func() (*schema.Provider, error) {
			return testAccKmsProvider, nil
		},
		"ldapminio": func() (*schema.Provider, error) {
			return testAccLdapProvider, nil
		},
	}

	testAccProtoV5ProviderFactories = map[string]func() (tfprotov5.ProviderServer, error){
		"minio":       newMuxedProviderServer(""),
		"secondminio": newMuxedProviderServer("SECOND_"),
		"thirdminio":  newMuxedProviderServer("THIRD_"),
		"fourthminio": newMuxedProviderServer("FOURTH_"),
		"kmsminio":    newMuxedProviderServer("KMS_"),
		"ldapminio":   newMuxedProviderServer("LDAP_"),
	}

	testAccProtoV5SecondProviderFactories = map[string]func() (tfprotov5.ProviderServer, error){
		"minio":       newMuxedProviderServer("SECOND_"),
		"secondminio": newMuxedProviderServer("SECOND_"),
		"thirdminio":  newMuxedProviderServer("THIRD_"),
		"fourthminio": newMuxedProviderServer("FOURTH_"),
	}
}

func TestProvider(t *testing.T) {
	if err := newProvider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ = newProvider()
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

// testMinioClient creates a direct MinIO client from environment variables.
// Use this in acceptance test check functions instead of testAccProvider.Meta().
func testMinioClient(t *testing.T) *S3MinioClient {
	t.Helper()
	cfg := &S3MinioConfig{
		S3HostPort:     os.Getenv("MINIO_ENDPOINT"),
		S3UserAccess:   os.Getenv("MINIO_USER"),
		S3UserSecret:   os.Getenv("MINIO_PASSWORD"),
		S3Region:       "us-east-1",
		S3APISignature: "v4",
		S3SSL:          os.Getenv("MINIO_ENABLE_HTTPS") == "true" || os.Getenv("MINIO_ENABLE_HTTPS") == "1",
	}
	raw, err := cfg.NewClient()
	if err != nil {
		t.Fatalf("failed to create test MinIO client: %s", err)
	}
	return raw.(*S3MinioClient)
}

// testMustGetMinioClient returns a MinIO client built from env vars. For use in
// check functions where *testing.T is not in scope. Panics on error.
func testMustGetMinioClient() *S3MinioClient {
	return testMustGetMinioClientWithPrefix("")
}

// testMustGetMinioClientWithPrefix returns a MinIO client for an endpoint
// identified by the given env var prefix. Panics on error.
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

// testMinioClientWithPrefix creates a MinIO client for a secondary endpoint
// identified by the given env var prefix (e.g. "SECOND_").
func testMinioClientWithPrefix(t *testing.T, prefix string) *S3MinioClient {
	t.Helper()
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
		t.Fatalf("failed to create test MinIO client (prefix=%s): %s", prefix, err)
	}
	return raw.(*S3MinioClient)
}
