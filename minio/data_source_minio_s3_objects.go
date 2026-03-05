package minio

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	miniogo "github.com/minio/minio-go/v7"
)

func dataSourceMinioS3Objects() *schema.Resource {
	return &schema.Resource{
		Description: "Lists objects in an S3 bucket with optional prefix, delimiter, and max keys filtering.",
		Read:        dataSourceMinioS3ObjectsRead,
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the bucket to list objects from.",
			},
			"prefix": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "Limits results to object keys that begin with this prefix.",
			},
			"delimiter": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "Character used to group keys. Use '/' to browse like a filesystem.",
			},
			"max_keys": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     1000,
				Description: "Maximum number of keys to return.",
			},
			"keys": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of object keys matching the filter.",
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"common_prefixes": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of common prefixes when using a delimiter (like subdirectories).",
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func dataSourceMinioS3ObjectsRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*S3MinioClient).S3Client
	ctx := context.Background()

	bucket := d.Get("bucket").(string)
	prefix := d.Get("prefix").(string)
	delimiter := d.Get("delimiter").(string)
	maxKeys := d.Get("max_keys").(int)

	opts := miniogo.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: delimiter == "",
	}

	var keys []string
	commonPrefixes := map[string]bool{}
	count := 0

	for obj := range client.ListObjects(ctx, bucket, opts) {
		if obj.Err != nil {
			return obj.Err
		}

		if delimiter != "" && obj.Key == "" {
			continue
		}

		if delimiter != "" && isCommonPrefix(obj.Key, prefix, delimiter) {
			cp := extractCommonPrefix(obj.Key, prefix, delimiter)
			if cp != "" {
				commonPrefixes[cp] = true
			}
			continue
		}

		keys = append(keys, obj.Key)
		count++
		if count >= maxKeys {
			break
		}
	}

	var cpList []string
	for cp := range commonPrefixes {
		cpList = append(cpList, cp)
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	_ = d.Set("keys", keys)
	_ = d.Set("common_prefixes", cpList)

	return nil
}

func isCommonPrefix(key, prefix, delimiter string) bool {
	remainder := key[len(prefix):]
	for i := 0; i < len(remainder); i++ {
		if string(remainder[i]) == delimiter {
			return true
		}
	}
	return false
}

func extractCommonPrefix(key, prefix, delimiter string) string {
	remainder := key[len(prefix):]
	for i := 0; i < len(remainder); i++ {
		if string(remainder[i]) == delimiter {
			return prefix + remainder[:i+1]
		}
	}
	return ""
}
