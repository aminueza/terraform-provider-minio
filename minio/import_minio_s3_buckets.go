package minio

import (
	"context"
	"fmt"
	"net/http"

	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
)

func resourceMinioS3BucketImportState(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) ([]*schema.ResourceData, error) {

	bucket := d.Id()

	if diag := minioReadBucket(ctx, d, meta); diag.HasError() {
		return nil, fmt.Errorf("could not read minio bucket")
	}

	// minioReadBucket clears the id instead of failing when the bucket is gone, which
	// would send every call below with an empty bucket name. See issue #1206.
	if d.Id() == "" {
		return nil, fmt.Errorf("cannot import bucket %q: the bucket does not exist", bucket)
	}

	bucketConfig := BucketConfig(d, meta)

	conn := meta.(*S3MinioClient).S3Client

	bucketObjectLocking, _, _, _, err := conn.GetObjectLockConfig(ctx, d.Id())
	object_locking := err == nil && bucketObjectLocking == "Enabled"
	_ = d.Set("object_locking", object_locking)

	pol, err := conn.GetBucketPolicy(ctx, d.Id())
	if err != nil {
		// A backend without bucket policies, such as Garage, can only hold the default,
		// so the bucket is private and the import can go on. See issue #1206.
		errResp := minio.ToErrorResponse(err)
		if errResp.Code != "NotImplemented" && errResp.StatusCode != http.StatusNotImplemented {
			return nil, fmt.Errorf("error importing Minio S3 bucket policy: %s", err)
		}
		pol = ""
	}

	if pol == "" {
		_ = d.Set("acl", "private")
		return []*schema.ResourceData{d}, nil
	}

	_ = d.Set("acl", policyToACLName(ctx, bucketConfig, pol))

	return []*schema.ResourceData{d}, nil
}

// policyToACLName names the ACL that would produce this policy. `public-read-write` builds the
// same policy as `public`, so it is not listed: the shared shape is reported as `public`, and an
// ordered list keeps that answer stable where map iteration would not.
func policyToACLName(ctx context.Context, bucketConfig *S3MinioBucket, pol string) string {
	defaultPolicies := []struct {
		acl    string
		policy string
	}{
		{"public-read", exportPolicyString(ctx, ReadOnlyPolicy(bucketConfig), bucketConfig.MinioBucket)},
		{"public-write", exportPolicyString(ctx, WriteOnlyPolicy(bucketConfig), bucketConfig.MinioBucket)},
		{"public", exportPolicyString(ctx, PublicPolicy(bucketConfig), bucketConfig.MinioBucket)},
	}

	for _, defaultPolicy := range defaultPolicies {
		if equivalent, err := awspolicy.PoliciesAreEquivalent(defaultPolicy.policy, pol); err == nil && equivalent {
			return defaultPolicy.acl
		}
	}

	return "private"
}
