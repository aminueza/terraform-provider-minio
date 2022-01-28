package minio

import (
	"context"
	"fmt"

	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7/pkg/policy"
)

func resourceMinioS3BucketImportState(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) ([]*schema.ResourceData, error) {

	if diag := minioReadBucket(ctx, d, meta); diag.HasError() {
		return nil, fmt.Errorf("Could not read minio bucket")
	}

	bucketConfig := BucketConfig(d, meta)

	conn := meta.(*S3MinioClient).S3Client
	pol, err := conn.GetBucketPolicy(ctx, d.Id())
	if err != nil {
		return nil, fmt.Errorf("Error importing Minio S3 bucket policy: %s", err)
	}
	if pol == "" {
		_ = d.Set("acl", "private")
		return []*schema.ResourceData{d}, nil
	}

	_ = d.Set("acl", policyToACLName(bucketConfig, pol))

	return []*schema.ResourceData{d}, nil
}

func policyToACLName(bucketConfig *S3MinioBucket, pol string) string {

	defaultPolicies := map[string]string{
		"private":           exportPolicyString(ReadOnlyPolicy(bucketConfig), bucketConfig.MinioBucket),
		"public-read":       exportPolicyString(PublicReadPolicy(bucketConfig), bucketConfig.MinioBucket),
		"public-write":      exportPolicyString(WriteOnlyPolicy(bucketConfig), bucketConfig.MinioBucket),
		"public-read-write": exportPolicyString(ReadWritePolicy(bucketConfig), bucketConfig.MinioBucket),
		"public":            exportPolicyString(PublicPolicy(bucketConfig), bucketConfig.MinioBucket),
	}

	for name, defaultPolicy := range defaultPolicies {
		if equivalent, err := awspolicy.PoliciesAreEquivalent(defaultPolicy, pol); err == nil && equivalent {
			return name
		}
	}

	return "private"
}

func policyNameToACLBucket(policyName string) string {

	policyMapping := map[string]string{
		policy.BucketPolicyReadOnly:     "public-read",
		policy.BucketPolicyWriteOnly:    "public-write",
		policy.BucketPolicyReadWrite:    "public-read-write",
		string(policy.BucketPolicyNone): "private",
		"":                              "private",
	}

	x, ok := policyMapping[policyName]
	if !ok {
		return "custom"
	}
	return x
}
