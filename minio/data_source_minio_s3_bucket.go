package minio

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
)

func dataSourceMinioS3Bucket() *schema.Resource {
	return &schema.Resource{
		Description: "Reads properties of an existing S3 bucket including versioning, region, and object lock status.",
		ReadContext: dataSourceMinioS3BucketRead,
		Schema: map[string]*schema.Schema{
			"bucket":             {Type: schema.TypeString, Required: true},
			"region":             {Type: schema.TypeString, Computed: true},
			"versioning_enabled": {Type: schema.TypeBool, Computed: true},
			"object_lock_enabled": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"policy": {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceMinioS3BucketRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	s3Client := client.S3Client

	bucket := d.Get("bucket").(string)

	region, err := s3Client.GetBucketLocation(ctx, bucket)
	if err != nil {
		return NewResourceError("reading bucket region", bucket, err)
	}

	d.SetId(bucket)

	if err := d.Set("region", region); err != nil {
		return NewResourceError("setting region", bucket, err)
	}

	var diags diag.Diagnostics

	versioningEnabled := false
	versioning, err := s3Client.GetBucketVersioning(ctx, bucket)
	switch {
	case err == nil:
		versioningEnabled = versioning.Enabled()
	case isS3CompatNotSupported(client, s3Compat405IsMissingFeature, err):
		diags = append(diags, dataSourceS3CompatGap(bucket, "versioning_enabled", "bucket versioning"))
	default:
		return NewResourceError("reading bucket versioning", bucket, err)
	}
	if err := d.Set("versioning_enabled", versioningEnabled); err != nil {
		return NewResourceError("setting versioning_enabled", bucket, err)
	}

	objectLockEnabled := false
	lockConfig, _, _, _, err := s3Client.GetObjectLockConfig(ctx, bucket)
	switch {
	case err == nil:
		objectLockEnabled = lockConfig == "Enabled"
	case minio.ToErrorResponse(err).Code == "ObjectLockConfigurationNotFoundError":
	case isS3CompatNotSupported(client, s3Compat405IsMissingFeature, err):
		diags = append(diags, dataSourceS3CompatGap(bucket, "object_lock_enabled", "object lock"))
	default:
		return NewResourceError("reading bucket object lock configuration", bucket, err)
	}
	if err := d.Set("object_lock_enabled", objectLockEnabled); err != nil {
		return NewResourceError("setting object_lock_enabled", bucket, err)
	}

	policy, err := s3Client.GetBucketPolicy(ctx, bucket)
	switch {
	case err == nil:
	case isS3CompatNotSupported(client, s3Compat405IsMissingFeature, err):
		diags = append(diags, dataSourceS3CompatGap(bucket, "policy", "bucket policies"))
	default:
		return NewResourceError("reading bucket policy", bucket, err)
	}
	if err := d.Set("policy", policy); err != nil {
		return NewResourceError("setting policy", bucket, err)
	}

	return diags
}

// dataSourceS3CompatGap tells the user that an attribute holds its zero value
// because the backend never answered, not because the bucket has that setting.
// A data source has no earlier state to keep, so unlike a resource read it can
// only report the gap. See issue #1207.
func dataSourceS3CompatGap(bucket, attribute, feature string) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.Warning,
		Summary:  fmt.Sprintf("%s is not supported by this S3 backend", feature),
		Detail: fmt.Sprintf(
			"The backend serving bucket %q does not implement %s, so %s holds its zero value rather than the bucket's setting. This is reported because s3_compat_mode is true; set it to false to make this an error.",
			bucket, feature, attribute,
		),
	}
}
