package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func testAccMuxProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"minio": func() (tfprotov6.ProviderServer, error) {
			return MuxProviderServer(context.Background())
		},
	}
}

func testAccRequireTerraform(t *testing.T, minMajor, minMinor int) {
	t.Helper()

	binary := os.Getenv("TF_ACC_TERRAFORM_PATH")
	if binary == "" {
		found, err := exec.LookPath("terraform")
		if err != nil {
			t.Skipf("terraform is not on PATH, so the required version cannot be checked: %s", err)
		}
		binary = found
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, binary, "version", "-json").Output()
	if err != nil {
		t.Skipf("reading the version of %s: %s", binary, err)
	}

	var parsed struct {
		Version string `json:"terraform_version"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Skipf("parsing the version reported by %s: %s", binary, err)
	}

	parts := strings.SplitN(parsed.Version, ".", 3)
	if len(parts) < 2 {
		t.Skipf("%s reports an unreadable version %q", binary, parsed.Version)
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		t.Skipf("%s reports an unreadable version %q", binary, parsed.Version)
	}

	if major < minMajor || (major == minMajor && minor < minMinor) {
		t.Skipf("Terraform %s is below %d.%d, which ephemeral resources need", parsed.Version, minMajor, minMinor)
	}
}

func TestAccMuxServesTheSDKHalfOverProtocolSix(t *testing.T) {
	bucketName := "tfacc-mux-sdk-" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccMuxProviderFactories(),
		CheckDestroy:             testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_s3_bucket" "sdk" {
  bucket = "` + bucketName + `"
}

data "minio_s3_bucket" "sdk" {
  bucket = minio_s3_bucket.sdk.bucket
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_s3_bucket.sdk", "bucket", bucketName),
					resource.TestCheckResourceAttr("data.minio_s3_bucket.sdk", "bucket", bucketName),
					resource.TestCheckResourceAttrSet("data.minio_s3_bucket.sdk", "region"),
				),
			},
		},
	})
}

func TestAccMuxServesEphemeralCredentialsToAnotherProvider(t *testing.T) {
	bucketName := "tfacc-mux-ephemeral-" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccRequireTerraform(t, 1, 10)
		},
		ProtoV6ProviderFactories: testAccMuxProviderFactories(),
		CheckDestroy:             testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
ephemeral "minio_sts_credentials" "scoped" {
  session_name     = "tfacc-mux"
  duration_seconds = 3600
}

provider "minio" {
  alias               = "scoped"
  minio_server        = %q
  minio_ssl           = %s
  minio_user          = ephemeral.minio_sts_credentials.scoped.access_key
  minio_password      = ephemeral.minio_sts_credentials.scoped.secret_key
  minio_session_token = ephemeral.minio_sts_credentials.scoped.session_token
}

resource "minio_s3_bucket" "scoped" {
  provider = minio.scoped
  bucket   = %q
}`, testAccEndpoint(""), testAccEnableHTTPS(""), bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_s3_bucket.scoped", "bucket", bucketName),
					testAccCheckStateHoldsNoEphemeralCredentials,
				),
			},
		},
	})
}

func testAccEnableHTTPS(prefix string) string {
	enabled, _ := strconv.ParseBool(os.Getenv(prefix + "MINIO_ENABLE_HTTPS"))
	return strconv.FormatBool(enabled)
}

func testAccCheckStateHoldsNoEphemeralCredentials(s *terraform.State) error {
	for name, rs := range s.RootModule().Resources {
		if rs.Type == "minio_sts_credentials" {
			return fmt.Errorf("%s is in the state: an ephemeral resource must not be persisted", name)
		}
		for attribute, value := range rs.Primary.Attributes {
			if value == "" {
				continue
			}
			if strings.Contains(attribute, "session_token") || strings.Contains(attribute, "secret_key") {
				return fmt.Errorf("%s holds a credential in %s", name, attribute)
			}
		}
	}
	return nil
}
