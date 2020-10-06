package minio

import (
	"bytes"
	"context"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/minio/minio-go/v7"
	"io"
)

func resourceMinioObject() *schema.Resource {
	return &schema.Resource{
		Create: minioCreateObject,
		Read:   minioReadObject,
		Update: minioUpdateObject,
		Delete: minioDeleteObject,

		SchemaVersion: 0,

		Schema: map[string]*schema.Schema{
			"bucket_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.NoZeroValues,
			},
			"object_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.NoZeroValues,
			},
			"content_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"source": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"content", "content_base64"},
			},
			"content": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"source", "content_base64"},
			},
			"content_base64": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"source", "content"},
			},
			"etag": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"version_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
		//CustomizeDiff: customDiff,
	}
}

func minioCreateObject(d *schema.ResourceData, meta interface{}) error {
	return minioPutObject(d, meta)
}

func minioPutObject(d *schema.ResourceData, meta interface{}) error {
	m := meta.(*S3MinioClient)

	var body io.ReadSeeker

	if v, ok := d.GetOk("content"); ok {
		content := v.(string)
		body = bytes.NewReader([]byte(content))
	}

	_, err := m.S3Client.PutObject(
		context.Background(),
		d.Get("bucket_name").(string),
		d.Get("object_name").(string),
		body, -1,
		minio.PutObjectOptions{},
	)

	if err != nil {
		return err
	}

	d.SetId(d.Get("object_name").(string))

	return minioReadObject(d, meta)
}

func minioReadObject(d *schema.ResourceData, meta interface{}) error {

	m := meta.(*S3MinioClient)

	objInfo, err := m.S3Client.StatObject(
		context.Background(),
		d.Get("bucket_name").(string),
		d.Get("object_name").(string),
		minio.StatObjectOptions{},
	)

	if err != nil {
		if err.Error() == "The specified key does not exist." {
			d.SetId("")
			return nil
		}
		return err
	}

	d.Set("etag", objInfo.ETag)
	d.Set("version_id", objInfo.VersionID)

	return nil
}

func minioUpdateObject(d *schema.ResourceData, meta interface{}) error {
	return minioPutObject(d, meta)
}

func minioDeleteObject(d *schema.ResourceData, meta interface{}) error {

	m := meta.(*S3MinioClient)

	err := m.S3Client.RemoveObject(
		context.Background(),
		d.Get("bucket_name").(string),
		d.Get("object_name").(string),
		minio.RemoveObjectOptions{},
	)

	if err != nil {
		return err
	}

	return nil
}
