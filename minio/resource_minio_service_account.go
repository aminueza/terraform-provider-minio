package minio

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioServiceAccount() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateServiceAccount,
		ReadContext:   minioReadServiceAccount,
		UpdateContext: minioUpdateServiceAccount,
		DeleteContext: minioDeleteServiceAccount,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"target_user": {
				Type:        schema.TypeString,
				Description: "User the service account will be created for",
				Required:    true,
				ForceNew:    true,
			},
			"disable_user": {
				Type:        schema.TypeBool,
				Description: "Disable service account",
				Optional:    true,
				Default:     false,
			},
			"update_secret": {
				Type:        schema.TypeBool,
				Description: "rotate secret key",
				Optional:    true,
				Default:     false,
			},
			"status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"secret_key": {
				Type:        schema.TypeString,
				Description: "secret key of service account",
				Computed:    true,
				Sensitive:   true,
			},
			"access_key": {
				Type:        schema.TypeString,
				Description: "access key of service account",
				Computed:    true,
			},
			"policy": {
				Type:             schema.TypeString,
				Description:      "policy of service account as encoded JSON string",
				Optional:         true,
				ValidateFunc:     validateIAMPolicyJSON,
				DiffSuppressFunc: suppressEquivalentAwsPolicyDiffs,
			},
			"name": {
				Type:             schema.TypeString,
				Description:      "Name of service account (32 bytes max), can't be cleared once set",
				Optional:         true,
				DiffSuppressFunc: stringChangedToEmpty,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringLenBetween(1, 32)),
			},
			"description": {
				Type:             schema.TypeString,
				Description:      "Description of service account (256 bytes max), can't be cleared once set",
				Optional:         true,
				DiffSuppressFunc: stringChangedToEmpty,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringLenBetween(1, 256)),
			},
			"expiration": {
				Type:             schema.TypeString,
				Description:      "Expiration of service account. Must be between NOW+15min & NOW+365d",
				Optional:         true,
				Default:          "1970-01-01T00:00:00Z",
				ValidateDiagFunc: validateExpiration,
				DiffSuppressFunc: suppressTimeDiffs,
			},
		},
	}
}

func minioCreateServiceAccount(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	serviceAccountConfig := ServiceAccountConfig(d, meta)

	var err error
	targetUser := serviceAccountConfig.MinioTargetUser
	policy := serviceAccountConfig.MinioSAPolicy
	expiration, err := time.Parse(time.RFC3339, serviceAccountConfig.MinioExpiration)
	if err != nil {
		return NewResourceError("Failed to parse expiration", serviceAccountConfig.MinioExpiration, err)
	}

	serviceAccount, err := serviceAccountConfig.MinioAdmin.AddServiceAccount(ctx, madmin.AddServiceAccountReq{
		Policy:      processServiceAccountPolicy(policy),
		TargetUser:  targetUser,
		Name:        serviceAccountConfig.MinioName,
		Description: serviceAccountConfig.MinioDescription,
		Expiration:  &expiration,
	})
	if err != nil {
		return NewResourceError("error creating service account", targetUser, err)
	}
	accessKey := serviceAccount.AccessKey
	secretKey := serviceAccount.SecretKey

	d.SetId(aws.StringValue(&accessKey))
	_ = d.Set("access_key", accessKey)
	_ = d.Set("secret_key", secretKey)

	if serviceAccountConfig.MinioDisableUser {
		err = serviceAccountConfig.MinioAdmin.UpdateServiceAccount(ctx, accessKey, madmin.UpdateServiceAccountReq{NewStatus: "off"})
		if err != nil {
			return NewResourceError("error disabling service account %s: %s", d.Id(), err)
		}
	}

	return minioReadServiceAccount(ctx, d, meta)
}

func minioUpdateServiceAccount(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	serviceAccountConfig := ServiceAccountConfig(d, meta)
	policy := serviceAccountConfig.MinioSAPolicy

	wantedStatus := "on"
	var err error

	if serviceAccountConfig.MinioDisableUser {
		wantedStatus = "off"
	}

	serviceAccountServerInfo, err := serviceAccountConfig.MinioAdmin.InfoServiceAccount(ctx, serviceAccountConfig.MinioAccessKey)
	if err != nil {
		return NewResourceError("error to disable service account", d.Id(), err)
	}
	if serviceAccountServerInfo.AccountStatus != wantedStatus {
		err := serviceAccountConfig.MinioAdmin.UpdateServiceAccount(ctx, serviceAccountConfig.MinioAccessKey, madmin.UpdateServiceAccountReq{
			NewStatus: wantedStatus,
			NewPolicy: processServiceAccountPolicy(policy),
		})
		if err != nil {
			return NewResourceError("error to disable service account", d.Id(), err)
		}
	}

	wantedSecret := serviceAccountConfig.MinioSecretKey
	if serviceAccountConfig.MinioUpdateKey {
		if secretKey, err := generateSecretAccessKey(); err != nil {
			return NewResourceError("error creating user", d.Id(), err)
		} else {
			wantedSecret = secretKey
		}
	}

	if d.HasChange("secret_key") || serviceAccountConfig.MinioSecretKey != wantedSecret {
		err := serviceAccountConfig.MinioAdmin.UpdateServiceAccount(ctx, d.Id(), madmin.UpdateServiceAccountReq{
			NewSecretKey: wantedSecret,
			NewPolicy:    processServiceAccountPolicy(policy),
		})
		if err != nil {
			return NewResourceError("error updating service account Key %s: %s", d.Id(), err)
		}

		_ = d.Set("secret_key", wantedSecret)
	}

	if d.HasChange("policy") {
		err := serviceAccountConfig.MinioAdmin.UpdateServiceAccount(ctx, d.Id(), madmin.UpdateServiceAccountReq{
			NewPolicy: processServiceAccountPolicy(policy),
		})
		if err != nil {
			return NewResourceError("error updating service account policy %s: %s", d.Id(), err)
		}

		_ = d.Set("policy", policy)
	}

	if d.HasChange("name") {
		if serviceAccountConfig.MinioName == "" {
			return NewResourceError("Minio does not support removing service account names", d.Id(), serviceAccountConfig.MinioName)
		}
		err := serviceAccountConfig.MinioAdmin.UpdateServiceAccount(ctx, d.Id(), madmin.UpdateServiceAccountReq{
			NewName: serviceAccountConfig.MinioName,
		})
		if err != nil {
			return NewResourceError("error updating service account name %s: %s", d.Id(), err)
		}
	}

	if d.HasChange("description") {
		if serviceAccountConfig.MinioDescription == "" {
			return NewResourceError("Minio does not support removing service account descriptions", d.Id(), serviceAccountConfig.MinioDescription)
		}
		err := serviceAccountConfig.MinioAdmin.UpdateServiceAccount(ctx, d.Id(), madmin.UpdateServiceAccountReq{
			NewDescription: serviceAccountConfig.MinioDescription,
		})
		if err != nil {
			return NewResourceError("error updating service account description %s: %s", d.Id(), err)
		}
	}

	if d.HasChange("expiration") {
		expiration, err := time.Parse(time.RFC3339, serviceAccountConfig.MinioExpiration)
		if err != nil {
			return NewResourceError("error parsing service account expiration %s: %s", d.Id(), err)
		}
		err = serviceAccountConfig.MinioAdmin.UpdateServiceAccount(ctx, d.Id(), madmin.UpdateServiceAccountReq{
			NewExpiration: &expiration,
		})
		if err != nil {
			return NewResourceError("error updating service account expiration %s: %s", d.Id(), err)
		}
	}

	return minioReadServiceAccount(ctx, d, meta)
}

func minioReadServiceAccount(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	serviceAccountConfig := ServiceAccountConfig(d, meta)

	output, err := serviceAccountConfig.MinioAdmin.InfoServiceAccount(ctx, d.Id())
	if err != nil && err.Error() == "The specified service account is not found (Specified service account does not exist)" {
		d.SetId("")
		return nil
	}
	if err != nil {
		return NewResourceError("error reading service account %s: %s", d.Id(), err)
	}

	log.Printf("[DEBUG] (%v)", output)

	if _, ok := d.GetOk("access_key"); !ok {
		_ = d.Set("access_key", d.Id())
	}

	if err := d.Set("status", output.AccountStatus); err != nil {
		return NewResourceError("reading service account failed", d.Id(), err)
	}

	_ = d.Set("disable_user", output.AccountStatus == "off")

	targetUser := parseUserFromParentUser(output.ParentUser)
	if err := d.Set("target_user", targetUser); err != nil {
		return NewResourceError("reading service account failed", d.Id(), err)
	}

	if !output.ImpliedPolicy {
		_ = d.Set("policy", output.Policy)
	}

	d.Set("name", output.Name)
	d.Set("description", output.Description)

	if output.Expiration == nil {
		d.Set("expiration", "1970-01-01T00:00:00Z")
	} else {
		d.Set("expiration", output.Expiration.Format(time.RFC3339))
	}

	return nil
}

func minioDeleteServiceAccount(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	serviceAccountConfig := ServiceAccountConfig(d, meta)

	err := deleteMinioServiceAccount(ctx, serviceAccountConfig)
	if err != nil {
		return NewResourceError("error deleting service account %s: %s", d.Id(), err)
	}

	// Actively set resource as deleted
	d.SetId("")

	return nil
}

func deleteMinioServiceAccount(ctx context.Context, serviceAccountConfig *S3MinioServiceAccountConfig) (err error) {
	log.Println("[DEBUG] Deleting service account request:", serviceAccountConfig.MinioAccessKey)
	err = serviceAccountConfig.MinioAdmin.DeleteServiceAccount(ctx, serviceAccountConfig.MinioAccessKey)
	if err == nil {
		return
	}

	serviceAccountList, err := serviceAccountConfig.MinioAdmin.ListServiceAccounts(ctx, serviceAccountConfig.MinioTargetUser)
	if err != nil {
		return
	}

	for _, account := range serviceAccountList.Accounts {
		if account.AccessKey == serviceAccountConfig.MinioAccessKey {
			err = fmt.Errorf("service account %s not deleted", serviceAccountConfig.MinioAccessKey)
			return
		}
	}

	return
}

func processServiceAccountPolicy(policy string) []byte {
	if len(policy) == 0 {
		emptyPolicy := "{\n\"Version\": \"\",\n\"Statement\": null\n}"
		return []byte(emptyPolicy)
	}
	return []byte(policy)
}

// Handle LDAP responses in ParentUser struct
func parseUserFromParentUser(parentUser string) string {
	user := parentUser

	// Iterate through comma-separated chunks, will be ignored if not LDAP
	for _, ldapSection := range strings.Split(parentUser, ",") {
		splitSection := strings.Split(ldapSection, "=")
		if len(splitSection) == 2 && strings.ToLower(strings.TrimSpace(splitSection[0])) == "cn" {
			return strings.TrimSpace(splitSection[1])
		}
	}

	return user
}

func stringChangedToEmpty(k, oldValue, newValue string, d *schema.ResourceData) bool {
	return oldValue != "" && newValue == ""
}

func suppressTimeDiffs(k, old, new string, d *schema.ResourceData) bool {
	old_exp, err := time.Parse(time.RFC3339, old)
	if err != nil {
		return false
	}
	new_exp, err := time.Parse(time.RFC3339, new)
	if err != nil {
		return false
	}

	return old_exp.Compare(new_exp) == 0
}

func validateExpiration(val any, p cty.Path) diag.Diagnostics {
	var diags diag.Diagnostics

	value := val.(string)
	expiration, err := time.Parse(time.RFC3339, value)
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Invalid expiration",
			Detail:   fmt.Sprintf("%q cannot be parsed as RFC3339 Timestamp Format", value),
		})
	}

	key_duration := time.Until(expiration)
	if key_duration < 15*time.Minute || key_duration > 365*24*time.Minute {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Invalid expiration",
			Detail:   "Expiration must between 15 minutes and 365 days in the future",
		})
	}

	return diags
}
