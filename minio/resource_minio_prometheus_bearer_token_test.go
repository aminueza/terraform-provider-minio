package minio

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestPrometheusBearerTokenLimitValidation(t *testing.T) {
	validate := resourceMinioPrometheusBearerToken().Schema["limit"].ValidateFunc

	cases := []struct {
		value   int
		wantErr bool
	}{
		{0, true},
		{1, false},
		{2562047, false},
		{2562048, true},
		{3000000, true},
	}

	for _, tc := range cases {
		_, errs := validate(tc.value, "limit")
		if tc.wantErr && len(errs) == 0 {
			t.Errorf("limit = %d: expected an error, got none", tc.value)
		}
		if !tc.wantErr && len(errs) != 0 {
			t.Errorf("limit = %d: expected no error, got %v", tc.value, errs)
		}
		if tc.wantErr && len(errs) != 0 && !strings.Contains(errs[0].Error(), "2562047") {
			t.Errorf("limit = %d: expected the error to name 2562047, got %v", tc.value, errs[0])
		}
	}
}

func TestPrometheusBearerTokenExpiresInValidation(t *testing.T) {
	validate := resourceMinioPrometheusBearerToken().Schema["expires_in"].ValidateFunc

	cases := []struct {
		value   string
		wantErr bool
	}{
		{"0h", true},
		{"00h", true},
		{"1h", false},
		{"24h", false},
		{"87600h", false},
		{"2562047h", false},
		{"2562048h", true},
		{"99999999999999999999h", true},
		{"24h30m", true},
		{"30m", true},
		{"", true},
	}

	for _, tc := range cases {
		_, errs := validate(tc.value, "expires_in")
		if tc.wantErr && len(errs) == 0 {
			t.Errorf("expires_in = %q: expected an error, got none", tc.value)
		}
		if !tc.wantErr && len(errs) != 0 {
			t.Errorf("expires_in = %q: expected no error, got %v", tc.value, errs)
		}
	}
}

func TestAccMinioPrometheusBearerToken_limitBounds(t *testing.T) {
	name := "test_" + acctest.RandString(8)
	resourceName := "minio_prometheus_bearer_token." + name

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccMinioPrometheusBearerTokenLimit(name, "cluster", "24h", 3000000),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`expected limit to be in the range \(1 - 2562047\), got 3000000`),
			},
			{
				Config: testAccMinioPrometheusBearerTokenLimit(name, "cluster", "24h", 2562047),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusBearerTokenExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "limit", "2562047"),
					resource.TestCheckResourceAttr(resourceName, "expires_in", "24h"),
					testAccCheckMinioPrometheusBearerTokenExpiresAround(resourceName, 24*time.Hour),
				),
			},
		},
	})
}

func TestAccMinioPrometheusBearerToken_expiresInBounds(t *testing.T) {
	name := "test_" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccMinioPrometheusBearerTokenNamed(name, "node", "0h"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`expires_in must be greater than 0h, got: 0h`),
			},
			{
				Config:      testAccMinioPrometheusBearerTokenNamed(name, "node", "2562048h"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`expires_in must be at most 2562047h, got: 2562048h`),
			},
		},
	})
}

// Not parallel: the test moves the package-level clock that Read uses.
func TestAccMinioPrometheusBearerToken_expiredTokenReplaced(t *testing.T) {
	name := "test_" + acctest.RandString(8)
	resourceName := "minio_prometheus_bearer_token." + name
	var firstToken string

	t.Cleanup(func() { prometheusTokenNow = time.Now })

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPrometheusBearerTokenNamed(name, "bucket", "24h"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusBearerTokenExists(resourceName),
					testAccCaptureMinioPrometheusBearerToken(resourceName, &firstToken),
				),
			},
			{
				PreConfig: func() {
					prometheusTokenNow = func() time.Time { return time.Now().Add(25 * time.Hour) }
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check:              testAccCheckMinioPrometheusBearerTokenGone(resourceName),
			},
			{
				PreConfig: func() { prometheusTokenNow = time.Now },
				Config:    testAccMinioPrometheusBearerTokenNamed(name, "bucket", "24h"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusBearerTokenExists(resourceName),
					testAccCheckMinioPrometheusBearerTokenChanged(resourceName, &firstToken),
					testAccCheckMinioPrometheusBearerTokenExpiresAround(resourceName, 24*time.Hour),
				),
			},
		},
	})
}

func testAccCaptureMinioPrometheusBearerToken(resourceName string, token *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		*token = rs.Primary.Attributes["token"]
		if *token == "" {
			return fmt.Errorf("%s has no token", resourceName)
		}
		return nil
	}
}

func testAccCheckMinioPrometheusBearerTokenChanged(resourceName string, previous *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		if rs.Primary.Attributes["token"] == *previous {
			return fmt.Errorf("%s still holds the expired token", resourceName)
		}
		return nil
	}
}

func testAccCheckMinioPrometheusBearerTokenGone(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if _, ok := s.RootModule().Resources[resourceName]; ok {
			return fmt.Errorf("%s is still in state after its token expired", resourceName)
		}
		return nil
	}
}

func testAccCheckMinioPrometheusBearerTokenExpiresAround(resourceName string, want time.Duration) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		expiry, err := time.Parse(time.RFC3339, rs.Primary.Attributes["token_expiry"])
		if err != nil {
			return fmt.Errorf("parsing token_expiry %q: %w", rs.Primary.Attributes["token_expiry"], err)
		}

		if drift := time.Until(expiry) - want; drift > time.Minute || drift < -time.Minute {
			return fmt.Errorf("token_expiry %s is not within a minute of now plus %s", expiry, want)
		}

		parts := strings.Split(rs.Primary.Attributes["token"], ".")
		if len(parts) != 3 {
			return fmt.Errorf("token is not a JWT: %d parts", len(parts))
		}

		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return fmt.Errorf("decoding JWT payload: %w", err)
		}

		var claims map[string]interface{}
		if err := json.Unmarshal(payload, &claims); err != nil {
			return fmt.Errorf("parsing JWT payload: %w", err)
		}

		exp, ok := claims["exp"].(float64)
		if !ok {
			return fmt.Errorf("JWT payload has no numeric exp: %s", payload)
		}

		if diff := int64(exp) - expiry.Unix(); diff > 1 || diff < -1 {
			return fmt.Errorf("JWT exp %d does not match token_expiry %d", int64(exp), expiry.Unix())
		}

		return nil
	}
}

func testAccMinioPrometheusBearerTokenNamed(name, metricType, expiresIn string) string {
	return fmt.Sprintf(`
resource "minio_prometheus_bearer_token" %q {
  metric_type = %q
  expires_in  = %q
}
`, name, metricType, expiresIn)
}

func testAccMinioPrometheusBearerTokenLimit(name, metricType, expiresIn string, limit int) string {
	return fmt.Sprintf(`
resource "minio_prometheus_bearer_token" %q {
  metric_type = %q
  expires_in  = %q
  limit       = %d
}
`, name, metricType, expiresIn, limit)
}

func TestAccMinioPrometheusBearerToken_basic(t *testing.T) {
	resourceName := "minio_prometheus_bearer_token.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPrometheusBearerTokenBasic("cluster"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusBearerTokenExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "metric_type", "cluster"),
					resource.TestCheckResourceAttr(resourceName, "expires_in", "87600h"),
					resource.TestCheckResourceAttrSet(resourceName, "token"),
					resource.TestCheckResourceAttrSet(resourceName, "token_expiry"),
				),
			},
			{
				ResourceName: resourceName,
				ImportState:  true,
				ExpectError:  regexp.MustCompile(`doesn't support import`),
			},
		},
	})
}

func TestAccMinioPrometheusBearerToken_allMetricTypes(t *testing.T) {
	metricTypes := []string{"cluster", "node", "bucket", "resource"}

	for _, metricType := range metricTypes {
		t.Run(metricType, func(t *testing.T) {
			resourceName := "minio_prometheus_bearer_token.test"

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:          func() { testAccPreCheck(t) },
				ProviderFactories: testAccProviders,
				Steps: []resource.TestStep{
					{
						Config: testAccMinioPrometheusBearerTokenBasic(metricType),
						Check: resource.ComposeTestCheckFunc(
							testAccCheckMinioPrometheusBearerTokenExists(resourceName),
							resource.TestCheckResourceAttr(resourceName, "metric_type", metricType),
							resource.TestCheckResourceAttrSet(resourceName, "token"),
						),
					},
				},
			})
		})
	}
}

func TestAccMinioPrometheusBearerToken_update(t *testing.T) {
	resourceName := "minio_prometheus_bearer_token.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPrometheusBearerTokenBasic("cluster"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusBearerTokenExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "expires_in", "87600h"),
				),
			},
			{
				Config: testAccMinioPrometheusBearerTokenExpiresIn("cluster", "24h"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioPrometheusBearerTokenExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "expires_in", "24h"),
				),
			},
		},
	})
}

func testAccCheckMinioPrometheusBearerTokenExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no bearer token ID is set")
		}

		return nil
	}
}

func testAccMinioPrometheusBearerTokenBasic(metricType string) string {
	return fmt.Sprintf(`
resource "minio_prometheus_bearer_token" "test" {
  metric_type = %q
  expires_in  = "87600h"
}
`, metricType)
}

func testAccMinioPrometheusBearerTokenExpiresIn(metricType, expiresIn string) string {
	return fmt.Sprintf(`
resource "minio_prometheus_bearer_token" "test" {
  metric_type = %q
  expires_in  = %q
}
`, metricType, expiresIn)
}
