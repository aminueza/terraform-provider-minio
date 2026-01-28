package minio

import (
	"context"
	"fmt"
	"io"
	"log"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
)

func dataSourceMinioS3Object() *schema.Resource {
	return &schema.Resource{
		Description:        "Reads an object from a MinIO bucket including its content and metadata.",
		ReadWithoutTimeout: dataSourceMinioS3ObjectRead,
		Schema: map[string]*schema.Schema{
			"bucket_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The name of the bucket containing the object",
			},
			"object_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The name of the object",
			},
			"content_type": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The content type of the object",
			},
			"content": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The content of the object",
			},
			"etag": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The ETag of the object",
			},
			"expires": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The  date and time at which the object is no longer able to be cached",
			},
			"expiration_rule_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The lifecycle expiry-date and ruleID associated with the expiry",
			},
			"is_latest": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether the object is the latest version",
			},
			"last_modified": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The last modified time of the object",
			},
			"owner": {
				Type:     schema.TypeMap,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Description: "The owner of the object",
			},
			"size": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "The size of the object",
			},
			"storage_class": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The storage class of the object",
			},
			"version_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The version ID of the object",
			},

			"checksum_crc32": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The CRC32 checksum of the object",
			},
			"checksum_crc32c": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The CRC32C checksum of the object",
			},
			"checksum_sha1": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The SHA1 checksum of the object",
			},
			"checksum_sha256": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The SHA256 checksum of the object",
			},
		},
	}
}

func dataSourceMinioS3ObjectRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	conn := meta.(*S3MinioClient).S3Client

	bucketName := d.Get("bucket_name").(string)
	if bucketName == "" {
		return diag.Errorf("bucket_name is required")
	}

	objectName := d.Get("object_name").(string)
	if objectName == "" {
		return diag.Errorf("object_name is required")
	}

	object, err := conn.GetObject(ctx, bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return diag.FromErr(fmt.Errorf("error reading object: %w", err))
	}
	defer func() {
		err := object.Close()
		if err != nil {
			log.Printf("[WARN] Error closing S3 object source (%s): %s", objectName, err)
		}
	}()

	objectInfo, err := object.Stat()
	if err != nil {
		return diag.FromErr(fmt.Errorf("error getting object info: %w", err))
	}

	objectContent := make([]byte, objectInfo.Size)
	bytesRead, err := object.Read(objectContent)
	if err != io.EOF {
		return diag.FromErr(fmt.Errorf("error reading object content: %w", err))
	}
	if bytesRead != int(objectInfo.Size) {
		return diag.Errorf("error reading object content: expected %d bytes, got %d", objectInfo.Size, bytesRead)
	}

	d.SetId(strconv.Itoa(HashcodeString(bucketName + objectName + objectInfo.VersionID)))

	owner := map[string]interface{}{
		"display_name": objectInfo.Owner.DisplayName,
		"id":           objectInfo.Owner.ID,
	}

	_ = d.Set("content_type", objectInfo.ContentType)
	_ = d.Set("content", string(objectContent))
	_ = d.Set("etag", objectInfo.ETag)
	_ = d.Set("expires", objectInfo.Expires)
	_ = d.Set("expiration_rule_id", objectInfo.ExpirationRuleID)
	_ = d.Set("is_latest", objectInfo.IsLatest)
	_ = d.Set("last_modified", objectInfo.LastModified)
	_ = d.Set("owner", owner)
	_ = d.Set("size", objectInfo.Size)
	_ = d.Set("storage_class", objectInfo.StorageClass)
	_ = d.Set("version_id", objectInfo.VersionID)
	_ = d.Set("checksum_crc32", objectInfo.ChecksumCRC32)
	_ = d.Set("checksum_crc32c", objectInfo.ChecksumCRC32C)
	_ = d.Set("checksum_sha1", objectInfo.ChecksumSHA1)
	_ = d.Set("checksum_sha256", objectInfo.ChecksumSHA256)

	return diags
}
