package minio

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
)

// testAccProtoV5ProviderFactories is the framework provider factory for acceptance
// tests. All resources use this.
var testAccProtoV5ProviderFactories map[string]func() (tfprotov5.ProviderServer, error)

// testAccProtoV5SecondProviderFactories is used by site-replication tests that
// spin up multiple independent MinIO endpoints.
var testAccProtoV5SecondProviderFactories map[string]func() (tfprotov5.ProviderServer, error)

func newFrameworkProviderServer(envPrefix string) func() (tfprotov5.ProviderServer, error) {
	return func() (tfprotov5.ProviderServer, error) {
		frameworkServer := providerserver.NewProtocol5(NewFrameworkProvider("test")())
		return frameworkServer(), nil
	}
}

func init() {
	// Framework provider factories
	testAccProtoV5ProviderFactories = map[string]func() (tfprotov5.ProviderServer, error){
		"minio":       newFrameworkProviderServer(""),
		"secondminio": newFrameworkProviderServer("SECOND_"),
		"thirdminio":  newFrameworkProviderServer("THIRD_"),
		"fourthminio": newFrameworkProviderServer("FOURTH_"),
		"kmsminio":    newFrameworkProviderServer("KMS_"),
		"ldapminio":   newFrameworkProviderServer("LDAP_"),
	}

	testAccProtoV5SecondProviderFactories = map[string]func() (tfprotov5.ProviderServer, error){
		"secondminio": newFrameworkProviderServer("SECOND_"),
		"thirdminio":  newFrameworkProviderServer("THIRD_"),
		"fourthminio": newFrameworkProviderServer("FOURTH_"),
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

