package minio

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Map of tags to assign to the bucket",
			},
		},
	}
}

func minioCreateBucketTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := cfg.MinioBucket
	tflog.Debug(ctx, fmt.Sprintf("Setting tags for bucket %s", bucket))

	if shouldSkipBucketTagging(cfg) {
		tflog.Info(ctx, "Bucket tagging is disabled for this provider configuration; skipping tag creation")
		if err := preserveBucketTagsState(d); err != nil {
			return NewResourceError("setting tags", bucket, err)
		}
		d.SetId(bucket)
		return nil
	}

	if v, ok := d.GetOk("tags"); ok && len(v.(map[string]interface{})) > 0 {
		tagsMap := v.(map[string]interface{})
		bucketTags, err := tags.NewTags(convertToStringMap(tagsMap), false)
		if err != nil {
			return NewResourceError("creating bucket tags", bucket, err)
		}
		if err := cfg.MinioClient.SetBucketTagging(ctx, bucket, bucketTags); err != nil {
			if !IsS3TaggingNotImplemented(err) {
				return NewResourceError("setting bucket tags", bucket, err)
			}
			tflog.Info(ctx, "Bucket tagging is not supported by backend; preserving state")
			if err := preserveBucketTagsState(d); err != nil {
				return NewResourceError("setting tags", bucket, err)
			}
		}
	}
	d.SetId(bucket)
	return minioReadBucketTags(ctx, d, meta)
}

func minioReadBucketTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := d.Id()

	if shouldSkipBucketTagging(cfg) {
		tflog.Info(ctx, "Bucket tagging is disabled for this provider configuration; preserving state")
		if err := preserveBucketTagsState(d); err != nil {
			return NewResourceError("setting tags", bucket, err)
		}
		return setResourceField(d, "bucket", bucket, bucket)
	}

	bucketTags, err := cfg.MinioClient.GetBucketTagging(ctx, bucket)
	if err != nil {
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && minioErr.Code == "NoSuchTagSet" {
			if diags := setResourceField(d, "bucket", bucket, bucket); diags != nil {
				return diags
			}
			return setResourceField(d, "tags", map[string]string{}, bucket)
		}
		if isNoSuchBucketError(err) {
			tflog.Warn(ctx, fmt.Sprintf("Bucket %s no longer exists, removing tags from state", bucket))
			d.SetId("")
			return nil
		}
		if IsS3TaggingNotImplemented(err) {
			tflog.Info(ctx, "Bucket tagging is not supported by backend; preserving state")
			if err := preserveBucketTagsState(d); err != nil {
				return NewResourceError("setting tags", bucket, err)
			}
			return setResourceField(d, "bucket", bucket, bucket)
		}
		return NewResourceError("reading bucket tags", bucket, err)
	}
	if diags := setResourceField(d, "bucket", bucket, bucket); diags != nil {
		return diags
	}
	return setResourceField(d, "tags", bucketTags.ToMap(), bucket)
}

func minioUpdateBucketTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := d.Id()

	if shouldSkipBucketTagging(cfg) {
		tflog.Info(ctx, "Bucket tagging is disabled for this provider configuration; preserving state")
		if err := preserveBucketTagsState(d); err != nil {
			return NewResourceError("setting tags", bucket, err)
		}
		return nil
	}

	if d.HasChange("tags") {
		if v, ok := d.GetOk("tags"); ok && len(v.(map[string]interface{})) > 0 {
			tagsMap := v.(map[string]interface{})
			bucketTags, err := tags.NewTags(convertToStringMap(tagsMap), false)
			if err != nil {
				return NewResourceError("updating bucket tags", bucket, err)
			}
			if err := cfg.MinioClient.SetBucketTagging(ctx, bucket, bucketTags); err != nil {
				if !IsS3TaggingNotImplemented(err) {
					return NewResourceError("setting bucket tags", bucket, err)
				}
				tflog.Info(ctx, "Bucket tagging is not supported by backend; preserving state")
				if err := preserveBucketTagsState(d); err != nil {
					return NewResourceError("setting tags", bucket, err)
				}
			}
		} else {
			if err := cfg.MinioClient.RemoveBucketTagging(ctx, bucket); err != nil {
				if !IsS3TaggingNotImplemented(err) {
					return NewResourceError("removing bucket tags", bucket, err)
				}
				tflog.Info(ctx, "Bucket tagging is not supported by backend; preserving state")
				if err := preserveBucketTagsState(d); err != nil {
					return NewResourceError("setting tags", bucket, err)
				}
			}
		}
	}
	return minioReadBucketTags(ctx, d, meta)
}

func minioDeleteBucketTags(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := d.Id()

	if shouldSkipBucketTagging(cfg) {
		tflog.Info(ctx, "Bucket tagging is disabled for this provider configuration; skipping deletion")
		return nil
	}

	if err := cfg.MinioClient.RemoveBucketTagging(ctx, bucket); err != nil {
		if !IsS3TaggingNotImplemented(err) {
			return NewResourceError("removing bucket tags", bucket, err)
		}
		tflog.Info(ctx, "Bucket tagging is not supported by backend; skipping deletion")
	}
	d.SetId("")
	return nil
}
