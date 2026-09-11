package minio

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

func TestEnvDefaultIntRejectsAValueThatIsNotAWholeNumber(t *testing.T) {
	for _, raw := range []string{"abc", " 9", "1.5", ""} {
		t.Setenv("MINIO_PROBE_INT", raw)

		value, err := envDefaultInt("MINIO_PROBE_INT", 7)()

		if raw == "" {
			if err != nil || value != 7 {
				t.Errorf("an unset variable gave (%v, %v), want the fallback 7 and no error", value, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%q gave %v, want an error", raw, value)
			continue
		}
		if !strings.Contains(err.Error(), "MINIO_PROBE_INT") || !strings.Contains(err.Error(), raw) {
			t.Errorf("the error for %q is %q: it must name the variable and the value so the operator can find the typo", raw, err)
		}
	}
}

func TestEnvDefaultIntSaysWhenAWholeNumberIsOutOfRange(t *testing.T) {
	t.Setenv("MINIO_PROBE_INT", "99999999999999999999")

	_, err := envDefaultInt("MINIO_PROBE_INT", 7)()
	if err == nil {
		t.Fatal("a value beyond the integer range must be reported")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("the error is %q: the value is a whole number, so the message must say the range is the problem", err)
	}
}

func TestEnvDefaultIntReadsAWholeNumber(t *testing.T) {
	t.Setenv("MINIO_PROBE_INT", "-3")

	value, err := envDefaultInt("MINIO_PROBE_INT", 7)()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if value != -3 {
		t.Errorf("value = %v, want -3: the fallback for a non-positive value belongs to the consumer, not to the parser", value)
	}
}

func TestNonPositiveRetryTuningFallsBackForBothHalves(t *testing.T) {
	clearRetryTuningEnvironment(t)

	sdkConfig := NewConfig(schema.TestResourceDataRaw(t, Provider().Schema, map[string]interface{}{
		"request_timeout_seconds": 0,
		"max_retries":             0,
		"retry_delay_ms":          0,
	}))

	model := nullFrameworkModel()
	model.RequestTimeoutSeconds = types.Int64Value(0)
	model.MaxRetries = types.Int64Value(0)
	model.RetryDelayMs = types.Int64Value(0)

	var diags diag.Diagnostics
	frameworkConfigured := frameworkConfig(context.Background(), model, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if sdkConfig.RequestTimeoutSeconds == frameworkConfigured.RequestTimeoutSeconds {
		t.Fatalf("both halves resolved request_timeout_seconds to %d: this test exists because they disagree, GetOk replacing 0 on the SDKv2 side only", sdkConfig.RequestTimeoutSeconds)
	}

	for _, tc := range []struct {
		half   string
		config *S3MinioConfig
	}{
		{"SDKv2", sdkConfig},
		{"framework", frameworkConfigured},
	} {
		transport, err := tc.config.customTransport(context.Background())
		if err != nil {
			t.Fatalf("building the %s transport: %s", tc.half, err)
		}
		if transport.ResponseHeaderTimeout != defaultRequestTimeoutSeconds*time.Second {
			t.Errorf("%s half times out after %v, want the default %ds", tc.half, transport.ResponseHeaderTimeout, defaultRequestTimeoutSeconds)
		}

		retry := getRetryConfig(&S3MinioClient{MaxRetries: tc.config.MaxRetries, RetryDelayMs: tc.config.RetryDelayMs})
		if retry.MaxRetries != defaultMaxRetries {
			t.Errorf("%s half retries %d times, want the default %d", tc.half, retry.MaxRetries, defaultMaxRetries)
		}
		if retry.MaxBackoff != time.Duration(defaultRetryDelayMs*20)*time.Millisecond {
			t.Errorf("%s half caps the backoff at %v, want the default", tc.half, retry.MaxBackoff)
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
