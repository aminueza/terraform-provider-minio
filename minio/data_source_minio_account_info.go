package minio

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
)

func dataSourceMinioAccountInfo() *schema.Resource {
	return &schema.Resource{
		Description: "Returns storage usage and bucket information for the authenticated account.",
		ReadContext: dataSourceMinioAccountInfoRead,
		Schema: map[string]*schema.Schema{
			"account_name":  {Type: schema.TypeString, Computed: true, Description: "Name of the authenticated account."},
			"bucket_count":  {Type: schema.TypeInt, Computed: true, Description: "Total number of buckets accessible to this account."},
			"total_size":    {Type: schema.TypeString, Computed: true, Description: "Total storage used across all buckets (bytes), as of MinIO's last scanner run."},
			"total_objects": {Type: schema.TypeString, Computed: true, Description: "Total object count across all buckets, as of MinIO's last scanner run."},
			"buckets": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name":    {Type: schema.TypeString, Computed: true},
						"size":    {Type: schema.TypeString, Computed: true},
						"objects": {Type: schema.TypeString, Computed: true},
					},
				},
			},
		},
	}
}

func dataSourceMinioAccountInfoRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	info, err := admin.AccountInfo(ctx, madmin.AccountOpts{})
	if err != nil {
		return NewResourceError("reading account info", "account_info", err)
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	if diags := setResourceField(d, "account_name", info.AccountName, "account_info"); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "bucket_count", len(info.Buckets), "account_info"); diags.HasError() {
		return diags
	}

	var totalSize uint64
	var totalObjects uint64
	var buckets []map[string]interface{}
	for _, b := range info.Buckets {
		totalSize += b.Size
		totalObjects += b.Objects
		buckets = append(buckets, map[string]interface{}{
			"name":    b.Name,
			"size":    strconv.FormatUint(b.Size, 10),
			"objects": strconv.FormatUint(b.Objects, 10),
		})
	}

	if diags := setResourceField(d, "total_size", strconv.FormatUint(totalSize, 10), "account_info"); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "total_objects", strconv.FormatUint(totalObjects, 10), "account_info"); diags.HasError() {
		return diags
	}
	if diags := setResourceField(d, "buckets", buckets, "account_info"); diags.HasError() {
		return diags
	}

	return nil
}

// RoundTrip passes every request through unchanged and rewrites only a
// successful accountinfo response. MinIO marshals a tagged bucket's
// details.tags with Go's default struct encoding, as
// {"XMLName":{...},"TagSet":{}}, while minio-go's tags.Tags.UnmarshalJSON
// accepts only a flat {"key":"value"} object, so a single tagged bucket makes
// madmin's AccountInfo fail for the whole account. The tag set's fields are
// unexported on the server, so that shape never carries tag data and is
// replaced with {}. Remove once the server or minio-go handles both shapes.
// See issue #1460.
func (t accountInfoTagsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusOK || !strings.HasSuffix(req.URL.Path, "/accountinfo") {
		return resp, err
	}

	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if fixed, changed := normalizeAccountInfoTags(body); changed {
		body = fixed
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return resp, nil
}

// normalizeAccountInfoTags replaces every Buckets[].details.tags value that is
// an object in the XML struct shape (it has an XMLName or TagSet key) with an
// empty object. It reports false and leaves the body to madmin when the body
// does not parse or holds nothing to replace. UseNumber keeps sizes and object
// counts above 2^53 exact through the round trip.
func normalizeAccountInfoTags(body []byte) ([]byte, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var doc map[string]interface{}
	if err := dec.Decode(&doc); err != nil {
		return nil, false
	}
	var buckets []interface{}
	for key, value := range doc {
		// madmin's AccountInfo.Buckets has no json tag and the server sends
		// "Buckets"; encoding/json matches field names case-insensitively.
		if strings.EqualFold(key, "buckets") {
			if list, ok := value.([]interface{}); ok {
				buckets = list
			}
		}
	}
	changed := false
	for _, b := range buckets {
		bucket, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		details, ok := bucket["details"].(map[string]interface{})
		if !ok {
			continue
		}
		tags, ok := details["tags"].(map[string]interface{})
		if !ok {
			continue
		}
		_, hasXMLName := tags["XMLName"]
		_, hasTagSet := tags["TagSet"]
		if hasXMLName || hasTagSet {
			details["tags"] = map[string]interface{}{}
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	fixed, err := json.Marshal(doc)
	if err != nil {
		return nil, false
	}
	return fixed, true
}
