package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioS3BucketAnonymousAccess() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the anonymous access policy for an existing MinIO bucket, returning the raw policy JSON and the derived canned access type when the policy matches one of the known canned forms.",
		ReadContext: dataSourceMinioS3BucketAnonymousAccessRead,
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the bucket",
			},
			"policy": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Anonymous access policy JSON for the bucket",
			},
			"access_type": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Canned access type if the policy matches one of public, public-read, public-read-write, or public-write; otherwise empty",
			},
		},
	}
}

func dataSourceMinioS3BucketAnonymousAccessRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	bucket := d.Get("bucket").(string)

	policy, err := client.S3Client.GetBucketPolicy(ctx, bucket)
	if err != nil {
		if isNoSuchBucketError(err) {
			d.SetId("")
			return nil
		}
		return NewResourceError("reading anonymous access policy", bucket, err)
	}

	d.SetId(bucket)

	if err := d.Set("policy", policy); err != nil {
		return NewResourceError("setting policy", bucket, err)
	}

	accessType, err := getAccessTypeFromPolicy(policy, bucket, client)
	if err != nil {
		return NewResourceError("determining access_type", bucket, err)
	}

	if err := d.Set("access_type", accessType); err != nil {
		return NewResourceError("setting access_type", bucket, err)
	}

	return nil
}
