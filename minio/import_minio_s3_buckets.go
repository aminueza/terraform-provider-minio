package minio

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7/pkg/policy"
)

func resourceMinioS3BucketImportState(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) ([]*schema.ResourceData, error) {

	conn := meta.(*S3MinioClient).S3Client
	pol, err := conn.GetBucketPolicy(ctx, d.Id())
	if err != nil {
		return nil, fmt.Errorf("Error importing Minio S3 bucket policy: %s", err)
	}
	if pol == "" {
		return []*schema.ResourceData{d}, nil
	}

	var bucketPolicy BucketPolicy
	err = json.Unmarshal([]byte(pol), &bucketPolicy)
	if err != nil {
		return nil, fmt.Errorf("Error importing Minio S3 bucket policy: %s", err)
	}

	policyName := policy.GetPolicy(bucketPolicy.Statements, d.Id(), "")

	_ = d.Set("acl", policyName)

	return []*schema.ResourceData{d}, nil
}
