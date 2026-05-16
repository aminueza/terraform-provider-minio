package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/madmin-go/v3"
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
		CustomizeDiff: func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
			secretRaw := d.Get("secret_key")
			secret := ""
			if secretRaw != nil {
				secret = strings.TrimSpace(secretRaw.(string))
			}

			_, hasSecretWO, err := getRawConfigStringAttr(d.GetRawConfig(), "secret_key_wo", "secret_key_wo")
			if err != nil {
				return err
			}

			versionRaw, hasVersion := d.GetOk("secret_key_version")
			version := ""
			if hasVersion {
				version = strings.TrimSpace(versionRaw.(string))
			}

			if secret != "" && hasSecretWO {
				return fmt.Errorf("secret_key and secret_key_wo cannot be set together")
			}

			// Enforce that when secret_key is set, secret_key_version must be provided
			if secret != "" && (!hasVersion || version == "") {
				return fmt.Errorf("secret_key_version must be provided when secret_key is set")
			}

			hasSecretVersionChange := d.HasChange("secret_key_version") && version != ""
			// When secret_key_version changes to non-empty value, validate secret_key availability
			if hasSecretVersionChange {
				// Check if secret_key is present in the configuration
				rawConfig := d.GetRawConfig()
				secretKeyAttr := rawConfig.GetAttr("secret_key")

				// Only error if secret_key is completely missing from config
				// This allows computed values from other resources (e.g., random_password)
				if secret == "" && secretKeyAttr.IsNull() {
					return fmt.Errorf("secret_key must be provided when secret_key_version changes")
				}
			}

			return nil
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Read:   schema.DefaultTimeout(2 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},
		Schema: map[string]*schema.Schema{
			"user": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The user for whom the access key is managed.",
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					v := val.(string)
					if v == "" {
						errs = append(errs, fmt.Errorf("%q cannot be empty", key))
					}
					return
				},
			},
			"access_key": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				// Sensitive so the marker matches when access_key is sourced
				// from a sensitive expression (e.g. random_password.result);
				// otherwise state (untagged) vs config (tagged) is reported
				// as a diff on every plan even when the value is identical.
				Sensitive:   true,
				Description: "The access key. If provided, must be between 8 and 20 characters.",
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					v := val.(string)
					if v != "" {
						if len(v) < 8 || len(v) > 20 {
							errs = append(errs, fmt.Errorf("%q must be between 8 and 20 characters when specified", key))
						}
					}
					return
				},
			},
			"secret_key": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				ConflictsWith: []string{
					"secret_key_wo",
					"secret_key_wo_version",
				},
				Description: "The secret key. If provided, must be at least 8 characters. This is a write-only field and will not be stored in state.",
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// On creation, do not suppress so validation and planning behave normally
					if d.Id() == "" {
						return false
					}
					// If secret_key_version changes, do NOT suppress so the new secret is available to Update/CustomizeDiff
					if d.HasChange("secret_key_version") {
						return false
					}
					// For existing resources with no version change, suppress diffs to keep plans clean
					return true
				},
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					v := val.(string)
					if v != "" {
						if len(v) < 8 {
							errs = append(errs, fmt.Errorf("%q must be at least 8 characters when specified", key))
						}
					}
					return
				},
			},
			"secret_key_wo": {
				Type:      schema.TypeString,
				Optional:  true,
				WriteOnly: true,
				Sensitive: true,
				RequiredWith: []string{
					"secret_key_wo_version",
				},
				ConflictsWith: []string{
					"secret_key",
					"secret_key_version",
				},
				Description: "Write-only secret key for the access key.",
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					if d.Id() == "" {
						return false
					}
					if d.HasChange("secret_key_wo_version") {
						return false
					}
					return true
				},
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					v := val.(string)
					if v != "" {
						if len(v) < 8 {
							errs = append(errs, fmt.Errorf("%q must be at least 8 characters when specified", key))
						}
					}
					return
				},
			},
			"secret_key_version": {
				Type:     schema.TypeString,
				Optional: true,
				ConflictsWith: []string{
					"secret_key_wo",
					"secret_key_wo_version",
				},
				Description: "Version identifier for the secret key. Change this value to trigger a secret key rotation. Can be a hash, version number, timestamp, or any string that changes when the secret changes.",
			},
			"secret_key_wo_version": {
				Type:         schema.TypeInt,
				Optional:     true,
				ValidateFunc: validation.IntAtLeast(1),
				RequiredWith: []string{
					"secret_key_wo",
				},
				ConflictsWith: []string{
					"secret_key",
					"secret_key_version",
				},
				Description: "Version identifier for secret_key_wo. Increment this integer to trigger rotation when using secret_key_wo.",
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
			"policy": {
				Type:             schema.TypeString,
				Optional:         true,
				Description:      "Policy to attach to the access key (policy name or JSON document).",
				DiffSuppressFunc: suppressEquivalentAwsPolicyDiffs,
			},
			"description": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Description for the access key (max 256 characters).",
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					v := val.(string)
					if len(v) > 256 {
						errs = append(errs, fmt.Errorf("%q must be at most 256 characters", key))
					}
					return
				},
			},
		},
	}
}

func minioCreateAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	user := d.Get("user").(string)
	accessKey := d.Get("access_key").(string)
	secretKey := d.Get("secret_key").(string)
	secretKeyWO, hasSecretKeyWO, err := getWriteOnlyStringAt(d, cty.GetAttrPath("secret_key_wo"), "secret_key_wo")
	if err != nil {
		return NewResourceError("retrieving secret_key_wo", user, err)
	}
	if hasSecretKeyWO {
		secretKey = secretKeyWO
	}
	status := d.Get("status").(string)
	policy := d.Get("policy").(string)
	description := d.Get("description").(string)

	log.Printf("[INFO] Creating accesskey for user %s", user)

	req := madmin.AddServiceAccountReq{
		SecretKey:   secretKey,
		AccessKey:   accessKey,
		TargetUser:  user,
		Description: description,
	}

	creds, err := client.S3Admin.AddServiceAccount(ctx, req)
	if err != nil {
		return NewResourceError("creating access key", user, err)
	}

	d.SetId(creds.AccessKey)
	_ = d.Set("access_key", creds.AccessKey)

	timeout := d.Timeout(schema.TimeoutCreate)
	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		_, err := client.S3Admin.InfoServiceAccount(ctx, creds.AccessKey)
		if err != nil {
			return retry.RetryableError(
				fmt.Errorf("waiting for accesskey %s to become available: %w", creds.AccessKey, err),
			)
		}
		return nil
	})
	if err != nil {
		return NewResourceError("waiting for access key readiness", creds.AccessKey, err)
	}

	// Attach policy if provided
	if policy != "" {
		err := client.S3Admin.UpdateServiceAccount(ctx, creds.AccessKey, madmin.UpdateServiceAccountReq{
			NewPolicy: []byte(policy),
		})
		if err != nil {
			return NewResourceError("updating access key policy", creds.AccessKey, err)
		}
	}

	if status == "disabled" {
		return minioUpdateAccessKey(ctx, d, meta)
	}

	return minioReadAccessKey(ctx, d, meta)
}

func minioReadAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	accessKeyID := d.Id()

	log.Printf("[INFO] Reading accesskey %s", accessKeyID)

	timeout := d.Timeout(schema.TimeoutRead)
	var info madmin.InfoServiceAccountResp
	var err error

	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		info, err = client.S3Admin.InfoServiceAccount(ctx, accessKeyID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "service account does not exist") {
				log.Printf("[WARN] AccessKey %s no longer exists", accessKeyID)
				d.SetId("")
				return nil
			}

			return retry.RetryableError(fmt.Errorf("error reading accesskey %s: %w", accessKeyID, err))
		}
		return nil
	})

	if err != nil {
		log.Printf("[ERROR] Failed to read accesskey %s after retries: %s", accessKeyID, err)
		return NewResourceError("reading access key", accessKeyID, err)
	}

	if d.Id() == "" {
		return nil
	}

	parentUser := info.ParentUser
	_ = d.Set("user", parentUser)

	var status string
	if info.AccountStatus == "on" {
		status = "enabled"
	} else {
		status = "disabled"
	}
	_ = d.Set("status", status)
	_ = d.Set("access_key", accessKeyID)
	_ = d.Set("description", info.Description)

	// Clear write-only secret fields from state.
	_ = d.Set("secret_key", "")
	_ = d.Set("secret_key_wo", "")

	// Only set policy in state if it's not implied
	if !info.ImpliedPolicy {
		policy := strings.TrimSpace(info.Policy)
		isEmptyPolicy := false
		if policy == "" || policy == "null" || policy == "{}" {
			isEmptyPolicy = true
		} else {
			var policyObj map[string]interface{}
			err := json.Unmarshal([]byte(policy), &policyObj)
			if err == nil {
				// Check for empty or null Statement and empty Version
				statement, hasStatement := policyObj["Statement"]
				version, hasVersion := policyObj["Version"]
				if hasStatement && hasVersion {
					statementIsEmpty := statement == nil || (fmt.Sprintf("%v", statement) == "<nil>" || fmt.Sprintf("%v", statement) == "null")
					versionIsEmpty := version == nil || version == ""
					if statementIsEmpty && versionIsEmpty {
						isEmptyPolicy = true
					}
				}
			}
		}

		if !isEmptyPolicy {
			oldPolicy := ""
			if v, ok := d.GetOk("policy"); ok {
				oldPolicy = v.(string)
			}
			normalized, err := NormalizeAndCompareJSONPolicies(oldPolicy, policy)
			if err != nil {
				_ = d.Set("policy", policy) // fallback to raw
			} else {
				_ = d.Set("policy", normalized)
			}
		} else {
			_ = d.Set("policy", nil)
		}
	} else {
		// If policy is implied, don't set it in state to avoid perpetual diff
		_ = d.Set("policy", nil)
	}

	return nil
}

func minioUpdateAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	accessKeyID := d.Id()
	status := d.Get("status").(string)
	policy := d.Get("policy").(string)
	description := d.Get("description").(string)

	hasStatusChange := d.HasChange("status")
	hasPolicyChange := d.HasChange("policy")
	secretVersion := strings.TrimSpace(d.Get("secret_key_version").(string))
	hasSecretChange := d.HasChange("secret_key_version") && secretVersion != ""
	hasDescriptionChange := d.HasChange("description")

	log.Printf("[INFO] Updating accesskey %s (status change: %v, policy change: %v, secret change: %v, description change: %v)", accessKeyID, hasStatusChange, hasPolicyChange, hasSecretChange, hasDescriptionChange)

	timeout := d.Timeout(schema.TimeoutUpdate)

	if hasStatusChange {
		newStatus := "on"
		if status == "disabled" {
			newStatus = "off"
		}

		log.Printf("[DEBUG] Updating accesskey %s status to %s", accessKeyID, newStatus)

		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			err := client.S3Admin.UpdateServiceAccount(ctx, accessKeyID, madmin.UpdateServiceAccountReq{NewStatus: newStatus})
			if err != nil {
				if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
					return retry.RetryableError(fmt.Errorf("transient error updating accesskey %s status: %w", accessKeyID, err))
				}

				return retry.NonRetryableError(fmt.Errorf("failed to update accesskey status: %w", err))
			}
			return nil
		})

		if err != nil {
			log.Printf("[ERROR] Failed to update accesskey %s status after retries: %s", accessKeyID, err)
			return NewResourceError("updating access key status", accessKeyID, err)
		}

		err = retry.RetryContext(ctx, 30*time.Second, func() *retry.RetryError {
			info, err := client.S3Admin.InfoServiceAccount(ctx, accessKeyID)
			if err != nil {
				return retry.RetryableError(fmt.Errorf("error verifying accesskey %s status update: %w", accessKeyID, err))
			}

			actualStatus := "enabled"
			if info.AccountStatus == "off" {
				actualStatus = "disabled"
			}

			if actualStatus != status {
				return retry.RetryableError(fmt.Errorf("accesskey %s status not yet updated (current: %s, expected: %s)",
					accessKeyID, actualStatus, status))
			}

			return nil
		})

		if err != nil {
			return NewResourceError("verifying access key status", accessKeyID, err)
		}
	}

	if hasPolicyChange {
		log.Printf("[DEBUG] Updating accesskey %s policy", accessKeyID)

		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			err := client.S3Admin.UpdateServiceAccount(ctx, accessKeyID, madmin.UpdateServiceAccountReq{NewPolicy: []byte(policy)})
			if err != nil {
				if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
					return retry.RetryableError(fmt.Errorf("transient error updating accesskey %s policy: %w", accessKeyID, err))
				}

				return retry.NonRetryableError(fmt.Errorf("failed to update accesskey policy: %w", err))
			}
			return nil
		})

		if err != nil {
			log.Printf("[ERROR] Failed to update accesskey %s policy after retries: %s", accessKeyID, err)
			return NewResourceError("updating access key policy", accessKeyID, err)
		}
	}

	_, hasSecretWOVersion := d.GetOk("secret_key_wo_version")
	hasSecretWOChange := d.HasChange("secret_key_wo_version") && hasSecretWOVersion

	if hasSecretChange || hasSecretWOChange {
		newSecret := strings.TrimSpace(d.Get("secret_key").(string))
		if hasSecretWOChange {
			secretWO, hasSecretWO, err := getWriteOnlyStringAt(d, cty.GetAttrPath("secret_key_wo"), "secret_key_wo")
			if err != nil {
				return NewResourceError("retrieving secret_key_wo", accessKeyID, err)
			}
			if !hasSecretWO {
				return NewResourceError("missing required secret_key_wo for secret rotation", accessKeyID, fmt.Errorf("secret_key_wo must be provided when secret_key_wo_version changes"))
			}
			newSecret = secretWO
		}
		if newSecret != "" {
			log.Printf("[DEBUG] Rotating secret for accesskey %s", accessKeyID)
			err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
				err := client.S3Admin.UpdateServiceAccount(ctx, accessKeyID, madmin.UpdateServiceAccountReq{NewSecretKey: newSecret})
				if err != nil {
					if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
						return retry.RetryableError(fmt.Errorf("transient error rotating accesskey %s secret: %w", accessKeyID, err))
					}
					return retry.NonRetryableError(fmt.Errorf("failed to rotate accesskey secret: %w", err))
				}
				return nil
			})
			if err != nil {
				log.Printf("[ERROR] Failed to rotate secret for accesskey %s after retries: %s", accessKeyID, err)
				return NewResourceError("rotating access key secret", accessKeyID, err)
			}
			// Clear secret_key from state after rotation
			_ = d.Set("secret_key", "")
		} else if hasSecretWOChange {
			return NewResourceError("missing required secret_key_wo for secret rotation", accessKeyID, fmt.Errorf("secret_key_wo must be provided when secret_key_wo_version changes"))
		} else {
			return NewResourceError("missing required secret_key for secret rotation", accessKeyID, fmt.Errorf("secret_key must be provided when secret_key_version changes"))
		}
	}

	if hasDescriptionChange {
		log.Printf("[DEBUG] Updating accesskey %s description", accessKeyID)

		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			err := client.S3Admin.UpdateServiceAccount(ctx, accessKeyID, madmin.UpdateServiceAccountReq{NewDescription: description})
			if err != nil {
				if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
					return retry.RetryableError(fmt.Errorf("transient error updating accesskey %s description: %w", accessKeyID, err))
				}

				return retry.NonRetryableError(fmt.Errorf("failed to update accesskey description: %w", err))
			}
			return nil
		})

		if err != nil {
			log.Printf("[ERROR] Failed to update accesskey %s description after retries: %s", accessKeyID, err)
			return NewResourceError("updating access key description", accessKeyID, err)
		}
	}

	return minioReadAccessKey(ctx, d, meta)
}

func minioDeleteAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	accessKeyID := d.Id()

	log.Printf("[INFO] Deleting accesskey %s", accessKeyID)

	_, err := client.S3Admin.InfoServiceAccount(ctx, accessKeyID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "service account does not exist") {
			log.Printf("[WARN] AccessKey %s no longer exists, removing from state", accessKeyID)
			d.SetId("")
			return nil
		}
		return NewResourceError("checking access key before deletion", accessKeyID, err)
	}

	timeout := d.Timeout(schema.TimeoutDelete)
	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err := client.S3Admin.DeleteServiceAccount(ctx, accessKeyID)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error deleting accesskey %s: %w", accessKeyID, err))
			}

			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "service account does not exist") {
				return nil
			}

			return retry.NonRetryableError(fmt.Errorf("failed to delete accesskey: %w", err))
		}
		return nil
	})

	if err != nil {
		log.Printf("[ERROR] Failed to delete accesskey %s after retries: %s", accessKeyID, err)
		return NewResourceError("deleting access key", accessKeyID, err)
	}

	err = retry.RetryContext(ctx, 30*time.Second, func() *retry.RetryError {
		_, err := client.S3Admin.InfoServiceAccount(ctx, accessKeyID)
		if err == nil {
			return retry.RetryableError(fmt.Errorf("waiting for accesskey %s to be deleted", accessKeyID))
		}

		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "service account does not exist") {
			return nil
		}

		return retry.RetryableError(fmt.Errorf("error checking if accesskey %s is deleted: %w", accessKeyID, err))
	})

	if err != nil {
		log.Printf("[ERROR] Failed to confirm deletion of accesskey %s: %s", accessKeyID, err)
		return NewResourceError("confirming access key deletion", accessKeyID, err)
	}

	d.SetId("")
	return nil
}

func suppressEquivalentAwsPolicyDiffs(k, old, new string, d *schema.ResourceData) bool {
	equivalent, err := awspolicy.PoliciesAreEquivalent(old, new)
	if err != nil {
		return false
	}

	return equivalent
}
