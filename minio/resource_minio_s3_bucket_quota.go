package minio

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioBucketQuota() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateBucketQuota,
		ReadContext:   minioReadBucketQuota,
		UpdateContext: minioUpdateBucketQuota,
		DeleteContext: minioDeleteBucketQuota,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Description:   "Manages quota limits for S3 buckets in MinIO.",
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the bucket",
			},
			"quota": {
				Type:         schema.TypeInt,
				Required:     true,
				Description:  "Quota size in bytes",
				ValidateFunc: validation.IntAtLeast(1),
			},
			"type": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "hard",
				Description:  "Quota type (only \"hard\" is supported)",
				ValidateFunc: validation.StringInSlice([]string{"hard"}, false),
			},
		},
	}
}

func minioCreateBucketQuota(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := cfg.MinioBucket
	quotaInt := d.Get("quota").(int)
	if quotaInt < 0 {
		return NewResourceError("setting bucket quota", bucket, fmt.Errorf("quota must be a non-negative value, got: %d", quotaInt))
	}
	quota := uint64(quotaInt) //#nosec G115 -- validated non-negative above

	log.Printf("[DEBUG] Setting quota for bucket %s", bucket)

	bucketQuota := madmin.BucketQuota{Quota: quota, Type: madmin.HardQuota}
	if err := cfg.MinioAdmin.SetBucketQuota(ctx, bucket, &bucketQuota); err != nil {
		return NewResourceError("setting bucket quota", bucket, err)
	}

	d.SetId(bucket)

	return minioReadBucketQuota(ctx, d, meta)
}

func minioReadBucketQuota(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := d.Id()

	log.Printf("[DEBUG] Reading quota for bucket %s", bucket)

	bucketQuota, err := cfg.MinioAdmin.GetBucketQuota(ctx, bucket)
	if err != nil {
		return NewResourceError("reading bucket quota", bucket, err)
	}

	if bucketQuota.Quota == 0 {
		log.Printf("[INFO] Bucket quota for %s is 0, removing from state", bucket)
		d.SetId("")
		return nil
	}

	if err := d.Set("bucket", bucket); err != nil {
		return NewResourceError("setting bucket", bucket, err)
	}
	quotaVal, ok := SafeUint64ToInt64(bucketQuota.Quota)
	if !ok {
		return NewResourceError("reading bucket quota", bucket, fmt.Errorf("quota value overflows int64: %d", bucketQuota.Quota))
	}
	_ = d.Set("quota", int(quotaVal))
	_ = d.Set("type", string(bucketQuota.Type))

	return nil
}

func minioUpdateBucketQuota(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := d.Id()

	if d.HasChanges("quota", "type") {
		quotaInt := d.Get("quota").(int)
		if quotaInt < 0 {
			return NewResourceError("updating bucket quota", bucket, fmt.Errorf("quota must be a non-negative value, got: %d", quotaInt))
		}
		quota := uint64(quotaInt) //#nosec G115 -- validated non-negative above
		log.Printf("[DEBUG] Updating quota for bucket %s", bucket)

		bucketQuota := madmin.BucketQuota{Quota: quota, Type: madmin.HardQuota}
		if err := cfg.MinioAdmin.SetBucketQuota(ctx, bucket, &bucketQuota); err != nil {
			return NewResourceError("updating bucket quota", bucket, err)
		}
	}

	return minioReadBucketQuota(ctx, d, meta)
}

func minioDeleteBucketQuota(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg := BucketConfig(d, meta)
	bucket := d.Id()

	log.Printf("[DEBUG] Clearing quota for bucket %s", bucket)

	bucketQuota := madmin.BucketQuota{Quota: 0}
	if err := cfg.MinioAdmin.SetBucketQuota(ctx, bucket, &bucketQuota); err != nil {
		return NewResourceError("deleting bucket quota", bucket, err)
	}

	d.SetId("")

	return nil
}
