package minio

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
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
	} else if v, ok := d.GetOk("content_base64"); ok {
		content := v.(string)
		contentRaw, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return fmt.Errorf("error decoding content_base64: %s", err)
		}
		body = bytes.NewReader(contentRaw)
	} else if _, ok := d.GetOk("content_base64"); ok {
		return errors.New("sorry, unsupported yet")
	} else {
		return errors.New("one of source / content / content_base64 is not set")
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

	if err := d.Set("etag", objInfo.ETag); err != nil {
		return err
	}
	if err := d.Set("version_id", objInfo.VersionID); err != nil {
		return err
	}

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
