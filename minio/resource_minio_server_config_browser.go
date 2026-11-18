package minio

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const serverConfigBrowserKey = "browser"

const (
	serverConfigBrowserCspPolicy             = "csp_policy"
	serverConfigBrowserHstsSeconds           = "hsts_seconds"
	serverConfigBrowserHstsIncludeSubdomains = "hsts_include_subdomains"
	serverConfigBrowserHstsPreload           = "hsts_preload"
	serverConfigBrowserReferrerPolicy        = "referrer_policy"
)

// browserReferrerPolicies is the fixed list of referrer policy values
// browser.LookupConfig accepts; any other value makes the server fail to
// apply the browser configuration on the next restart.
var browserReferrerPolicies = []string{
	"no-referrer",
	"no-referrer-when-downgrade",
	"origin",
	"origin-when-cross-origin",
	"same-origin",
	"strict-origin",
	"strict-origin-when-cross-origin",
	"unsafe-url",
}

func resourceMinioServerConfigBrowser() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages the security headers of the MinIO console. The browser subsystem controls the Content-Security-Policy, Strict-Transport-Security and Referrer-Policy headers served with console responses.",
		CreateContext: minioServerConfigBrowserSet,
		ReadContext:   minioServerConfigBrowserRead,
		UpdateContext: minioServerConfigBrowserSet,
		DeleteContext: minioServerConfigBrowserDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"csp_policy": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateConfigValue,
				Description:  "Value of the Content-Security-Policy response header, for example \"default-src 'self' https://console.example.com\". An empty value clears the policy and the server uses its built-in default.",
			},
			"hsts_seconds": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "max-age in seconds of the Strict-Transport-Security response header. The server only sends the header while this value is greater than 0; 0 keeps the server default.",
			},
			"hsts_include_subdomains": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Whether the Strict-Transport-Security response header carries the includeSubDomains directive.",
			},
			"hsts_preload": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Whether the Strict-Transport-Security response header carries the preload directive.",
			},
			"referrer_policy": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateBrowserReferrerPolicy,
				Description:  "Value of the Referrer-Policy response header. One of: no-referrer, no-referrer-when-downgrade, origin, origin-when-cross-origin, same-origin, strict-origin, strict-origin-when-cross-origin, unsafe-url. An empty value clears the policy and the server uses its built-in default.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether a MinIO server restart is required.",
			},
		},
	}
}

func minioServerConfigBrowserSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigBrowserConfig(d, meta)

	tflog.Debug(ctx, "Setting browser configuration")

	cfgData := buildServerConfigBrowserCfgData(config)
	restart, err := config.MinioAdmin.SetConfigKV(ctx, serverConfigBrowserKey+" "+cfgData)
	if err != nil {
		return NewResourceError("setting browser configuration", serverConfigBrowserKey, err)
	}

	d.SetId(serverConfigBrowserKey)
	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", serverConfigBrowserKey, setErr)
	}

	tflog.Debug(ctx, fmt.Sprintf("Set browser config (restart_required=%v)", restart))

	return minioServerConfigBrowserRead(ctx, d, meta)
}

func minioServerConfigBrowserRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigBrowserConfig(d, meta)

	tflog.Debug(ctx, "Reading browser configuration")

	configData, err := config.MinioAdmin.GetConfigKV(ctx, serverConfigBrowserKey)
	if err != nil {
		return NewResourceError("reading browser configuration", serverConfigBrowserKey, err)
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, serverConfigBrowserKey+" ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	d.SetId(serverConfigBrowserKey)

	if v, ok := cfgMap["csp_policy"]; ok {
		if setErr := d.Set("csp_policy", v); setErr != nil {
			return NewResourceError("setting csp_policy", serverConfigBrowserKey, setErr)
		}
	}
	if v, ok := cfgMap["hsts_seconds"]; ok {
		seconds := 0
		if v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return NewResourceError("parsing hsts_seconds from the server", serverConfigBrowserKey, err)
			}
			seconds = n
		}
		if setErr := d.Set("hsts_seconds", seconds); setErr != nil {
			return NewResourceError("setting hsts_seconds", serverConfigBrowserKey, setErr)
		}
	}
	if v, ok := cfgMap["hsts_include_subdomains"]; ok {
		if setErr := d.Set("hsts_include_subdomains", v == "on"); setErr != nil {
			return NewResourceError("setting hsts_include_subdomains", serverConfigBrowserKey, setErr)
		}
	}
	if v, ok := cfgMap["hsts_preload"]; ok {
		if setErr := d.Set("hsts_preload", v == "on"); setErr != nil {
			return NewResourceError("setting hsts_preload", serverConfigBrowserKey, setErr)
		}
	}
	if v, ok := cfgMap["referrer_policy"]; ok {
		if setErr := d.Set("referrer_policy", v); setErr != nil {
			return NewResourceError("setting referrer_policy", serverConfigBrowserKey, setErr)
		}
	}

	return nil
}

func minioServerConfigBrowserDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigBrowserConfig(d, meta)

	tflog.Debug(ctx, "Deleting browser configuration")

	_, err := config.MinioAdmin.DelConfigKV(ctx, serverConfigBrowserKey)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			return NewResourceError("resetting browser configuration", serverConfigBrowserKey, err)
		}
	}

	d.SetId("")
	tflog.Debug(ctx, "Deleted browser configuration")

	return nil
}

// buildServerConfigBrowserCfgData builds the key=value string sent to
// set-config-kv. The resource owns the singleton: a key the configuration
// leaves empty is sent as key= so the server drops whatever it holds for
// that key and falls back to its built-in default, including a value set
// outside Terraform. The bool keys carry a schema default, so they are
// always sent as on or off.
func buildServerConfigBrowserCfgData(config *S3MinioServerConfigBrowser) string {
	parts := make([]string, 0, 5)

	if config.CspPolicy != "" {
		parts = append(parts, buildConfigParam(serverConfigBrowserCspPolicy, config.CspPolicy))
	} else {
		parts = append(parts, serverConfigBrowserCspPolicy+"=")
	}

	if config.HstsSeconds > 0 {
		parts = append(parts, fmt.Sprintf("%s=%d", serverConfigBrowserHstsSeconds, config.HstsSeconds))
	} else {
		parts = append(parts, serverConfigBrowserHstsSeconds+"=")
	}

	if config.HstsIncludeSubdomains {
		parts = append(parts, serverConfigBrowserHstsIncludeSubdomains+"=on")
	} else {
		parts = append(parts, serverConfigBrowserHstsIncludeSubdomains+"=off")
	}

	if config.HstsPreload {
		parts = append(parts, serverConfigBrowserHstsPreload+"=on")
	} else {
		parts = append(parts, serverConfigBrowserHstsPreload+"=off")
	}

	if config.ReferrerPolicy != "" {
		parts = append(parts, buildConfigParam(serverConfigBrowserReferrerPolicy, config.ReferrerPolicy))
	} else {
		parts = append(parts, serverConfigBrowserReferrerPolicy+"=")
	}

	return strings.Join(parts, " ")
}

// validateBrowserReferrerPolicy accepts an empty value and the exact list of
// policies MinIO's browser.LookupConfig accepts. Any other value is rejected
// client-side because the server stores it and only fails to start when it
// applies the configuration on the next restart.
func validateBrowserReferrerPolicy(val interface{}, key string) (warns []string, errs []error) {
	v, _ := val.(string)
	if v == "" {
		return
	}
	for _, allowed := range browserReferrerPolicies {
		if v == allowed {
			return
		}
	}
	errs = append(errs, fmt.Errorf("%q must be one of: %s (MinIO's browser subsystem rejects any other value on restart)", key, strings.Join(browserReferrerPolicies, ", ")))
	return
}
