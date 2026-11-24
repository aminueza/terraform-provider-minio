package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/set"
)

func resourceMinioConfig() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateConfig,
		ReadContext:   minioReadConfig,
		UpdateContext: minioUpdateConfig,
		DeleteContext: minioDeleteConfig,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Read:   schema.DefaultTimeout(2 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},
		Schema: map[string]*schema.Schema{
			"key": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "The configuration key (e.g., 'api', 'notify_webhook:1', 'region')",
				ValidateFunc: validateConfigKey,
			},
			"value": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The configuration value in key=value format (e.g., 'requests_max=1000'). For multiple settings, separate with spaces.",
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					v := val.(string)
					if v == "" {
						errs = append(errs, fmt.Errorf("%q cannot be empty", key))
					}
					return
				},
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// Parse the user's config (new) into a map
					userParams := parseConfigParams(new)
					// Parse the server's response (old) into a map
					serverParams := parseConfigParams(old)

					for key, userValue := range userParams {
						serverValue, exists := serverParams[key]
						if !exists || serverValue != userValue {
							return false
						}
					}
					return true
				},
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Indicates whether a server restart is required for the configuration to take effect",
			},
		},
	}
}

func minioCreateConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	key := d.Get("key").(string)
	value := d.Get("value").(string)

	tflog.Info(ctx, fmt.Sprintf("Creating/Setting MinIO config: %s", key))

	timeout := d.Timeout(schema.TimeoutCreate)
	var restartRequired bool
	var err error

	configString := fmt.Sprintf("%s %s", key, value)
	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		restart, err := client.S3Admin.SetConfigKV(ctx, configString)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error setting config %s: %w", key, err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to set config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("Failed to set config %s after retries: %s", key, err))
		return NewResourceError("setting config", key, err)
	}

	d.SetId(key)
	if diags := setResourceField(d, "restart_required", restartRequired, key); diags != nil {
		return diags
	}

	if restartRequired {
		tflog.Warn(ctx, fmt.Sprintf("Config change for %s requires MinIO server restart to take effect", key))
	}

	return minioReadConfig(ctx, d, meta)
}

func minioReadConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	key := d.Id()

	tflog.Info(ctx, fmt.Sprintf("Reading MinIO config: %s", key))

	timeout := d.Timeout(schema.TimeoutRead)
	var configData []byte
	var err error

	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		configData, err = client.S3Admin.GetConfigKV(ctx, key)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
				tflog.Warn(ctx, fmt.Sprintf("Config %s no longer exists", key))
				d.SetId("")
				return nil
			}

			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("error reading config %s: %w", key, err))
			}

			return retry.NonRetryableError(fmt.Errorf("failed to get config: %w", err))
		}
		return nil
	})

	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("Failed to read config %s after retries: %s", key, err))
		return NewResourceError("reading config", key, err)
	}

	if d.Id() == "" {
		return nil
	}

	configStr := strings.TrimSpace(string(configData))
	tflog.Debug(ctx, fmt.Sprintf("Raw config data for key %s: %s", key, configStr))

	valueToSet := configStr
	if strings.HasPrefix(configStr, key+" ") {
		// Format: "key subsys:target key1=value1 key2=value2" -> "key1=value1 key2=value2"
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			fullValue := strings.TrimSpace(parts[1])
			tflog.Debug(ctx, fmt.Sprintf("Setting full value from MinIO: %s", fullValue))
			valueToSet = fullValue
		}
	}
	if diags := setResourceField(d, "value", valueToSet, key); diags != nil {
		return diags
	}

	return setResourceField(d, "key", key, key)
}

func minioUpdateConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	key := d.Get("key").(string)
	value := d.Get("value").(string)

	tflog.Info(ctx, fmt.Sprintf("Updating MinIO config: %s", key))

	timeout := d.Timeout(schema.TimeoutUpdate)
	var restartRequired bool
	var err error

	configString := fmt.Sprintf("%s %s", key, value)
	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		restart, err := client.S3Admin.SetConfigKV(ctx, configString)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error updating config %s: %w", key, err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to set config: %w", err))
		}
		if restart {
			restartRequired = true
		}
		return nil
	})

	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("Failed to update config %s after retries: %s", key, err))
		return NewResourceError("updating config", key, err)
	}

	if diags := setResourceField(d, "restart_required", restartRequired, key); diags != nil {
		return diags
	}

	if restartRequired {
		tflog.Warn(ctx, fmt.Sprintf("Config change for %s requires MinIO server restart to take effect", key))
	}

	return minioReadConfig(ctx, d, meta)
}

func minioDeleteConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	key := d.Id()

	tflog.Info(ctx, fmt.Sprintf("Deleting MinIO config: %s", key))

	_, err := client.S3Admin.GetConfigKV(ctx, key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
			tflog.Warn(ctx, fmt.Sprintf("Config %s no longer exists, removing from state", key))
			d.SetId("")
			return nil
		}
		return NewResourceError("checking config before deletion", key, err)
	}

	timeout := d.Timeout(schema.TimeoutDelete)
	var restart bool

	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		restart, err = client.S3Admin.DelConfigKV(ctx, key)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error deleting config %s: %w", key, err))
			}

			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
				return nil
			}

			return retry.NonRetryableError(fmt.Errorf("failed to delete config: %w", err))
		}
		return nil
	})

	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("Failed to delete config %s after retries: %s", key, err))
		return NewResourceError("deleting config", key, err)
	}

	if restart {
		tflog.Warn(ctx, fmt.Sprintf("Config deletion for %s requires MinIO server restart to take effect", key))
	}

	d.SetId("")
	return nil
}

// parseConfigParams parses a space-separated key=value string into a map.
// MinIO prints a value that contains whitespace in double quotes, for example
// `comment="primary webhook"`, so a quoted value is read up to its closing
// quote.
func parseConfigParams(configStr string) map[string]string {
	params := make(map[string]string)
	text := strings.TrimSpace(configStr)

	for text != "" {
		key, rest, found := strings.Cut(text, "=")
		if !found {
			break
		}
		if i := strings.LastIndexAny(key, " \t\n"); i >= 0 {
			key = key[i+1:]
		}

		var value string
		if strings.HasPrefix(rest, `"`) {
			value, rest, _ = strings.Cut(rest[1:], `"`)
		} else if i := strings.IndexAny(rest, " \t\n"); i >= 0 {
			value, rest = rest[:i], rest[i:]
		} else {
			value, rest = rest, ""
		}

		if key != "" {
			params[key] = value
		}
		text = strings.TrimSpace(rest)
	}

	return params
}

// buildConfigParam quotes without escaping: the server reads a quoted value
// up to the next '"' and never unescapes, so %q escaping corrupts
// backslashes and quotes (issue #1300).
func buildConfigParam(key, val string) string {
	if strings.ContainsAny(val, " \t") {
		return fmt.Sprintf(`%s="%s"`, key, val)
	}
	return key + "=" + val
}

// validateConfigValue rejects config values MinIO cannot represent: a line
// break splits the one-key=value-per-line storage, a value starting with '"'
// is mis-parsed as a quoted value, a value ending with '"' is stored without
// that quote so it never matches the configuration, a quoted value cannot
// contain '"' at all because the format has no escape for it, and a value
// starting or ending with a single quote loses that quote, because madmin's
// SanitizeValue (parse-config.go) trims one leading and one trailing single
// quote from every value (issue #1300). A quote inside a value, as in a"b or
// it's, is stored literally and round-trips, so it is allowed.
func validateConfigValue(val interface{}, key string) (warns []string, errs []error) {
	v := val.(string)
	switch {
	case v == "":
	case strings.ContainsAny(v, "\r\n"):
		errs = append(errs, fmt.Errorf("%q cannot contain a line break: MinIO's config format stores one key=value pair per line", key))
	case strings.HasPrefix(v, `"`):
		errs = append(errs, fmt.Errorf("%q cannot start with a double quote: MinIO's config format cannot parse it", key))
	case strings.Contains(v, `"`) && strings.ContainsAny(v, " \t\n"):
		errs = append(errs, fmt.Errorf(`%q cannot contain a double quote together with whitespace: MinIO's config format cannot store a double quote inside a quoted value`, key))
	case strings.HasSuffix(v, `"`):
		errs = append(errs, fmt.Errorf("%q cannot end with a double quote: MinIO's config format drops it from an unquoted value", key))
	case strings.HasPrefix(v, `'`):
		errs = append(errs, fmt.Errorf("%q cannot start with a single quote: MinIO's config format trims it from the stored value", key))
	case strings.HasSuffix(v, `'`):
		errs = append(errs, fmt.Errorf("%q cannot end with a single quote: MinIO's config format trims it from the stored value", key))
	}
	return
}

func validateConfigKey(val interface{}, key string) (warns []string, errs []error) {
	v := val.(string)
	if v == "" {
		errs = append(errs, fmt.Errorf("%q cannot be empty", key))
		return
	}

	subsystem := v
	if separator := strings.Index(v, ":"); separator >= 0 {
		subsystem = v[:separator]
	}

	if !configSubSystems().Contains(subsystem) {
		warns = append(warns, fmt.Sprintf("Config key %q names no MinIO subsystem. A key is a subsystem such as `api` or `compression`, optionally followed by a target, as in `notify_webhook:primary`.", v))
	}

	return
}

func configSubSystems() set.StringSet {
	return madmin.SubSystems.Union(madmin.EOSSubSystems)
}
