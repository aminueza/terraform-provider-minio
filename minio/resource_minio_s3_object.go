package minio

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		CustomizeDiff: minioObjectCustomizeDiff,
		UpdateContext: minioUpdateObject,
		DeleteContext: minioDeleteObject,
		Importer: &schema.ResourceImporter{
			StateContext: minioImportObject,
		},

		SchemaVersion: 0,

		Schema: map[string]*schema.Schema{
			"bucket_name": {
				Type:         schema.TypeString,
				Description:  "Name of the bucket",
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.NoZeroValues,
			},
			"object_name": {
				Type:         schema.TypeString,
				Description:  "Name of the object",
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.NoZeroValues,
			},
			"content_type": {
				Type:        schema.TypeString,
				Description: "Content type of the object, in the form of a MIME type",
				Optional:    true,
				Computed:    true,
			},
			"source": {
				Type:          schema.TypeString,
				Description:   "Path to the file that will be uploaded. Use only one of content, content_base64, or source",
				Optional:      true,
				ConflictsWith: []string{"content", "content_base64"},
			},
			"content": {
				Type:          schema.TypeString,
				Description:   "Content of the object as a string. Use only one of content, content_base64, or source",
				Optional:      true,
				ConflictsWith: []string{"source", "content_base64"},
			},
			"content_base64": {
				Type:          schema.TypeString,
				Description:   "Base64-encoded content of the object. Use only one of content, content_base64, or source",
				Optional:      true,
				ConflictsWith: []string{"source", "content"},
			},
			"etag": {
				Type:        schema.TypeString,
				Description: "ETag of the object",
				Optional:    true,
				Computed:    true,
			},
			"version_id": {
				Type:        schema.TypeString,
				Description: "Version ID of the object",
				Optional:    true,
				Computed:    true,
			},
			"acl": {
				Type:        schema.TypeString,
				Description: "The canned ACL to apply to the object. Valid values: private, public-read, public-read-write, authenticated-read",
				Optional:    true,
				Default:     "private",
				ValidateFunc: validation.StringInSlice([]string{
					"private",
					"public-read",
					"public-read-write",
					"authenticated-read",
				}, false),
			},
			"metadata": {
				Type:             schema.TypeMap,
				Optional:         true,
				Elem:             &schema.Schema{Type: schema.TypeString},
				DiffSuppressFunc: suppressCaseInsensitiveMapDiff("metadata"),
			},
			"cache_control": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"content_disposition": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"content_encoding": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"expires": {
				Type:             schema.TypeString,
				Optional:         true,
				ValidateFunc:     validation.IsRFC3339Time,
				DiffSuppressFunc: suppressTimeDiffs,
			},
			"storage_class": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ValidateFunc: validation.StringInSlice(
					[]string{"STANDARD", "REDUCED_REDUNDANCY", "ONEZONE_IA", "INTELLIGENT_TIERING"}, false),
			},
		},
	}
}

func minioCreateObject(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return minioPutObject(ctx, d, meta)
}

// Content written outside Terraform never changes the configuration, so the
// plan stays empty and the drift is never reverted. Compare the MD5 of the
// configured content with the ETag stored in state, and force an update on a
// mismatch. See issue #1330.
func minioObjectCustomizeDiff(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
	// GetChange returns the stored ETag, not one set explicitly in the
	// configuration, so the comparison always looks at what the server holds.
	storedETagValue, _ := d.GetChange("etag")
	storedETag := strings.Trim(storedETagValue.(string), `"`)

	// The comparison only applies to ETags that are a bare MD5: on create
	// there is no stored ETag yet, multipart uploads carry a -N suffix, and
	// SSE-KMS/SSE-C objects are not returned as the MD5 of the content. The
	// schema has no encryption field, so the shape of the ETag is the only
	// thing that is observable here.
	if len(storedETag) != 32 {
		return nil
	}
	for _, c := range storedETag {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return nil
		}
	}

	h := md5.New()
	if v, ok := d.GetOk("source"); ok {
		source := v.(string)
		path, err := homedir.Expand(source)
		if err != nil {
			return fmt.Errorf("expanding homedir in source (%s): %w", source, err)
		}
		path = filepath.Clean(path)
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening S3 object source (%s): %w", path, err)
		}
		defer file.Close()
		if _, err := io.Copy(h, file); err != nil {
			return fmt.Errorf("hashing S3 object source (%s): %w", path, err)
		}
	} else if v, ok := d.GetOk("content"); ok {
		h.Write([]byte(v.(string)))
	} else if v, ok := d.GetOk("content_base64"); ok {
		raw, err := base64.StdEncoding.DecodeString(v.(string))
		if err != nil {
			return fmt.Errorf("decoding content_base64: %w", err)
		}
		h.Write(raw)
	} else {
		return nil
	}

	expected := fmt.Sprintf("%x", h.Sum(nil))
	if strings.EqualFold(expected, storedETag) {
		return nil
	}

	// The stored object differs from the configured content: mark the ETag as
	// changed so the plan is not empty and the next apply re-uploads.
	return d.SetNew("etag", fmt.Sprintf("%q", expected))
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
				tflog.Warn(ctx, fmt.Sprintf("Error closing S3 object source (%s): %s", path, err))
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
	if v, ok := d.GetOk("cache_control"); ok {
		options.CacheControl = v.(string)
	}
	if v, ok := d.GetOk("content_disposition"); ok {
		options.ContentDisposition = v.(string)
	}
	if v, ok := d.GetOk("content_encoding"); ok {
		options.ContentEncoding = v.(string)
	}
	if v, ok := d.GetOk("expires"); ok {
		t, err := time.Parse(time.RFC3339, v.(string))
		if err != nil {
			return NewResourceError("parsing expires", d.Id(), err)
		}
		options.Expires = t
	}
	if v, ok := d.GetOk("storage_class"); ok {
		options.StorageClass = v.(string)
	}
	if v, ok := d.GetOk("metadata"); ok {
		metadata := make(map[string]string)
		for k, val := range v.(map[string]interface{}) {
			metadata[k] = val.(string)
		}
		options.UserMetadata = metadata
	}

	// Set ACL via x-amz-acl header
	if acl := d.Get("acl").(string); acl != "" && acl != "private" {
		if options.UserMetadata == nil {
			options.UserMetadata = make(map[string]string)
		}
		options.UserMetadata["x-amz-acl"] = acl
	}

	_, err := m.S3Client.PutObject(
		ctx,
		d.Get("bucket_name").(string),
		d.Get("object_name").(string),
		body, -1,
		options,
	)

	if err != nil {
		return NewResourceError("putting object failed", d.Id(), s3CompatWriteError(m, "object", err))
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
		if s3CompatReadUnsupported(ctx, m, s3Compat405IsAnAnswer, d, "object", err) {
			return nil
		}
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
	if err := d.Set("content_encoding", objInfo.ContentEncoding); err != nil {
		return NewResourceError("reading object failed", d.Id(), err)
	}
	storageClass := objInfo.StorageClass
	if storageClass == "" {
		storageClass = "STANDARD"
	}
	if err := d.Set("storage_class", storageClass); err != nil {
		return NewResourceError("reading object failed", d.Id(), err)
	}

	if v := objInfo.Metadata.Get("Cache-Control"); v != "" {
		_ = d.Set("cache_control", v)
	}
	if v := objInfo.Metadata.Get("Content-Disposition"); v != "" {
		_ = d.Set("content_disposition", v)
	}
	if !objInfo.Expires.IsZero() {
		_ = d.Set("expires", objInfo.Expires.Format(time.RFC3339))
	}

	userMeta := make(map[string]string)
	for k, v := range objInfo.UserMetadata {
		lower := strings.ToLower(k)
		if lower == "x-amz-acl" || lower == "content-type" {
			continue
		}
		userMeta[lower] = v
	}
	if len(userMeta) > 0 {
		_ = d.Set("metadata", userMeta)
	}

	return nil
}

func minioUpdateObject(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return minioPutObject(ctx, d, meta)
}

func minioImportObject(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	parts := strings.SplitN(d.Id(), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("unexpected import ID format (%q), expected bucket_name/object_name", d.Id())
	}

	_ = d.Set("bucket_name", parts[0])
	_ = d.Set("object_name", parts[1])
	d.SetId(parts[1])

	return []*schema.ResourceData{d}, nil
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
