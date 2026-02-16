package minio

import (
	"context"
	"errors"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/tags"
)

func resourceMinioBucketTags() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateBucketTags,
		ReadContext:   minioReadBucketTags,
		UpdateContext: minioUpdateBucketTags,
		DeleteContext: minioDeleteBucketTags,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Description:   "Manages tags for S3 buckets in MinIO.",
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the bucket",
			},
			"tags": {
				Type:        schema.TypeMap,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Map of tags to assign to the bucket",
			},
		},
	}
}

func minioCreateBucketTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := cfg.MinioBucket
	log.Printf("[DEBUG] Setting tags for bucket %s", bucket)
	if v, ok := d.GetOk("tags"); ok && len(v.(map[string]interface{})) > 0 {
		tagsMap := v.(map[string]interface{})
		bucketTags, err := tags.NewTags(convertToStringMap(tagsMap), false)
		if err != nil {
			return NewResourceError("creating bucket tags", bucket, err)
		}
		if err := cfg.MinioClient.SetBucketTagging(ctx, bucket, bucketTags); err != nil {
			return NewResourceError("setting bucket tags", bucket, err)
		}
	}
	d.SetId(bucket)
	return minioReadBucketTags(ctx, d, meta)
}

func minioReadBucketTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := d.Id()
	bucketTags, err := cfg.MinioClient.GetBucketTagging(ctx, bucket)
	if err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && minioErr.Code == "NoSuchTagSet" {
			_ = d.Set("bucket", bucket)
			_ = d.Set("tags", map[string]string{})
			return nil
		}
		if IsS3TaggingNotImplemented(err) {
			_ = d.Set("bucket", bucket)
			return nil
		}
		return NewResourceError("reading bucket tags", bucket, err)
	}
	if err := d.Set("bucket", bucket); err != nil {
		return NewResourceError("setting bucket", bucket, err)
	}
	_ = d.Set("tags", bucketTags.ToMap())
	return nil
}

func minioUpdateBucketTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := d.Id()
	if d.HasChange("tags") {
		if v, ok := d.GetOk("tags"); ok && len(v.(map[string]interface{})) > 0 {
			tagsMap := v.(map[string]interface{})
			bucketTags, err := tags.NewTags(convertToStringMap(tagsMap), false)
			if err != nil {
				return NewResourceError("updating bucket tags", bucket, err)
			}
			if err := cfg.MinioClient.SetBucketTagging(ctx, bucket, bucketTags); err != nil {
				return NewResourceError("setting bucket tags", bucket, err)
			}
		} else {
			if err := cfg.MinioClient.RemoveBucketTagging(ctx, bucket); err != nil {
				return NewResourceError("removing bucket tags", bucket, err)
			}
		}
	}
	return minioReadBucketTags(ctx, d, meta)
}

func minioDeleteBucketTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := d.Id()
	if err := cfg.MinioClient.RemoveBucketTagging(ctx, bucket); err != nil {
		return NewResourceError("deleting bucket tags", bucket, err)
	}
	d.SetId("")
	return nil
}
