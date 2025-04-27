package minio

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var testAccProviders map[string]func() (*schema.Provider, error)
var testAccProvider *schema.Provider
var testAccSecondProvider *schema.Provider
var testAccThirdProvider *schema.Provider
var testAccFourthProvider *schema.Provider

func init() {
	testAccProvider = newProvider()
	testAccSecondProvider = newProvider("SECOND_")
	testAccThirdProvider = newProvider("THIRD_")
	testAccFourthProvider = newProvider("FOURTH_")
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
	}
}

func TestProvider(t *testing.T) {
	if err := newProvider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ *schema.Provider = newProvider()
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
	var valid bool = true

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
	
	// Check that all MinIO instances are healthy before running tests
	checkMinioHealth()
}

// checkMinioHealth verifies that all MinIO instances are up and running
func checkMinioHealth() {
	minioEndpoints := []string{
		os.Getenv("MINIO_ENDPOINT"),
		os.Getenv("SECOND_MINIO_ENDPOINT"),
		os.Getenv("THIRD_MINIO_ENDPOINT"),
		os.Getenv("FOURTH_MINIO_ENDPOINT"),
	}

	fmt.Println("Checking MinIO instances health...")
	for i, endpoint := range minioEndpoints {
		if endpoint == "" {
			continue
		}

		var healthy bool
		var lastErr error
		for retries := 0; retries < 10; retries++ {
			// Try a simple HTTP request to the health endpoint
			healthURL := fmt.Sprintf("http://%s/minio/health/live", endpoint)
			fmt.Printf("Checking health of MinIO instance %d at %s (attempt %d/10)\n", i+1, endpoint, retries+1)
			
			resp, err := http.Get(healthURL)
			if err != nil {
				lastErr = err
				fmt.Printf("Error connecting to %s: %v\n", healthURL, err)
				time.Sleep(3 * time.Second)
				continue
			}
			
			if resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				healthy = true
				break
			}
			
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			fmt.Printf("Unexpected status code from %s: %d\n", healthURL, resp.StatusCode)
			resp.Body.Close()
			time.Sleep(3 * time.Second)
		}
		
		if !healthy {
			fmt.Printf("WARNING: MinIO instance %d at %s may not be fully healthy: %v\n", i+1, endpoint, lastErr)
			// Don't fail the test, just warn
		} else {
			fmt.Printf("MinIO instance %d at %s is healthy and ready\n", i+1, endpoint)
		}
	}

	// Add an additional delay to ensure everything is fully initialized
	fmt.Println("Waiting an additional 5 seconds before proceeding...")
	time.Sleep(5 * time.Second)
}
