package minio

import (
	"bytes"
	"context"
	"crypto/md5" //#nosec G501 -- reproduces the S3 ETag of a single-part upload; nothing is protected by it
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
// mismatch. An overwrite whose ETag is not a bare MD5 (a multipart or
// streamed upload) is not detected. See issue #1330.
func minioObjectCustomizeDiff(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
	// GetChange returns the stored ETag, not one set explicitly in the
	// configuration, so the comparison always looks at what the server holds.
	storedETagValue, _ := d.GetChange("etag")
	storedETag := strings.Trim(storedETagValue.(string), `"`)

	// The comparison only applies to ETags that are a bare MD5: on create
	// there is no stored ETag yet, and multipart or streamed uploads carry
	// a -N suffix.
	if len(storedETag) != 32 {
		return nil
	}
	for _, c := range storedETag {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return nil
		}
	}

	h := md5.New() //#nosec G401 -- reproduces the S3 ETag of a single-part upload; nothing is protected by it
	if v, ok := d.GetOk("source"); ok {
		source := v.(string)
		path, err := homedir.Expand(source)
		if err != nil {
			return fmt.Errorf("expanding homedir in source (%s): %w", source, err)
		}
		path = filepath.Clean(path)
		file, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				// The file may not exist until the apply itself, for
				// example when it is produced by another resource: skip
				// the comparison instead of failing the plan.
				return nil
			}
			return fmt.Errorf("opening S3 object source (%s): %w", path, err)
		}
		defer func() {
			if err := file.Close(); err != nil {
				tflog.Warn(ctx, fmt.Sprintf("Error closing S3 object source (%s): %s", path, err))
			}
		}()
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

	// The stored object differs from the configured content: mark the ETag for
	// recomputation so the plan is not empty and the next apply re-uploads.
	return d.SetNewComputed("etag")
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

	bucketName := d.Get("bucket_name").(string)
	objectName := d.Get("object_name").(string)
	objectID := s3ObjectID(bucketName, objectName)

	_, err := m.S3Client.PutObject(
		ctx,
		bucketName,
		objectName,
		body, -1,
		options,
	)

	if err != nil {
		return NewResourceError("putting object failed", objectID, s3CompatWriteError(m, "object", err))
	}

	d.SetId(objectID)

	return minioReadObject(ctx, d, meta)
}

func minioReadObject(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	m := meta.(*S3MinioClient)
	bucketName := d.Get("bucket_name").(string)
	objectName := d.Get("object_name").(string)
	if bucketName == "" && objectName == "" {
		// Only a hand-corrupted state reaches this: import and normal reads
		// always have both attributes. Recover them from the ID so the state
		// ends self-consistent instead of diffing a ForceNew attribute.
		parts := strings.SplitN(d.Id(), "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return diag.Errorf("unexpected object ID format (%q), expected bucket_name/object_name", d.Id())
		}
		bucketName, objectName = parts[0], parts[1]
		if err := d.Set("bucket_name", bucketName); err != nil {
			return NewResourceError("setting bucket name", d.Id(), err)
		}
		if err := d.Set("object_name", objectName); err != nil {
			return NewResourceError("setting object name", d.Id(), err)
		}
	} else if bucketName == "" || objectName == "" {
		// A legacy ID is a bare object name: parsing it against the one
		// present attribute could StatObject a wrong key and silently drop
		// the resource, so refuse to guess.
		return diag.Errorf("state has bucket_name (%q) and object_name (%q); both must be set, or both empty to recover from a bucket/object ID", bucketName, objectName)
	}
	objectID := s3ObjectID(bucketName, objectName)

	objInfo, err := m.S3Client.StatObject(
		ctx,
		bucketName,
		objectName,
		minio.StatObjectOptions{},
	)

	if err != nil {
		if s3CompatReadUnsupported(ctx, m, s3Compat405IsAnAnswer, d, "object", err) {
			d.SetId(objectID)
			return nil
		}
		if err.Error() == "The specified key does not exist." {
			d.SetId("")
			return nil
		}
		return NewResourceError("reading object failed", objectID, err)
	}

	d.SetId(objectID)

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
		if diags := setResourceField(d, "cache_control", v, d.Id()); diags != nil {
			return diags
		}
	}
	if v := objInfo.Metadata.Get("Content-Disposition"); v != "" {
		if diags := setResourceField(d, "content_disposition", v, d.Id()); diags != nil {
			return diags
		}
	}
	if !objInfo.Expires.IsZero() {
		if diags := setResourceField(d, "expires", objInfo.Expires.Format(time.RFC3339), d.Id()); diags != nil {
			return diags
		}
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
		if diags := setResourceField(d, "metadata", userMeta, d.Id()); diags != nil {
			return diags
		}
	}

	return nil
}

func minioUpdateObject(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return minioPutObject(ctx, d, meta)
}

// s3ObjectID builds the resource ID. Bucket names cannot contain "/", so the
// first "/" always delimits the bucket from the object key, which may.
func s3ObjectID(bucketName, objectName string) string {
	return fmt.Sprintf("%s/%s", bucketName, objectName)
}

func minioImportObject(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	parts := strings.SplitN(d.Id(), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("unexpected import ID format (%q), expected bucket_name/object_name", d.Id())
	}

	objectID := s3ObjectID(parts[0], parts[1])
	if err := d.Set("bucket_name", parts[0]); err != nil {
		return nil, errors.New(NewResourceErrorStr("setting bucket_name", objectID, err))
	}
	if err := d.Set("object_name", parts[1]); err != nil {
		return nil, errors.New(NewResourceErrorStr("setting object_name", objectID, err))
	}
	d.SetId(objectID)

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
