package minio

import (
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var retryTuningAttributes = []struct {
	attribute   string
	envKey      string
	builtIn     int
	envValue    int
	configValue int
}{
	{"request_timeout_seconds", "MINIO_REQUEST_TIMEOUT_SECONDS", 30, 45, 12},
	{"max_retries", "MINIO_MAX_RETRIES", 6, 9, 2},
	{"retry_delay_ms", "MINIO_RETRY_DELAY_MS", 1000, 250, 75},
}

func retryTuning(config *S3MinioConfig) map[string]int {
	return map[string]int{
		"request_timeout_seconds": config.RequestTimeoutSeconds,
		"max_retries":             config.MaxRetries,
		"retry_delay_ms":          config.RetryDelayMs,
	}
}

func clearRetryTuningEnvironment(t *testing.T) {
	t.Helper()
	for _, tc := range retryTuningAttributes {
		t.Setenv(tc.envKey, "")
	}
}

func TestNewConfigUsesTheBuiltInRetryDefaults(t *testing.T) {
	clearRetryTuningEnvironment(t)

	config := NewConfig(schema.TestResourceDataRaw(t, Provider().Schema, map[string]interface{}{}))

	for _, tc := range retryTuningAttributes {
		if got := retryTuning(config)[tc.attribute]; got != tc.builtIn {
			t.Errorf("%s = %d, want the built-in default %d", tc.attribute, got, tc.builtIn)
		}
	}
}

func TestNewConfigReadsTheRetryEnvironmentVariables(t *testing.T) {
	for _, tc := range retryTuningAttributes {
		t.Setenv(tc.envKey, strconv.Itoa(tc.envValue))
	}

	config := NewConfig(schema.TestResourceDataRaw(t, Provider().Schema, map[string]interface{}{}))

	for _, tc := range retryTuningAttributes {
		if got := retryTuning(config)[tc.attribute]; got != tc.envValue {
			t.Errorf("%s = %d, want %d from %s", tc.attribute, got, tc.envValue, tc.envKey)
		}
	}
}

func TestNewConfigPrefersTheConfigurationOverTheRetryEnvironment(t *testing.T) {
	raw := map[string]interface{}{}
	for _, tc := range retryTuningAttributes {
		t.Setenv(tc.envKey, strconv.Itoa(tc.envValue))
		raw[tc.attribute] = tc.configValue
	}

	config := NewConfig(schema.TestResourceDataRaw(t, Provider().Schema, raw))

	for _, tc := range retryTuningAttributes {
		if got := retryTuning(config)[tc.attribute]; got != tc.configValue {
			t.Errorf("%s = %d, want the configured value %d", tc.attribute, got, tc.configValue)
		}
	}
}

func TestProviderRetryAttributesDeclareNoStaticDefault(t *testing.T) {
	for _, tc := range retryTuningAttributes {
		attribute := Provider().Schema[tc.attribute]
		if attribute.Default != nil {
			t.Errorf("%s declares Default %v: DefaultValue returns Default without ever calling DefaultFunc, so %s would be ignored", tc.attribute, attribute.Default, tc.envKey)
		}
		if attribute.DefaultFunc == nil {
			t.Errorf("%s declares no DefaultFunc, so %s cannot be read", tc.attribute, tc.envKey)
		}
	}
}
