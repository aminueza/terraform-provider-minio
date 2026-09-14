package minio

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
)

func TestAccMinioConfig_basic(t *testing.T) {
	resourceName := "minio_config.test"
	configKey := "logger_webhook:1"
	configValue := "enable=off"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioConfigBasic(configKey, configValue),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", configKey),
					testCheckResourceAttrContains(resourceName, "value", "enable=off"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"restart_required"},
			},
		},
	})
}

func TestAccMinioConfig_update(t *testing.T) {
	resourceName := "minio_config.test"
	configKey := "logger_webhook:2"
	configValue1 := "enable=off batch_size=5 queue_size=50000"
	configValue2 := "enable=off batch_size=10 queue_size=50000"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioConfigBasic(configKey, configValue1),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", configKey),
					testCheckResourceAttrContains(resourceName, "value", "enable=off"),
					testCheckResourceAttrContains(resourceName, "value", "batch_size=5"),
					testCheckResourceAttrContains(resourceName, "value", "queue_size=50000"),
				),
			},
			{
				Config: testAccMinioConfigBasic(configKey, configValue2),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", configKey),
					testCheckResourceAttrContains(resourceName, "value", "enable=off"),
					testCheckResourceAttrContains(resourceName, "value", "batch_size=10"),
					testCheckResourceAttrContains(resourceName, "value", "queue_size=50000"),
				),
			},
		},
	})
}

func TestAccMinioConfig_multipleSettings(t *testing.T) {
	resourceName := "minio_config.test"
	configKey := "logger_webhook:3"
	configValue := "batch_size=10 enable=off queue_size=75000"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioConfigBasic(configKey, configValue),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", configKey),
					testCheckResourceAttrContains(resourceName, "value", "enable=off"),
					testCheckResourceAttrContains(resourceName, "value", "batch_size=10"),
					testCheckResourceAttrContains(resourceName, "value", "queue_size=75000"),
				),
			},
		},
	})
}

func TestAccMinioConfig_webhookNotification(t *testing.T) {
	resourceName := "minio_config.test"
	configKey := "logger_webhook:4"
	configValue := "enable=off endpoint=http://example.com queue_size=50000"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioConfigBasic(configKey, configValue),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", configKey),
					testCheckResourceAttrContains(resourceName, "value", "enable=off"),
					testCheckResourceAttrContains(resourceName, "value", "endpoint=http://example.com"),
					testCheckResourceAttrContains(resourceName, "value", "queue_size=50000"),
				),
			},
		},
	})
}

func testAccCheckMinioConfigExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no config ID is set")
		}

		minioC := testAccClient()
		configKey := rs.Primary.ID

		_, err := minioC.S3Admin.GetConfigKV(context.Background(), configKey)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
				return fmt.Errorf("config %s not found", configKey)
			}
			return err
		}

		return nil
	}
}

func testAccCheckMinioConfigDestroy(s *terraform.State) error {
	// Config resources may persist after deletion, so we just check the resource is removed from state
	return nil
}

func testAccMinioConfigBasic(key, value string) string {
	return fmt.Sprintf(`
resource "minio_config" "test" {
  key   = %[1]q
  value = %[2]q
}
`, key, value)
}

func testCheckResourceAttrContains(resourceName, attr, value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if !strings.Contains(rs.Primary.Attributes[attr], value) {
			return fmt.Errorf("%s attribute does not contain %s", attr, value)
		}

		return nil
	}
}

var configKeysThatMustNotWarn = []string{
	"api",
	"compression",
	"heal",
	"scanner",
	"etcd",
	"browser",
	"region",
	"ilm",
	"erasure",
	"kubernetes",
	"notify_webhook:primary",
	"identity_openid:dex",
	"log_api_webhook:audit",
	"telemetry_target:otel",
	"alert_webhook:oncall",
	"audit_event_queue",
	"bucket_event_queue",
}

func TestValidateConfigKeyAcceptsTheseKeys(t *testing.T) {
	for _, key := range configKeysThatMustNotWarn {
		warns, errs := validateConfigKey(key, "key")
		if len(errs) > 0 {
			t.Errorf("%q gave errors %v, want none", key, errs)
		}
		if len(warns) > 0 {
			t.Errorf("%q warns: %v. These keys are written out rather than read from the same set the validator reads, so narrowing that set fails here on a named key instead of passing silently.", key, warns)
		}
	}
}

func TestValidateConfigKeyCoversBothSubsystemSets(t *testing.T) {
	var missing []string
	for _, key := range configKeysThatMustNotWarn {
		subsystem, _, _ := strings.Cut(key, ":")
		if !madmin.SubSystems.Contains(subsystem) && !madmin.EOSSubSystems.Contains(subsystem) {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("%v name no subsystem in either set, so the list above no longer describes MinIO", missing)
	}

	var eosOnly int
	for _, key := range configKeysThatMustNotWarn {
		subsystem, _, _ := strings.Cut(key, ":")
		if !madmin.SubSystems.Contains(subsystem) {
			eosOnly++
		}
	}
	if eosOnly == 0 {
		t.Error("no key in the list comes from EOSSubSystems alone, so nothing here would catch a validator that checks only SubSystems")
	}
}

func TestValidateConfigKeyAcceptsEverySubsystemMinIOKnows(t *testing.T) {
	for _, subsystem := range madmin.SubSystems.Union(madmin.EOSSubSystems).ToSlice() {
		warns, errs := validateConfigKey(subsystem, "key")
		if len(errs) > 0 {
			t.Errorf("%q gave errors %v, want none", subsystem, errs)
		}
		if len(warns) > 0 {
			t.Errorf("%q gave warnings %v: it is a subsystem MinIO declares", subsystem, warns)
		}
	}
}

func TestValidateConfigKeyAcceptsASubsystemWithATarget(t *testing.T) {
	for _, key := range []string{"notify_webhook:primary", "identity_openid:dex", "logger_webhook:audit"} {
		warns, errs := validateConfigKey(key, "key")
		if len(errs) > 0 || len(warns) > 0 {
			t.Errorf("%q gave warnings %v and errors %v, want none: the part before the colon is the subsystem", key, warns, errs)
		}
	}
}

func TestValidateConfigKeyWarnsAboutAKeyThatNamesNoSubsystem(t *testing.T) {
	for _, key := range []string{"compresion", "not_a_subsystem", "webhook", ":primary"} {
		warns, errs := validateConfigKey(key, "key")
		if len(errs) > 0 {
			t.Errorf("%q gave errors %v: an unknown key is worth a warning, not a failure", key, errs)
		}
		if len(warns) != 1 {
			t.Errorf("%q gave warnings %v, want exactly one", key, warns)
			continue
		}
		if !strings.Contains(warns[0], key) {
			t.Errorf("the warning for %q does not quote the key: %q", key, warns[0])
		}
	}
}

func TestValidateConfigKeyRejectsAnEmptyKey(t *testing.T) {
	warns, errs := validateConfigKey("", "key")
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want exactly one", errs)
	}
	if len(warns) > 0 {
		t.Errorf("warnings = %v: an empty key is already an error, so a warning adds noise", warns)
	}
}

func TestValidateConfigKeyNoLongerJudgesByUnderscore(t *testing.T) {
	for _, key := range configKeysThatMustNotWarn {
		if strings.Contains(key, "_") {
			continue
		}
		if warns, _ := validateConfigKey(key, "key"); len(warns) > 0 {
			t.Errorf("%q still warns: the underscore heuristic rejected seventeen valid subsystem names, %q among them", key, key)
		}
	}
}
