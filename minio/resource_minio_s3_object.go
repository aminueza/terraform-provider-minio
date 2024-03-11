package minio

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7"
	"github.com/mitchellh/go-homedir"
)

func resourceMinioObject() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateObject,
		ReadContext:   minioReadObject,
		UpdateContext: minioUpdateObject,
		DeleteContext: minioDeleteObject,

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

func minioCreateObject(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return minioPutObject(ctx, d, meta)
}

func minioPutObject(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	m := meta.(*S3MinioClient)

	var body io.ReadSeeker

	if v, ok := d.GetOk("source"); ok {
		source := v.(string)
		path, err := homedir.Expand(source)
		if err != nil {
			return NewResourceError(fmt.Sprintf("expanding homedir in source (%s)", source), d.Id(), err)
		}
		path = filepath.Clean(path)
		file, err := os.Open(path)
		if err != nil {
			return NewResourceError(fmt.Sprintf("opening S3 object source (%s)", path), d.Id(), err)
		}

		body = file
		defer func() {
			err := file.Close()
			if err != nil {
				log.Printf("[WARN] Error closing S3 object source (%s): %s", path, err)
			}
		}()
	} else if v, ok := d.GetOk("content"); ok {
		content := v.(string)
		body = bytes.NewReader([]byte(content))
	} else if v, ok := d.GetOk("content_base64"); ok {
		content := v.(string)
		contentRaw, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return NewResourceError("error decoding content_base64", d.Id(), err)
		}
		body = bytes.NewReader(contentRaw)
	} else {
		return NewResourceError("putting object failed", d.Id(), errors.New("one of source / content / content_base64 is not set"))
	}

	options := minio.PutObjectOptions{}
	if v, ok := d.GetOk("content_type"); ok {
		options.ContentType = v.(string)
	}

	_, err := m.S3Client.PutObject(
		ctx,
		d.Get("bucket_name").(string),
		d.Get("object_name").(string),
		body, -1,
		options,
	)

	if err != nil {
		return NewResourceError("putting object failed", d.Id(), err)
	}

	d.SetId(d.Get("object_name").(string))

	return minioReadObject(ctx, d, meta)
}

func minioReadObject(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	m := meta.(*S3MinioClient)

	objInfo, err := m.S3Client.StatObject(
		ctx,
		d.Get("bucket_name").(string),
		d.Get("object_name").(string),
		minio.StatObjectOptions{},
	)

	if err != nil {
		if err.Error() == "The specified key does not exist." {
			d.SetId("")
			return nil
		}
		return NewResourceError("reading object failed", d.Id(), err)
	}

	if err := d.Set("etag", objInfo.ETag); err != nil {
		return NewResourceError("reading object failed", d.Id(), err)
	}
	if err := d.Set("version_id", objInfo.VersionID); err != nil {
		return NewResourceError("reading object failed", d.Id(), err)
	}
	if err := d.Set("content_type", objInfo.ContentType); err != nil {
		return NewResourceError("reading object failed", d.Id(), err)
	}

	return nil
}

func minioUpdateObject(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return minioPutObject(ctx, d, meta)
}

func minioDeleteObject(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	m := meta.(*S3MinioClient)

	err := m.S3Client.RemoveObject(
		ctx,
		d.Get("bucket_name").(string),
		d.Get("object_name").(string),
		minio.RemoveObjectOptions{},
	)

	if err != nil {
		return NewResourceError("deleting object failed", d.Id(), err)
	}

	return nil
}
