package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
)

func resourceMinioKMSKey() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateKMSKey,
		ReadContext:   minioReadKMSKey,
		DeleteContext: minioDeleteKMSKey,
		Importer: &schema.ResourceImporter{
			StateContext: func(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
				if err := d.Set("key_id", d.Id()); err != nil {
					return nil, err
				}

				return []*schema.ResourceData{d}, nil
			},
		},

		Schema: map[string]*schema.Schema{
			"key_id": {
				Type:        schema.TypeString,
				Description: "KMS key ID",
				Required:    true,
				ForceNew:    true,
			},
		},
	}
}

func minioCreateKMSKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	keyConfig := KMSKeyConfig(d, meta)

	keyID := keyConfig.MinioKMSKeyID

	if err := keyConfig.MinioAdmin.CreateKey(ctx, keyID); err != nil {
		return NewResourceError("error creating KMS key", keyID, err)
	}

	d.SetId(keyID)
	_ = d.Set("key_id", d.Id())

	return minioReadKMSKey(ctx, d, meta)
}

func minioReadKMSKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	keyConfig := KMSKeyConfig(d, meta)

	tflog.Debug(ctx, fmt.Sprintf("Reading KMS key [%s]", keyConfig.MinioKMSKeyID))

	status, err := keyConfig.MinioAdmin.GetKeyStatus(ctx, keyConfig.MinioKMSKeyID)
	if err != nil {
		return NewResourceError("error reading KMS key", keyConfig.MinioKMSKeyID, err)
	}

	if !d.IsNewResource() && isKMSKeyNotFound(status.EncryptionErr) {
		tflog.Warn(ctx, fmt.Sprintf("KMS key [%s] not found, removing from state", keyConfig.MinioKMSKeyID))
		d.SetId("")

		return nil
	}

	tflog.Debug(ctx, fmt.Sprintf("KMS key [%s] exists!", keyConfig.MinioKMSKeyID))

	if status.EncryptionErr != "" {
		return NewResourceError("KMS key has encryption error", keyConfig.MinioKMSKeyID, status.EncryptionErr)
	}

	if status.DecryptionErr != "" {
		return NewResourceError("KMS key has decryption error", keyConfig.MinioKMSKeyID, status.DecryptionErr)
	}

	_ = d.Set("key_id", d.Id())

	return nil
}

func minioDeleteKMSKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	keyConfig := KMSKeyConfig(d, meta)

	tflog.Debug(ctx, fmt.Sprintf("Deleting KMS key [%s]", d.Id()))

	if err := keyConfig.MinioAdmin.DeleteKey(ctx, d.Id()); err != nil {
		errResp := madmin.ToErrorResponse(err)
		errStr := err.Error()
		if errResp.Code == "BadRequest" ||
			strings.HasPrefix(errResp.Code, "4") ||
			strings.Contains(errResp.Code, "NotImplemented") ||
			strings.Contains(errStr, "not supported") ||
			strings.Contains(errStr, "not implemented") {
			tflog.Debug(ctx, fmt.Sprintf("DeleteKey not supported for KMS key [%s] (external KMS backend): %v", d.Id(), err))
			_ = d.Set("key_id", "")
			d.SetId("")
			return nil
		}
		tflog.Error(ctx, NewResourceErrorStr("unable to remove KMS key", d.Id(), err))
		return NewResourceError("unable to remove KMS key", d.Id(), err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Deleted KMS key: [%s]", d.Id()))

	_ = d.Set("key_id", "")

	return nil
}

// isKMSKeyNotFound reports whether a key status error means the key does not
// exist. MinIO answers the status call with 200 and puts the KMS error in
// EncryptionErr: "key with given key ID does not exist" for the built-in KMS
// and MinIO KMS, "key does not exist" for KES.
func isKMSKeyNotFound(encryptionErr string) bool {
	return strings.Contains(encryptionErr, "does not exist")
}
