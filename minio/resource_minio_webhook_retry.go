package minio

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// The audit and logger webhooks share MinIO's retry and timeout keys. MinIO
// only enforces their limits on an enabled target, so the plan checks them
// either way (internal/logger/config.go): max_retry not negative,
// retry_interval at most 1m, http_timeout at least 1s. See issue #1370.

// webhookRetryFields returns the three shared fields. max_retry is plain
// Optional: MinIO seeds 0 into every stored target, which equals the zero
// value, so no Computed is needed and removal is a native diff. retry_interval
// and http_timeout are Optional and Computed because MinIO seeds their
// non-zero defaults (3s and 5s) into every stored target; their removal is
// invisible to the diff, so customizeDiffWebhookRetry plans them back to the
// default and the builders write it.
func webhookRetryFields() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"max_retry": {
			Type:         schema.TypeInt,
			Optional:     true,
			ValidateFunc: validation.IntAtLeast(0),
			Description:  "Retries before an undelivered batch is dropped; `0` retries forever. First available in MinIO RELEASE.2024-09-09T16-59-28Z.",
		},
		"retry_interval": {
			Type:         schema.TypeString,
			Optional:     true,
			Computed:     true,
			ValidateFunc: validateWebhookRetryInterval,
			Description:  "Pause between retries, at most `1m`; MinIO seeds `3s`. First available in MinIO RELEASE.2024-09-09T16-59-28Z.",
		},
		"http_timeout": {
			Type:         schema.TypeString,
			Optional:     true,
			Computed:     true,
			ValidateFunc: validateWebhookHTTPTimeout,
			Description:  "Timeout for each request, at least `1s`; MinIO seeds `5s`. First available in MinIO RELEASE.2024-09-13T20-26-02Z.",
		},
	}
}

// addWebhookRetryFields copies the retry fields into specific without
// mergeSchemas' validateConfigValue pass. The caller runs mergeSchemas over
// the result, which adds the validator exactly once; nesting two mergeSchemas
// calls would wrap every string validator twice and report a bad value twice.
func addWebhookRetryFields(specific map[string]*schema.Schema) map[string]*schema.Schema {
	for k, v := range webhookRetryFields() {
		specific[k] = v
	}
	return specific
}

func validateWebhookRetryInterval(val interface{}, key string) (warns []string, errs []error) {
	v := val.(string)
	dur, err := time.ParseDuration(v)
	if err != nil {
		errs = append(errs, fmt.Errorf("%q must be a golang duration like '10s', got %q", key, v))
		return
	}
	if dur > time.Minute {
		errs = append(errs, fmt.Errorf("%q has a maximum of '1m', got %q", key, v))
	}
	return
}

func validateWebhookHTTPTimeout(val interface{}, key string) (warns []string, errs []error) {
	v := val.(string)
	dur, err := time.ParseDuration(v)
	if err != nil {
		errs = append(errs, fmt.Errorf("%q must be a golang duration like '5s', got %q", key, v))
		return
	}
	if dur < time.Second {
		errs = append(errs, fmt.Errorf("%q has a minimum of '1s', got %q", key, v))
	}
	return
}

// customizeDiffWebhookRetry covers the removals the diff cannot see:
// retry_interval and http_timeout are Optional and Computed (MinIO seeds
// their 3s and 5s defaults into every stored target), so removing one from
// the configuration keeps the old value in the diff and no update would run.
// The raw config sees the removal, and SetNew plans the MinIO default so the
// update runs and the builder writes the default back; an empty value would
// fail to parse once the target is enabled.
func customizeDiffWebhookRetry(_ context.Context, d *schema.ResourceDiff, _ interface{}) error {
	raw := d.GetRawConfig()
	if raw.IsNull() {
		return nil
	}
	if raw.GetAttr("retry_interval").IsNull() && d.Get("retry_interval").(string) != "" {
		if err := d.SetNew("retry_interval", "3s"); err != nil {
			return err
		}
	}
	if raw.GetAttr("http_timeout").IsNull() && d.Get("http_timeout").(string) != "" {
		if err := d.SetNew("http_timeout", "5s"); err != nil {
			return err
		}
	}
	return nil
}

// buildWebhookRetryCfg adds the retry and timeout keys the configuration or
// state holds. A removed max_retry goes back to 0 (notifyBuildCfgAddInt-style
// skipping would leave the old value on the server, and an empty value fails
// to parse); a removed retry_interval or http_timeout reads back as the
// default that customizeDiffWebhookRetry planned, so sending the current
// value writes the default back.
func buildWebhookRetryCfg(parts *[]string, d *schema.ResourceData) {
	if mr := d.Get("max_retry").(int); mr > 0 {
		*parts = append(*parts, fmt.Sprintf("max_retry=%d", mr))
	} else if d.HasChange("max_retry") {
		*parts = append(*parts, "max_retry=0")
	}
	for _, key := range []string{"retry_interval", "http_timeout"} {
		if v := d.Get(key).(string); v != "" {
			*parts = append(*parts, buildConfigParam(key, v))
		}
	}
}

// readWebhookRetryFields stores the retry and timeout keys the server
// reports. Older servers do not have them and their absence leaves the state
// untouched; a value that does not parse is skipped, as notifySetInt does.
func readWebhookRetryFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if v, ok := cfgMap["max_retry"]; ok {
		if n, err := parseInt(v); err == nil {
			if setErr := d.Set("max_retry", n); setErr != nil {
				return NewResourceError("setting max_retry", d.Id(), setErr)
			}
		}
	}
	for _, key := range []string{"retry_interval", "http_timeout"} {
		if v, ok := cfgMap[key]; ok {
			if setErr := d.Set(key, v); setErr != nil {
				return NewResourceError("setting "+key, d.Id(), setErr)
			}
		}
	}
	return nil
}
