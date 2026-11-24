package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceMinioServerConfigEtcd() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages MinIO etcd configuration for federated deployments and external IAM storage.",
		CreateContext: minioServerConfigEtcdSet,
		ReadContext:   minioServerConfigEtcdRead,
		UpdateContext: minioServerConfigEtcdSet,
		DeleteContext: minioServerConfigEtcdDelete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"endpoints": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateConfigValue,
				Description:  "Comma-separated list of etcd endpoint URLs (e.g., \"http://etcd1:2379,http://etcd2:2379\").",
			},
			"path_prefix": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validateConfigValue,
				Description:  "Key prefix for MinIO data in etcd. Enables tenant isolation when set.",
			},
			"coredns_path": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validateConfigValue,
				Description:  "CoreDNS path for bucket DNS registration (default: \"/skydns\").",
			},
			// RequiredWith keeps a cert-only configuration out of the plan: the
			// readback would put the server's key path into state and the empty
			// config value would then clear it, and the etcd builder omits empty
			// params, so the pair could never converge. See issue #1439. The
			// key alone stays allowed: removing the certificate while the key
			// remains is how the pair gets cleared.
			"client_cert": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateConfigValue,
				RequiredWith: []string{"client_cert_key"},
				Description:  "Path to client TLS certificate for etcd mTLS.",
			},
			"client_cert_key": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				ValidateFunc: validateConfigValue,
				Description:  "Path to client TLS private key for etcd mTLS.",
			},
			"comment": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validateConfigValue,
				Description:  "Optional comment for the etcd configuration.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether a MinIO server restart is required to apply the configuration.",
			},
		},
	}
}

func minioServerConfigEtcdSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	var parts []string
	addParam := func(key, val string) {
		if val != "" {
			parts = append(parts, buildConfigParam(key, val))
		}
	}

	addParam("endpoints", d.Get("endpoints").(string))
	if v, ok := d.GetOk("path_prefix"); ok {
		addParam("path_prefix", v.(string))
	}
	if v, ok := d.GetOk("coredns_path"); ok {
		addParam("coredns_path", v.(string))
	}
	if v, ok := d.GetOk("client_cert"); ok {
		addParam("client_cert", v.(string))
	}
	if v, ok := d.GetOk("client_cert_key"); ok {
		addParam("client_cert_key", v.(string))
	}
	if v, ok := d.GetOk("comment"); ok {
		addParam("comment", v.(string))
	}

	configString := "etcd " + strings.Join(parts, " ")
	restart, err := admin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("setting etcd configuration", "etcd", err)
	}

	d.SetId("etcd")
	_ = d.Set("restart_required", restart)
	tflog.Debug(ctx, fmt.Sprintf("Set etcd config (restart_required=%v)", restart))

	if restart {
		return nil
	}
	return minioServerConfigEtcdRead(ctx, d, meta)
}

func minioServerConfigEtcdRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	configData, err := admin.GetConfigKV(ctx, "etcd")
	if err != nil {
		return NewResourceError("reading etcd configuration", "etcd", err)
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, "etcd ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	d.SetId("etcd")

	if v, ok := cfgMap["endpoints"]; ok && v != "" {
		_ = d.Set("endpoints", v)
	}
	if v, ok := cfgMap["path_prefix"]; ok && v != "" {
		_ = d.Set("path_prefix", v)
	}
	if v, ok := cfgMap["coredns_path"]; ok && v != "" {
		_ = d.Set("coredns_path", v)
	}
	if v, ok := cfgMap["client_cert"]; ok && v != "" {
		_ = d.Set("client_cert", v)
	}
	if v, ok := cfgMap["client_cert_key"]; ok {
		if setErr := d.Set("client_cert_key", v); setErr != nil {
			return diag.FromErr(setErr)
		}
	}
	if v, ok := cfgMap["comment"]; ok && v != "" {
		_ = d.Set("comment", v)
	}

	return nil
}

func minioServerConfigEtcdDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	_, err := admin.DelConfigKV(ctx, "etcd")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			return NewResourceError("resetting etcd configuration", "etcd", err)
		}
	}
	d.SetId("")
	return nil
}
