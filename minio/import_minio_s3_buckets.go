package minio

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioS3BucketImportState(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) ([]*schema.ResourceData, error) {

	results := make([]*schema.ResourceData, 1)
	results[0] = d

	conn := meta.(*S3MinioClient).S3Client
	pol, err := conn.GetBucketPolicy(ctx, d.Id())
	if err != nil {
		return nil, fmt.Errorf("Error importing Minio S3 bucket policy: %s", err)
	}

	policy := resourceMinioBucket()
	pData := policy.Data(nil)
	pData.SetId(d.Id())
	pData.SetType("minio_s3_bucket_policy")
	_ = pData.Set("bucket", d.Id())
	_ = pData.Set("acl", pol)
	results = append(results, pData)

	return results, nil
}
