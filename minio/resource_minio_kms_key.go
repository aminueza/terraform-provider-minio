package minio

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioKMSKey() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateKMSKey,
		ReadContext:   minioReadKMSKey,
		DeleteContext: minioDeleteKMSKey,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"key_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func minioCreateKMSKey(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	keyConfig := KMSKeyConfig(d, meta)

	keyID := keyConfig.MinioKMSKeyID

	if err := keyConfig.MinioAdmin.CreateKey(ctx, keyID); err != nil {
		return NewResourceError("error creating service account", keyID, err)
	}

	d.SetId(aws.StringValue(&keyID))
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
	var err error

	keyConfig := KMSKeyConfig(d, meta)

	log.Printf("[DEBUG] Deleting KMS key [%s]", d.Id())

	if err = keyConfig.MinioAdmin.DeleteKey(ctx, d.Id()); err != nil {
		log.Printf("%s", NewResourceErrorStr("unable to remove KMS key", d.Id(), err))

		return NewResourceError("unable to remove KMS key", d.Id(), err)
	}

	log.Printf("[DEBUG] Deleted KMS key: [%s]", d.Id())

	_ = d.Set("key_id", "")

	return nil

}
