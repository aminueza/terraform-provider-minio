package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
)

func dataSourceMinioS3BucketTags() *schema.Resource {
	return &schema.Resource{
		Description: "Reads tags from an existing S3 bucket.",
		ReadContext: dataSourceMinioS3BucketTagsRead,
		Schema: map[string]*schema.Schema{
			"bucket": {Type: schema.TypeString, Required: true},
			"tags": {
				Type:     schema.TypeMap,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func dataSourceMinioS3BucketTagsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient)
	bucket := d.Get("bucket").(string)

	var diags diag.Diagnostics
	tagsMap := map[string]string{}

	tagging, err := client.S3Client.GetBucketTagging(ctx, bucket)
	switch {
	case err == nil:
		tagsMap = tagging.ToMap()
	case minio.ToErrorResponse(err).Code == "NoSuchTagSet":
	case isS3CompatNotSupported(client, s3Compat405IsMissingFeature, err):
		diags = append(diags, dataSourceS3CompatGap(bucket, "tags", "bucket tagging"))
	default:
		return NewResourceError("reading bucket tags", bucket, err)
	}

	d.SetId(bucket)
	if err := d.Set("tags", tagsMap); err != nil {
		return NewResourceError("setting tags", bucket, err)
	}

	return diags
}
