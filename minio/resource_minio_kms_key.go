package minio

import (
	"context"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
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

	log.Printf("[DEBUG] Reading KMS key [%s]", keyConfig.MinioKMSKeyID)

	status, err := keyConfig.MinioAdmin.GetKeyStatus(ctx, keyConfig.MinioKMSKeyID)
	if err != nil {
		log.Printf("%s", NewResourceErrorStr("error reading KMS key", keyConfig.MinioKMSKeyID, err))
		d.SetId("")

		return nil
	}

	log.Printf("[DEBUG] KMS key [%s] exists!", keyConfig.MinioKMSKeyID)

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

	log.Printf("[DEBUG] Deleting KMS key [%s]", d.Id())

	if err := keyConfig.MinioAdmin.DeleteKey(ctx, d.Id()); err != nil {
		errResp := madmin.ToErrorResponse(err)
		errStr := err.Error()
		if strings.Contains(errResp.Code, "NotImplemented") ||
			strings.Contains(errStr, "not supported") ||
			strings.Contains(errStr, "not implemented") {
			log.Printf("[DEBUG] DeleteKey not supported for KMS key [%s] (external KMS backend): %v", d.Id(), err)
			_ = d.Set("key_id", "")
			d.SetId("")
			return nil
		}
		log.Printf("%s", NewResourceErrorStr("unable to remove KMS key", d.Id(), err))
		return NewResourceError("unable to remove KMS key", d.Id(), err)
	}

	log.Printf("[DEBUG] Deleted KMS key: [%s]", d.Id())

	_ = d.Set("key_id", "")

	return nil
}
