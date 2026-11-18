package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const serverConfigPolicyPluginKey = "policy_plugin"

func resourceMinioServerConfigPolicyPlugin() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages the MinIO policy plugin configuration. While a URL is configured, MinIO sends every authorization decision to the external service at that URL (for example Open Policy Agent) instead of evaluating its own policies. The plugin only takes effect after a server restart, and while it is active every request is denied if the service is unavailable or answers no.",
		CreateContext: minioServerConfigPolicyPluginSet,
		ReadContext:   minioServerConfigPolicyPluginRead,
		UpdateContext: minioServerConfigPolicyPluginSet,
		DeleteContext: minioServerConfigPolicyPluginDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"url": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateConfigValue,
				Description:  "URL of the external authorization service. The plugin is active whenever this is non-empty.",
			},
			"auth_token": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				ValidateFunc: validateConfigValue,
				Description:  "Token sent as the Authorization header to the plugin endpoint. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"enable_http2": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Whether MinIO uses HTTP/2 to talk to the plugin endpoint.",
			},
			"comment": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateConfigValue,
				Description:  "Comment or description for the policy plugin configuration.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether a MinIO server restart is required.",
			},
		},
	}
}

func minioServerConfigPolicyPluginSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigPolicyPluginConfig(d, meta)

	tflog.Debug(ctx, "Setting policy plugin configuration")

	configString := serverConfigPolicyPluginKey + " " + buildServerConfigPolicyPluginCfgData(config)
	restart, err := config.MinioAdmin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("setting policy plugin configuration", serverConfigPolicyPluginKey, err)
	}

	d.SetId(serverConfigPolicyPluginKey)
	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", serverConfigPolicyPluginKey, setErr)
	}

	tflog.Debug(ctx, fmt.Sprintf("Set policy plugin config (restart_required=%v)", restart))

	return minioServerConfigPolicyPluginRead(ctx, d, meta)
}

func minioServerConfigPolicyPluginRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigPolicyPluginConfig(d, meta)

	tflog.Debug(ctx, "Reading policy plugin configuration")

	configData, err := config.MinioAdmin.GetConfigKV(ctx, serverConfigPolicyPluginKey)
	if err != nil {
		return NewResourceError("reading policy plugin configuration", serverConfigPolicyPluginKey, err)
	}

	configStr := strings.TrimSpace(string(configData))
	cfgMap, found := notifyTargetConfigParams(configStr, serverConfigPolicyPluginKey)
	d.SetId(serverConfigPolicyPluginKey)

	if !found || len(cfgMap) == 0 {
		// The response carried no keys for the subsystem, nothing can be
		// converged on; keep the state values.
		tflog.Warn(ctx, "Policy plugin response carried no keys, keeping state values")
		return nil
	}

	// The policy plugin is a non-dynamic subsystem: get-config-kv serves the
	// pre-restart config while a restart is pending, so a response that does
	// not match the configured values is stale and the state keeps them. Once
	// the server has restarted, the response matches and the flag is cleared.
	if d.Get("restart_required").(bool) {
		if !policyPluginRunningConfigMatchesState(cfgMap, config) {
			tflog.Warn(ctx, "Policy plugin still shows the pre-restart config, keeping configured values in state")
			return nil
		}
		if setErr := d.Set("restart_required", false); setErr != nil {
			return NewResourceError("clearing restart_required", serverConfigPolicyPluginKey, setErr)
		}
	}

	// The server redacts auth_token on read, so the state keeps the value
	// from the configuration.

	if v, ok := cfgMap["url"]; ok {
		if setErr := d.Set("url", v); setErr != nil {
			return NewResourceError("setting url", serverConfigPolicyPluginKey, setErr)
		}
	}

	// The server treats a missing enable_http2 key as off.
	enabled := false
	if v, ok := cfgMap["enable_http2"]; ok {
		enabled = v == "on"
	}
	if setErr := d.Set("enable_http2", enabled); setErr != nil {
		return NewResourceError("setting enable_http2", serverConfigPolicyPluginKey, setErr)
	}

	if v, ok := cfgMap["comment"]; ok {
		if setErr := d.Set("comment", v); setErr != nil {
			return NewResourceError("setting comment", serverConfigPolicyPluginKey, setErr)
		}
	}

	return nil
}

func minioServerConfigPolicyPluginDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigPolicyPluginConfig(d, meta)

	tflog.Debug(ctx, "Deleting policy plugin configuration")

	_, err := config.MinioAdmin.DelConfigKV(ctx, serverConfigPolicyPluginKey)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "there is no target") {
			return NewResourceError("resetting policy plugin configuration", serverConfigPolicyPluginKey, err)
		}
	}

	d.SetId("")
	tflog.Debug(ctx, "Deleted policy plugin configuration")

	return nil
}

// buildServerConfigPolicyPluginCfgData builds the key=value string sent to
// set-config-kv. The resource owns every key of the singleton: MinIO merges
// the sent keys into the stored config, so a key the configuration leaves
// empty is sent as key= and clears whatever the server holds, including a
// value set outside Terraform.
func buildServerConfigPolicyPluginCfgData(config *S3MinioServerConfigPolicyPlugin) string {
	parts := make([]string, 0, 4)

	parts = append(parts, buildConfigParam("url", config.URL))
	parts = append(parts, buildConfigParam("auth_token", config.AuthToken))
	if config.EnableHTTP2 {
		parts = append(parts, "enable_http2=on")
	} else {
		parts = append(parts, "enable_http2=off")
	}
	parts = append(parts, buildConfigParam("comment", config.Comment))

	return strings.Join(parts, " ")
}

// policyPluginRunningConfigMatchesState reports whether the values the server
// reports agree with what the provider wrote for the current state, the way
// the notification targets check their pre-restart config. The provider sends
// every key of the subsystem, so a key that disagrees is stale; a non-empty
// auth_token is redacted from the response and is never compared, while an
// empty one comes back as auth_token= and must agree.
func policyPluginRunningConfigMatchesState(cfgMap map[string]string, config *S3MinioServerConfigPolicyPlugin) bool {
	intended := parseConfigParams(buildServerConfigPolicyPluginCfgData(config))
	for key, serverValue := range cfgMap {
		if key == "auth_token" && intended["auth_token"] != "" {
			continue
		}
		if serverValue != intended[key] {
			return false
		}
	}
	return true
}
