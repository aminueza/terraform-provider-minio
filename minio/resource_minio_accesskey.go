package minio

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioAccessKey() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateAccessKey,
		ReadContext:   minioReadAccessKey,
		UpdateContext: minioUpdateAccessKey,
		DeleteContext: minioDeleteAccessKey,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"user": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The user for whom the access key is managed.",
			},
			"access_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "The access key.",
			},
			"secret_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				Description: "The secret key.",
			},
			"status": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "enabled",
				Description: "The status of the access key (enabled/disabled).",
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					status := val.(string)
					if status != "enabled" && status != "disabled" {
						errs = append(errs, fmt.Errorf("%q must be either 'enabled' or 'disabled', got: %s", key, status))
					}
					return
				},
			},
			"minio_alias": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "myminio",
				Description: "The MinIO alias to use with mc CLI (must be configured in mc).",
			},
		},
	}
}

func minioCreateAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	user := d.Get("user").(string)
	accessKey := d.Get("access_key").(string)
	secretKey := d.Get("secret_key").(string)
	minioAlias := d.Get("minio_alias").(string)

	// Build mc admin accesskey create command
	cmdArgs := []string{"admin", "accesskey", "create", minioAlias, user}
	if accessKey != "" {
		cmdArgs = append(cmdArgs, accessKey)
	}
	if secretKey != "" {
		cmdArgs = append(cmdArgs, secretKey)
	}

	out, err := runMcCommand(cmdArgs...)
	if err != nil {
		return diag.Errorf("failed to create accesskey: %s - output: %s", err, out)
	}

	// Parse output to extract generated keys
	// Example output:
	// Access Key: AKIAEXAMPLEKEY
	// Secret Key: mySuperSecretKey

	// Only try to parse if keys weren't provided (MinIO would have generated them)
	if accessKey == "" || secretKey == "" {
		parsedAccessKey := ""
		parsedSecretKey := ""
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "Access Key:") {
				parsedAccessKey = strings.TrimSpace(strings.TrimPrefix(line, "Access Key:"))
			} else if strings.HasPrefix(line, "Secret Key:") {
				parsedSecretKey = strings.TrimSpace(strings.TrimPrefix(line, "Secret Key:"))
			}
		}

		if parsedAccessKey != "" {
			d.SetId(parsedAccessKey)
			_ = d.Set("access_key", parsedAccessKey)
		}
		if parsedSecretKey != "" {
			_ = d.Set("secret_key", parsedSecretKey)
		}
	} else {
		// If keys were provided, use those
		d.SetId(accessKey)
	}

	// Apply status if needed
	if d.Get("status").(string) == "disabled" {
		return minioUpdateAccessKey(ctx, d, meta)
	}

	return minioReadAccessKey(ctx, d, meta)
}

func minioReadAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	user := d.Get("user").(string)
	minioAlias := d.Get("minio_alias").(string)
	cmdArgs := []string{"admin", "accesskey", "info", minioAlias, user}
	out, err := runMcCommand(cmdArgs...)
	if err != nil {
		log.Printf("[WARN] Failed to read accesskey for user %s: %s", user, err)
		d.SetId("")
		return diag.Errorf("failed to read accesskey: %s - output: %s", err, out)
	}

	// Example output:
	// Access Key: AKIAEXAMPLEKEY
	// Secret Key: mySuperSecretKey
	// Status: enabled

	accessKey := ""
	secretKey := ""
	status := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Access Key:") {
			accessKey = strings.TrimSpace(strings.TrimPrefix(line, "Access Key:"))
		} else if strings.HasPrefix(line, "Secret Key:") {
			secretKey = strings.TrimSpace(strings.TrimPrefix(line, "Secret Key:"))
		} else if strings.HasPrefix(line, "Status:") {
			status = strings.TrimSpace(strings.TrimPrefix(line, "Status:"))
		}
	}

	if accessKey == "" {
		log.Printf("[WARN] No access key found in output for user %s", user)
		d.SetId("")
		return diag.Errorf("access key not found in CLI output")
	}

	d.SetId(accessKey)
	_ = d.Set("access_key", accessKey)
	_ = d.Set("secret_key", secretKey)
	_ = d.Set("status", status)
	return nil
}

func minioUpdateAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	user := d.Get("user").(string)
	minioAlias := d.Get("minio_alias").(string)
	status := d.Get("status").(string)
	
	log.Printf("[INFO] Updating accesskey for user %s with status %s", user, status)
	cmdArgs := []string{"admin", "accesskey", "edit", minioAlias, user, "--status", status}
	out, err := runMcCommand(cmdArgs...)
	if err != nil {
		return diag.Errorf("failed to update accesskey: %s - output: %s", err, out)
	}
	return minioReadAccessKey(ctx, d, meta)
}

func minioDeleteAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	user := d.Get("user").(string)
	minioAlias := d.Get("minio_alias").(string)
	accessKey := d.Get("access_key").(string)
	
	log.Printf("[INFO] Deleting accesskey %s for user %s", accessKey, user)
	cmdArgs := []string{"admin", "accesskey", "remove", minioAlias, user}
	out, err := runMcCommand(cmdArgs...)
	if err != nil {
		return diag.Errorf("failed to delete accesskey: %s - output: %s", err, out)
	}
	d.SetId("")
	return nil
}

// runMcCommand executes the mc CLI with the given arguments and returns output/error.
func runMcCommand(args ...string) (string, error) {
	// NOTE: Assumes 'mc' is in PATH and required alias is configured.
	log.Printf("[DEBUG] Running command: mc %s", strings.Join(args, " "))
	cmd := exec.Command("mc", args...)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)
	
	if err != nil {
		log.Printf("[ERROR] Command failed: %s\nOutput: %s", err, outputStr)
		return outputStr, fmt.Errorf("command failed: %w, output: %s", err, outputStr)
	}
	
	log.Printf("[DEBUG] Command output: %s", outputStr)
	return outputStr, nil
}

