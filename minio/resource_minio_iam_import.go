package minio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioIAMImport() *schema.Resource {
	return &schema.Resource{
		Description: "Imports an IAM configuration into the MinIO server (users, groups, policies, service accounts). " +
			"Pair with `data.minio_iam_export` for cross-cluster migration or backup/restore. " +
			"MinIO's export embeds non-deterministic zip metadata, so chaining `data.minio_iam_export` -> " +
			"`minio_iam_import` in the same root module shows drift on every plan even when IAM is unchanged. " +
			"For stable plans, run export and import in separate states, or set " +
			"`lifecycle { ignore_changes = [iam_data] }` once the initial import has succeeded. " +
			"Delete is a no-op: MinIO does not provide a primitive to undo an import. " +
			"To purge imported entities, manage them as individual `minio_iam_*` resources or remove them out-of-band.",
		CreateContext: resourceMinioIAMImportApply,
		ReadContext:   resourceMinioIAMImportRead,
		UpdateContext: resourceMinioIAMImportApply,
		DeleteContext: resourceMinioIAMImportDelete,
		Schema: map[string]*schema.Schema{
			"iam_data": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
				ValidateFunc: func(v interface{}, k string) ([]string, []error) {
					if _, err := base64.StdEncoding.DecodeString(v.(string)); err != nil {
						return nil, []error{fmt.Errorf("%s: not valid base64: %w", k, err)}
					}
					return nil, nil
				},
				DiffSuppressFunc: suppressIAMImportDiffWhenSameSha,
				Description:      "Base64-encoded zip archive produced by `data.minio_iam_export.iam_data`. Re-import is skipped when the decoded payload's SHA-256 matches the previously applied one.",
			},
			"sha256": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "SHA-256 of the raw (decoded) payload that was last applied.",
			},
			"added_users":              countAttr("Number of users added by the last import."),
			"added_groups":             countAttr("Number of groups added by the last import."),
			"added_policies":           countAttr("Number of policies added by the last import."),
			"added_service_accounts":   countAttr("Number of service accounts added by the last import."),
			"skipped_users":            countAttr("Number of users skipped by the last import."),
			"skipped_groups":           countAttr("Number of groups skipped by the last import."),
			"skipped_policies":         countAttr("Number of policies skipped by the last import."),
			"skipped_service_accounts": countAttr("Number of service accounts skipped by the last import."),
			"removed_users":            countAttr("Number of users removed by the last import."),
			"removed_groups":           countAttr("Number of groups removed by the last import."),
			"removed_policies":         countAttr("Number of policies removed by the last import (empty policies are pruned)."),
			"removed_service_accounts": countAttr("Number of service accounts removed by the last import."),
			"failed_users":             countAttr("Number of users that failed to import."),
			"failed_groups":            countAttr("Number of groups that failed to import."),
			"failed_policies":          countAttr("Number of policies that failed to import."),
			"failed_service_accounts":  countAttr("Number of service accounts that failed to import."),
		},
	}
}

func countAttr(desc string) *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeInt,
		Computed:    true,
		Description: desc,
	}
}

func resourceMinioIAMImportApply(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := IAMImportConfig(d, meta)

	log.Printf("[DEBUG] Importing IAM configuration")

	raw, err := base64.StdEncoding.DecodeString(config.IAMData)
	if err != nil {
		return NewResourceError("decoding iam_data", "iam", err)
	}
	if len(raw) == 0 {
		return NewResourceError("decoding iam_data", "iam", errors.New("iam_data decoded to zero bytes"))
	}

	result, err := config.MinioAdmin.ImportIAMV2(ctx, io.NopCloser(bytes.NewReader(raw)))
	if err != nil {
		return NewResourceError("importing IAM", "iam", err)
	}

	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	d.SetId(digest)

	log.Printf("[DEBUG] Imported IAM configuration: sha256=%s", digest)

	return setIAMImportResult(d, &result, digest)
}

func resourceMinioIAMImportRead(_ context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
	if d.Id() == "" {
		return nil
	}
	encoded := d.Get("iam_data").(string)
	if encoded == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return NewResourceError("reading iam_data from state", "iam", err)
	}
	sum := sha256.Sum256(raw)
	if err := d.Set("sha256", hex.EncodeToString(sum[:])); err != nil {
		return NewResourceError("setting sha256", "iam", err)
	}
	return nil
}

func resourceMinioIAMImportDelete(_ context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] Deleting IAM import (no-op): %s", d.Id())
	d.SetId("")
	log.Printf("[DEBUG] Deleted IAM import")
	return nil
}

func setIAMImportResult(d *schema.ResourceData, result *madmin.ImportIAMResult, digest string) diag.Diagnostics {
	for _, field := range []struct {
		key   string
		value int
	}{
		{"added_users", len(result.Added.Users)},
		{"added_groups", len(result.Added.Groups)},
		{"added_policies", len(result.Added.Policies)},
		{"added_service_accounts", len(result.Added.ServiceAccounts)},
		{"skipped_users", len(result.Skipped.Users)},
		{"skipped_groups", len(result.Skipped.Groups)},
		{"skipped_policies", len(result.Skipped.Policies)},
		{"skipped_service_accounts", len(result.Skipped.ServiceAccounts)},
		{"removed_users", len(result.Removed.Users)},
		{"removed_groups", len(result.Removed.Groups)},
		{"removed_policies", len(result.Removed.Policies)},
		{"removed_service_accounts", len(result.Removed.ServiceAccounts)},
		{"failed_users", len(result.Failed.Users)},
		{"failed_groups", len(result.Failed.Groups)},
		{"failed_policies", len(result.Failed.Policies)},
		{"failed_service_accounts", len(result.Failed.ServiceAccounts)},
	} {
		if err := d.Set(field.key, field.value); err != nil {
			return NewResourceError("setting "+field.key, "iam", err)
		}
	}
	if err := d.Set("sha256", digest); err != nil {
		return NewResourceError("setting sha256", "iam", err)
	}
	return nil
}

func suppressIAMImportDiffWhenSameSha(_, oldVal, newVal string, _ *schema.ResourceData) bool {
	if oldVal == "" || newVal == "" {
		return false
	}
	oldRaw, err := base64.StdEncoding.DecodeString(oldVal)
	if err != nil {
		return false
	}
	newRaw, err := base64.StdEncoding.DecodeString(newVal)
	if err != nil {
		return false
	}
	oldSum := sha256.Sum256(oldRaw)
	newSum := sha256.Sum256(newRaw)
	return oldSum == newSum
}
