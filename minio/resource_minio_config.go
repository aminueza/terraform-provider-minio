package minio

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The configuration key (e.g., 'api', 'notify_webhook:1', 'region')",
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					v := val.(string)
					if v == "" {
						errs = append(errs, fmt.Errorf("%q cannot be empty", key))
					}
					// Validate key format - should contain subsystem name
					if !strings.Contains(v, "_") && v != "region" && v != "name" {
						warns = append(warns, fmt.Sprintf("Config key %q should typically contain the subsystem (e.g., 'api', 'notify_webhook:1')", v))
					}
					return
				},
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

					// Check if all user-specified parameters exist in server response with same values
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

	log.Printf("[INFO] Creating/Setting MinIO config: %s", key)

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
		log.Printf("[ERROR] Failed to set config %s after retries: %s", key, err)
		return diag.FromErr(err)
	}

	// Set the ID to the key
	d.SetId(key)
	_ = d.Set("restart_required", restartRequired)

	if restartRequired {
		log.Printf("[WARN] Config change for %s requires MinIO server restart to take effect", key)
	}

	// Verify the config was set by reading it back
	return minioReadConfig(ctx, d, meta)
}

func minioReadConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	key := d.Id()

	log.Printf("[INFO] Reading MinIO config: %s", key)

	timeout := d.Timeout(schema.TimeoutRead)
	var configData []byte
	var err error

	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		configData, err = client.S3Admin.GetConfigKV(ctx, key)
		if err != nil {
			// Check if config key doesn't exist
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
				log.Printf("[WARN] Config %s no longer exists", key)
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
		log.Printf("[ERROR] Failed to read config %s after retries: %s", key, err)
		return diag.FromErr(err)
	}

	// If the resource was removed
	if d.Id() == "" {
		return nil
	}

	// Parse the config data to extract the value
	configStr := strings.TrimSpace(string(configData))
	log.Printf("[DEBUG] Raw config data for key %s: %s", key, configStr)

	// Handle different config formats
	if strings.HasPrefix(configStr, key+" ") {
		// Format: "key subsys:target key1=value1 key2=value2" -> "key1=value1 key2=value2"
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			fullValue := strings.TrimSpace(parts[1])
			log.Printf("[DEBUG] Setting full value from MinIO: %s", fullValue)
			_ = d.Set("value", fullValue)
		}
	} else {
		// For configs that don't follow the standard format (like region)
		// Just use the raw config string
		_ = d.Set("value", configStr)
	}

	_ = d.Set("key", key)

	return nil
}

func minioUpdateConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	key := d.Get("key").(string)
	value := d.Get("value").(string)

	log.Printf("[INFO] Updating MinIO config: %s", key)

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
		log.Printf("[ERROR] Failed to update config %s after retries: %s", key, err)
		return diag.FromErr(err)
	}

	_ = d.Set("restart_required", restartRequired)

	if restartRequired {
		log.Printf("[WARN] Config change for %s requires MinIO server restart to take effect", key)
	}

	return minioReadConfig(ctx, d, meta)
}

func minioDeleteConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	key := d.Id()

	log.Printf("[INFO] Deleting MinIO config: %s", key)

	// Check if config exists before attempting deletion
	_, err := client.S3Admin.GetConfigKV(ctx, key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
			log.Printf("[WARN] Config %s no longer exists, removing from state", key)
			d.SetId("")
			return nil
		}
		return diag.Errorf("error checking config before deletion: %s", err)
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
		log.Printf("[ERROR] Failed to delete config %s after retries: %s", key, err)
		return diag.FromErr(err)
	}

	if restart {
		log.Printf("[WARN] Config deletion for %s requires MinIO server restart to take effect", key)
	}

	d.SetId("")
	return nil
}

// parseConfigParams parses a space-separated key=value string into a map
func parseConfigParams(configStr string) map[string]string {
	params := make(map[string]string)
	if configStr == "" {
		return params
	}

	// Split by spaces to get individual key=value pairs
	pairs := strings.Fields(configStr)
	for _, pair := range pairs {
		// Split each pair by '=' to get key and value
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			params[parts[0]] = parts[1]
		}
	}

	return params
}
