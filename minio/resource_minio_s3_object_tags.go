package minio

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
)

func resourceMinioObjectTags() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateObjectTags,
		ReadContext:   minioReadObjectTags,
		UpdateContext: minioUpdateObjectTags,
		DeleteContext: minioDeleteObjectTags,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Description:   "Manages tags for S3 objects in a MinIO bucket.",
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the bucket",
			},
			"key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Object key",
			},
			"tags": {
				Type:        schema.TypeMap,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Map of tags to assign to the object",
			},
		},
	}
}

func parseBucketAndKeyFromID(id string) (bucket, objectKey string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) == 2 {
		bucket = parts[0]
		objectKey = parts[1]
	}
	return bucket, objectKey
}

func minioCreateObjectTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)
	objectKey := d.Get("key").(string)

	if bucket == "" || objectKey == "" {
		bucket, objectKey = parseBucketAndKeyFromID(d.Id())
	}

	cfg := &S3MinioObjectTags{
		MinioClient: meta.(*S3MinioClient).S3Client,
	}

	log.Printf("[DEBUG] Setting tags for object %s in bucket %s", objectKey, bucket)

	if v, ok := d.GetOk("tags"); ok && len(v.(map[string]interface{})) > 0 {
		tagsMap := convertToStringMap(v.(map[string]interface{}))

		srcOpts := minio.CopySrcOptions{
			Bucket: bucket,
			Object: objectKey,
		}

		dstOpts := minio.CopyDestOptions{
			Bucket:      bucket,
			Object:      objectKey,
			UserTags:    tagsMap,
			ReplaceTags: true,
		}

		if _, err := cfg.MinioClient.CopyObject(ctx, dstOpts, srcOpts); err != nil {
			return NewResourceError("creating object tags", fmt.Sprintf("%s/%s", bucket, objectKey), err)
		}
	}

	d.SetId(fmt.Sprintf("%s/%s", bucket, objectKey))
	return minioReadObjectTags(ctx, d, meta)
}

func minioReadObjectTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)
	objectKey := d.Get("key").(string)

	if bucket == "" || objectKey == "" {
		bucket, objectKey = parseBucketAndKeyFromID(d.Id())
	}

	cfg := &S3MinioObjectTags{
		MinioClient: meta.(*S3MinioClient).S3Client,
	}

	opts := minio.GetObjectTaggingOptions{}
	objectTags, err := cfg.MinioClient.GetObjectTagging(ctx, bucket, objectKey, opts)
	if err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && minioErr.Code == "NoSuchTagSet" {
			if err := d.Set("bucket", bucket); err != nil {
				return NewResourceError("setting bucket", fmt.Sprintf("%s/%s", bucket, objectKey), err)
			}
			if err := d.Set("key", objectKey); err != nil {
				return NewResourceError("setting key", fmt.Sprintf("%s/%s", bucket, objectKey), err)
			}
			_ = d.Set("tags", map[string]string{})
			return nil
		}
		return NewResourceError("reading object tags", fmt.Sprintf("%s/%s", bucket, objectKey), err)
	}

	if err := d.Set("bucket", bucket); err != nil {
		return NewResourceError("setting bucket", fmt.Sprintf("%s/%s", bucket, objectKey), err)
	}
	if err := d.Set("key", objectKey); err != nil {
		return NewResourceError("setting key", fmt.Sprintf("%s/%s", bucket, objectKey), err)
	}
	if err := d.Set("tags", objectTags.ToMap()); err != nil {
		return NewResourceError("setting tags", fmt.Sprintf("%s/%s", bucket, objectKey), err)
	}
	return nil
}

func minioUpdateObjectTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)
	objectKey := d.Get("key").(string)

	if bucket == "" || objectKey == "" {
		bucket, objectKey = parseBucketAndKeyFromID(d.Id())
	}

	cfg := &S3MinioObjectTags{
		MinioClient: meta.(*S3MinioClient).S3Client,
	}

	if d.HasChange("tags") {
		if v, ok := d.GetOk("tags"); ok && len(v.(map[string]interface{})) > 0 {
			tagsMap := convertToStringMap(v.(map[string]interface{}))

			srcOpts := minio.CopySrcOptions{
				Bucket: bucket,
				Object: objectKey,
			}

			dstOpts := minio.CopyDestOptions{
				Bucket:      bucket,
				Object:      objectKey,
				UserTags:    tagsMap,
				ReplaceTags: true,
			}

			if _, err := cfg.MinioClient.CopyObject(ctx, dstOpts, srcOpts); err != nil {
				return NewResourceError("updating object tags", fmt.Sprintf("%s/%s", bucket, objectKey), err)
			}
		} else {
			srcOpts := minio.CopySrcOptions{
				Bucket: bucket,
				Object: objectKey,
			}

			dstOpts := minio.CopyDestOptions{
				Bucket:      bucket,
				Object:      objectKey,
				ReplaceTags: true,
			}

			if _, err := cfg.MinioClient.CopyObject(ctx, dstOpts, srcOpts); err != nil {
				return NewResourceError("removing object tags", fmt.Sprintf("%s/%s", bucket, objectKey), err)
			}
		}
	}
	return minioReadObjectTags(ctx, d, meta)
}

func minioDeleteObjectTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucket := d.Get("bucket").(string)
	objectKey := d.Get("key").(string)

	if bucket == "" || objectKey == "" {
		bucket, objectKey = parseBucketAndKeyFromID(d.Id())
	}

	cfg := &S3MinioObjectTags{
		MinioClient: meta.(*S3MinioClient).S3Client,
	}

	srcOpts := minio.CopySrcOptions{
		Bucket: bucket,
		Object: objectKey,
	}

	dstOpts := minio.CopyDestOptions{
		Bucket:      bucket,
		Object:      objectKey,
		ReplaceTags: true,
	}

	if _, err := cfg.MinioClient.CopyObject(ctx, dstOpts, srcOpts); err != nil {
		return NewResourceError("deleting object tags", fmt.Sprintf("%s/%s", bucket, objectKey), err)
	}
	d.SetId("")
	return nil
}
