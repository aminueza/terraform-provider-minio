package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func testAccPreCheckKMSKey(t *testing.T) {
	t.Helper()
	testAccPreCheckKMS(t)

	endpoint := os.Getenv("KMS_MINIO_ENDPOINT")
	user := os.Getenv("KMS_MINIO_USER")
	pass := os.Getenv("KMS_MINIO_PASSWORD")
	if endpoint == "" || user == "" || pass == "" {
		t.Skip("KMS_MINIO_ENDPOINT/USER/PASSWORD not set; skipping KMS key tests")
	}

	admin, err := madmin.NewWithOptions(endpoint, &madmin.Options{
		Creds:  credentials.NewStaticV4(user, pass, ""),
		Secure: os.Getenv("KMS_MINIO_ENABLE_HTTPS") == "true",
	})
	if err != nil {
		t.Fatalf("creating KMS admin client: %v", err)
	}

	testKey := fmt.Sprintf("tfacc-precheck-%d", acctest.RandInt())
	if err := admin.CreateKey(context.Background(), testKey); err != nil {
		if strings.Contains(err.Error(), "not supported") {
			t.Skip("CreateKey is not supported (static KEK mode); skipping KMS key tests")
		}
	} else {
		_ = admin.DeleteKey(context.Background(), testKey)
	}
}

func TestAccMinioKMSKey_basic(t *testing.T) {
	keyID := fmt.Sprintf("tfacc-kms-key-%d", acctest.RandInt())
	resourceName := "minio_kms_key.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckKMSKey(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioKMSKeyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioKMSKeyConfig(keyID),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioKMSKeyExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key_id", keyID),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckMinioKMSKeyDestroy(s *terraform.State) error {
	conn := testAccKmsProvider.Meta().(*S3MinioClient).S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_kms_key" {
			continue
		}

		_, err := conn.GetKeyStatus(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("KMS key %s still exists", rs.Primary.ID)
		}
	}

	return nil
}

func testAccCheckMinioKMSKeyExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no KMS key ID is set")
		}

		conn := testAccKmsProvider.Meta().(*S3MinioClient).S3Admin

		status, err := conn.GetKeyStatus(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error reading KMS key: %w", err)
		}

		if status.KeyID != rs.Primary.ID {
			return fmt.Errorf("KMS key not found")
		}

		return nil
	}
}

func testAccMinioKMSKeyConfig(keyID string) string {
	return fmt.Sprintf(`
resource "minio_kms_key" "test" {
  provider = "kmsminio"
  key_id   = "%s"
}
`, keyID)
}

// TestMinioDeleteKMSKey_externalBackendErrors verifies that DeleteKey errors from
// external KMS backends (Vault, etc.) are treated as success so that
// terraform destroy doesn't fail when keys cannot be deleted via the MinIO API.
func TestMinioDeleteKMSKey_externalBackendErrors(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		body       string
		wantErr    bool
	}{
		{
			name:       "NotImplemented error code clears resource",
			statusCode: http.StatusNotImplemented,
			body:       `{"Code":"NotImplemented","Message":"key deletion is not supported by this KMS"}`,
			wantErr:    false,
		},
		{
			name:       "not supported in message clears resource",
			statusCode: http.StatusBadRequest,
			body:       `{"Code":"XMinioKMSError","Message":"operation not supported by external KMS backend"}`,
			wantErr:    false,
		},
		{
			name:       "not implemented in message clears resource",
			statusCode: http.StatusInternalServerError,
			body:       `{"Code":"XMinioError","Message":"key deletion not implemented for this backend"}`,
			wantErr:    false,
		},
		{
			name:       "unrelated error propagates",
			statusCode: http.StatusForbidden,
			body:       `{"Code":"AccessDenied","Message":"access denied"}`,
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()

			host := strings.TrimPrefix(srv.URL, "http://")
			adminClient, err := madmin.NewWithOptions(host, &madmin.Options{
				Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
				Secure: false,
			})
			if err != nil {
				t.Fatalf("creating admin client: %v", err)
			}

			d := schema.TestResourceDataRaw(t, resourceMinioKMSKey().Schema, map[string]interface{}{
				"key_id": "test-key",
			})
			d.SetId("test-key")

			meta := &S3MinioClient{S3Admin: adminClient}
			diags := minioDeleteKMSKey(context.Background(), d, meta)

			if tc.wantErr {
				if len(diags) == 0 {
					t.Error("expected error diagnostics but got none")
				}
				if d.Id() == "" {
					t.Error("expected resource ID to remain set on real error")
				}
			} else {
				if len(diags) > 0 {
					t.Errorf("expected no error but got: %v", diags)
				}
				if d.Id() != "" {
					t.Errorf("expected resource ID to be cleared, got %q", d.Id())
				}
			}
		})
	}
}
