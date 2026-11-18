package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const serverConfigCompressionKey = "compression"

func resourceMinioServerConfigCompression() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages the MinIO object compression configuration. MinIO transparently compresses objects whose extension or mime type matches the configured lists.",
		CreateContext: minioServerConfigCompressionSet,
		ReadContext:   minioServerConfigCompressionRead,
		UpdateContext: minioServerConfigCompressionSet,
		DeleteContext: minioServerConfigCompressionDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"enable": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Whether object compression is enabled.",
			},
			"extensions": {
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString, ValidateFunc: validateCompressionListItem},
				Description: "File extensions to compress, for example \".txt\", \".log\" or \".csv\". Removing the attribute leaves the server's last value; set the list to [] to clear it.",
			},
			"mime_types": {
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString, ValidateFunc: validateCompressionListItem},
				Description: "Wildcard mime types to compress, for example \"text/*\" or \"application/json\". Removing the attribute leaves the server's last value; set the list to [] to clear it.",
			},
			"allow_encryption": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Whether compression is allowed for encrypted objects.",
			},
			"comment": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateConfigValue,
				Description:  "Comment or description for the compression configuration.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether a MinIO server restart is required.",
			},
		},
	}
}

func minioServerConfigCompressionSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigCompressionConfig(d, meta)

	tflog.Debug(ctx, "Setting compression configuration")

	cfgData := buildServerConfigCompressionCfgData(d, config)
	restart, err := config.MinioAdmin.SetConfigKV(ctx, serverConfigCompressionKey+" "+cfgData)
	if err != nil {
		return NewResourceError("setting compression configuration", serverConfigCompressionKey, err)
	}

	d.SetId(serverConfigCompressionKey)
	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", serverConfigCompressionKey, setErr)
	}

	tflog.Debug(ctx, fmt.Sprintf("Set compression config (restart_required=%v)", restart))

	return minioServerConfigCompressionRead(ctx, d, meta)
}

func minioServerConfigCompressionRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigCompressionConfig(d, meta)

	tflog.Debug(ctx, "Reading compression configuration")

	configData, err := config.MinioAdmin.GetConfigKV(ctx, serverConfigCompressionKey)
	if err != nil {
		return NewResourceError("reading compression configuration", serverConfigCompressionKey, err)
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, serverConfigCompressionKey+" ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)
	d.SetId(serverConfigCompressionKey)

	if len(cfgMap) > 0 {
		// The server prints enable only when it is off (SubsysInfo.WriteTo),
		// so a response without it means compression is on. See issue #1317.
		enabled := true
		if v, ok := cfgMap["enable"]; ok {
			enabled = v != "off"
		}
		if setErr := d.Set("enable", enabled); setErr != nil {
			return NewResourceError("setting enable", serverConfigCompressionKey, setErr)
		}
	}

	if v, ok := cfgMap["allow_encryption"]; ok {
		if setErr := d.Set("allow_encryption", v == "on"); setErr != nil {
			return NewResourceError("setting allow_encryption", serverConfigCompressionKey, setErr)
		}
	}
	if v, ok := cfgMap["extensions"]; ok {
		if setErr := d.Set("extensions", compressionConfigList(v)); setErr != nil {
			return NewResourceError("setting extensions", serverConfigCompressionKey, setErr)
		}
	}
	if v, ok := cfgMap["mime_types"]; ok {
		if setErr := d.Set("mime_types", compressionConfigList(v)); setErr != nil {
			return NewResourceError("setting mime_types", serverConfigCompressionKey, setErr)
		}
	}
	if v, ok := cfgMap["comment"]; ok {
		if setErr := d.Set("comment", v); setErr != nil {
			return NewResourceError("setting comment", serverConfigCompressionKey, setErr)
		}
	}

	return nil
}

func minioServerConfigCompressionDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := ServerConfigCompressionConfig(d, meta)

	tflog.Debug(ctx, "Deleting compression configuration")

	_, err := config.MinioAdmin.DelConfigKV(ctx, serverConfigCompressionKey)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			return NewResourceError("resetting compression configuration", serverConfigCompressionKey, err)
		}
	}

	d.SetId("")
	tflog.Debug(ctx, "Deleted compression configuration")

	return nil
}

// buildServerConfigCompressionCfgData builds the key=value string sent to
// set-config-kv. MinIO merges the sent keys into the stored config, so a key
// that is left out keeps its old value; a key the configuration cleared is
// sent as key= so the server drops it.
func buildServerConfigCompressionCfgData(d *schema.ResourceData, config *S3MinioServerConfigCompression) string {
	parts := make([]string, 0, 5)

	if config.Enable {
		parts = append(parts, "enable=on")
	} else {
		parts = append(parts, "enable=off")
	}

	if config.AllowEncryption {
		parts = append(parts, "allow_encryption=on")
	} else {
		parts = append(parts, "allow_encryption=off")
	}

	appendList := func(key string, values []string) {
		if len(values) > 0 {
			parts = append(parts, buildConfigParam(key, strings.Join(values, ",")))
			return
		}
		// d.HasChange cannot see an explicit empty list at create time: the
		// diff treats no old value and an empty new value as equal, so the
		// raw configuration is checked instead. An attribute that is absent
		// from the configuration (null) must keep the server's value.
		if configuredEmptyList(d.GetRawConfig(), key) {
			parts = append(parts, key+"=")
		}
	}
	appendList("extensions", config.Extensions)
	appendList("mime_types", config.MimeTypes)

	// The resource owns the singleton's comment: an empty configuration
	// clears whatever comment the server holds, including one set outside
	// Terraform.
	if config.Comment != "" {
		parts = append(parts, buildConfigParam("comment", config.Comment))
	} else {
		parts = append(parts, "comment=")
	}

	return strings.Join(parts, " ")
}

// configuredEmptyList reports whether the raw configuration explicitly sets
// a list attribute to an empty list. cty.NilVal is what GetRawConfig returns
// when the ResourceData carries no raw config, such as in unit tests built
// with TestResourceDataRaw.
func configuredEmptyList(raw cty.Value, key string) bool {
	if raw == cty.NilVal || raw.IsNull() || !raw.IsKnown() {
		return false
	}
	ty := raw.Type()
	if !ty.IsObjectType() || !ty.HasAttribute(key) {
		return false
	}
	v := raw.GetAttr(key)
	return v.IsKnown() && !v.IsNull() && v.LengthInt() == 0
}

// compressionConfigList splits the comma-separated value the server stores
// for extensions and mime_types. An empty value means the list was cleared.
func compressionConfigList(value string) []string {
	if value == "" {
		return []string{}
	}
	return strings.Split(value, ",")
}

// validateCompressionListItem applies the config-value rules to one entry of
// extensions or mime_types, where an empty entry and a comma are errors too:
// the joined value is what the server stores, and the server rejects an
// empty extension or mime type and splits on commas.
func validateCompressionListItem(val interface{}, key string) (warns []string, errs []error) {
	v := val.(string)
	switch {
	case v == "":
		errs = append(errs, fmt.Errorf("%q cannot be empty: MinIO rejects an empty extension or mime type", key))
	case strings.Contains(v, ","):
		errs = append(errs, fmt.Errorf("%q cannot contain a comma: MinIO uses the comma as the separator between extensions and mime types", key))
	default:
		warns, errs = validateConfigValue(v, key)
	}
	return
}
