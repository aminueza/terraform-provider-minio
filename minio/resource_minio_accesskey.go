package minio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
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
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				Description: "The secret key. If provided, must be at least 8 characters.",
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// Suppress diffs when the provided secret matches the stored hash (or legacy plaintext in state).
					// This avoids perpetual diffs while keeping the plaintext secret out of state.
					// If user isn't providing a secret, no diff should be recorded.
					if strings.TrimSpace(new) == "" && strings.TrimSpace(old) == "" {
						return true
					}
					storedHash := ""
					if v, ok := d.GetOk("secret_key_hash"); ok {
						storedHash = v.(string)
					}
					// Back-compat: if we don't have a hash yet but old plaintext exists in state, use it to compute a temp hash
					if storedHash == "" && strings.TrimSpace(old) != "" {
						storedHash = computeSecretHash(old)
					}
					if storedHash == "" || strings.TrimSpace(new) == "" {
						return false
					}
					return computeSecretHash(new) == storedHash
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
			"secret_key_hash": {
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
				Description: "Hash of the secret key, stored for change detection without persisting plaintext.",
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
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Policy to attach to the access key (policy name or JSON document).",
			},
		},
	}
}

func minioCreateAccessKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	user := d.Get("user").(string)
	accessKey := d.Get("access_key").(string)
	secretKey := d.Get("secret_key").(string)
	status := d.Get("status").(string)
	policy := d.Get("policy").(string)

	log.Printf("[INFO] Creating accesskey for user %s", user)

	req := madmin.AddServiceAccountReq{
		SecretKey:  secretKey,
		AccessKey:  accessKey,
		TargetUser: user,
	}

	creds, err := client.S3Admin.AddServiceAccount(ctx, req)
	if err != nil {
		returnErr := fmt.Errorf("failed to create accesskey: %w", err)
		log.Printf("[ERROR] %s", returnErr)
		return diag.FromErr(returnErr)
	}

	d.SetId(aws.StringValue(&creds.AccessKey))
	_ = d.Set("access_key", creds.AccessKey)

	// Store only a hash of the secret in state for change detection (no plaintext persistence).
	usedSecret := secretKey
	if usedSecret == "" {
		usedSecret = creds.SecretKey
	}
	if strings.TrimSpace(usedSecret) != "" {
		_ = d.Set("secret_key_hash", computeSecretHash(usedSecret))
	}

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
		return diag.FromErr(err)
	}

	// Attach policy if provided
	if policy != "" {
		err := client.S3Admin.UpdateServiceAccount(ctx, creds.AccessKey, madmin.UpdateServiceAccountReq{
			NewPolicy: []byte(policy),
		})
		if err != nil {
			return diag.FromErr(fmt.Errorf("failed to attach policy to accesskey: %w", err))
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
		return diag.FromErr(err)
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

	// Migration: if we previously stored plaintext secret in state, compute its hash and then clear it.
	if v, ok := d.GetOk("secret_key_hash"); !ok || v.(string) == "" {
		if legacyPlain, ok2 := d.GetOk("secret_key"); ok2 {
			legacy := strings.TrimSpace(legacyPlain.(string))
			if legacy != "" {
				_ = d.Set("secret_key_hash", computeSecretHash(legacy))
			}
		}
	}

	// Clear secret_key from state - it's write-only and only available during creation
	_ = d.Set("secret_key", "")

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

	hasStatusChange := d.HasChange("status")
	hasPolicyChange := d.HasChange("policy")
	hasSecretChange := d.HasChange("secret_key")

	log.Printf("[INFO] Updating accesskey %s (status change: %v, policy change: %v)", accessKeyID, hasStatusChange, hasPolicyChange)

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
			return diag.FromErr(err)
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
			return diag.FromErr(err)
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
			return diag.FromErr(err)
		}
	}

	if hasSecretChange {
		newSecret := strings.TrimSpace(d.Get("secret_key").(string))
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
				return diag.FromErr(err)
			}
			_ = d.Set("secret_key_hash", computeSecretHash(newSecret))
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
		return diag.Errorf("error checking accesskey before deletion: %s", err)
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
		return diag.FromErr(err)
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
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

func computeSecretHash(secret string) string {
    sum := sha256.Sum256([]byte(secret))
    return hex.EncodeToString(sum[:])
}
