package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func notifyConfigKey(subsystem, name string) string {
	return fmt.Sprintf("%s:%s", subsystem, name)
}

func notifyCreate(nrc notifyResourceConfig) schema.CreateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		admin := meta.(*S3MinioClient).S3Admin
		name := d.Get("name").(string)

		tflog.Debug(ctx, "Creating notification target", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})

		cfgData := nrc.buildCfg(d, meta)
		configString := fmt.Sprintf("%s %s", notifyConfigKey(nrc.subsystem, name), cfgData)
		restart, err := admin.SetConfigKV(ctx, configString)
		if err != nil {
			return NewResourceError(fmt.Sprintf("creating %s target", nrc.subsystem), name, err)
		}

		d.SetId(name)
		_ = d.Set("restart_required", restart)

		tflog.Debug(ctx, "Created notification target", map[string]interface{}{"subsystem": nrc.subsystem, "name": name, "restart_required": restart})

		return notifyRead(nrc)(ctx, d, meta)
	}
}

func notifyRead(nrc notifyResourceConfig) schema.ReadContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		admin := meta.(*S3MinioClient).S3Admin
		name := d.Id()

		tflog.Debug(ctx, "Reading notification target", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})

		configKey := notifyConfigKey(nrc.subsystem, name)
		configData, err := admin.GetConfigKV(ctx, configKey)
		if err != nil {
			return handleNotifyReadError(ctx, err, nrc.subsystem, name, d)
		}

		configStr := strings.TrimSpace(string(configData))
		tflog.Debug(ctx, "Raw notification target config", map[string]interface{}{"subsystem": nrc.subsystem, "name": name, "raw": configStr})

		var valueStr string
		for _, line := range strings.Split(configStr, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, configKey+" ") {
				parts := strings.SplitN(line, " ", 2)
				if len(parts) == 2 {
					valueStr = strings.TrimSpace(parts[1])
				}
				break
			}
		}
		if valueStr == "" && !strings.Contains(configStr, "\n") {
			valueStr = configStr
		}

		cfgMap := parseConfigParams(valueStr)

		_ = d.Set("name", name)

		if len(cfgMap) == 0 {
			// The response carried no keys for this target: env-override
			// comment lines only, or a line the matcher did not find. Nothing
			// can be compared, so keep the state values instead of letting the
			// read store defaults for keys the response never had.
			tflog.Warn(ctx, "Notification target response carried no keys, keeping state values", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})
			return nil
		}

		if d.Get("restart_required").(bool) && nrc.buildCfg != nil {
			if !notifyRunningConfigMatchesState(nrc, cfgMap, d, meta) {
				tflog.Warn(ctx, "Notification target still shows the pre-restart config, keeping configured values in state", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})
				return nil
			}
			if err := d.Set("restart_required", false); err != nil {
				return NewResourceError("clearing restart_required", name, err)
			}
			tflog.Debug(ctx, "Notification target config took effect after restart", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})
		}

		if nrc.readFields != nil {
			if diags := nrc.readFields(cfgMap, d); diags != nil {
				return diags
			}
		}

		return nil
	}
}

// notifyRunningConfigMatchesState reports whether the values the server
// reports for the target agree with what the provider would write for the
// current state.
//
// While a restart is pending, get-config-kv serves the pre-restart config:
// changed values are stale and a removed value is still shown with its old
// value. Only keys the provider itself sends are compared: MinIO seeds the
// subsystem defaults into every target (Config.SetKVS), and a default the
// provider never sent, such as sasl_mechanism=plain, is not the provider's to
// converge on. Keys missing from the response are treated as stale only where
// the stored value is always printed: a cleared comment or queue_dir must come
// back empty, and enable=off must come back as off, because the server prints
// enable only when it is off. Secret keys are absent from the response and are
// never compared. See issue #1299.
func notifyRunningConfigMatchesState(nrc notifyResourceConfig, cfgMap map[string]string, d *schema.ResourceData, meta interface{}) bool {
	intended := parseConfigParams(nrc.buildCfg(d, meta))
	for key, serverValue := range cfgMap {
		want, sent := intended[key]
		if !sent {
			if key == "comment" || key == "queue_dir" {
				if serverValue != "" {
					return false
				}
			}
			continue
		}
		if serverValue != want {
			return false
		}
	}
	if intended["enable"] == "off" {
		if got, ok := cfgMap["enable"]; !ok || got != "off" {
			return false
		}
	}
	return true
}

func notifyUpdate(nrc notifyResourceConfig) schema.UpdateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		admin := meta.(*S3MinioClient).S3Admin
		name := d.Get("name").(string)

		tflog.Debug(ctx, "Updating notification target", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})

		cfgData := nrc.buildCfg(d, meta)
		configString := fmt.Sprintf("%s %s", notifyConfigKey(nrc.subsystem, name), cfgData)
		restart, err := admin.SetConfigKV(ctx, configString)
		if err != nil {
			return NewResourceError(fmt.Sprintf("updating %s target", nrc.subsystem), name, err)
		}

		_ = d.Set("restart_required", restart)

		tflog.Debug(ctx, "Updated notification target", map[string]interface{}{"subsystem": nrc.subsystem, "name": name, "restart_required": restart})

		return notifyRead(nrc)(ctx, d, meta)
	}
}

func notifyDelete(nrc notifyResourceConfig) schema.DeleteContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		admin := meta.(*S3MinioClient).S3Admin
		name := d.Id()

		tflog.Debug(ctx, "Deleting notification target", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})

		configKey := notifyConfigKey(nrc.subsystem, name)
		_, err := admin.DelConfigKV(ctx, configKey)
		if err != nil {
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") ||
				strings.Contains(errMsg, "there is no target") {
				tflog.Warn(ctx, "Notification target already removed", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})
				d.SetId("")
				return nil
			}
			return NewResourceError(fmt.Sprintf("deleting %s target", nrc.subsystem), name, err)
		}

		d.SetId("")
		tflog.Debug(ctx, "Deleted notification target", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})

		return nil
	}
}

func handleNotifyReadError(ctx context.Context, err error, subsystem, name string, d *schema.ResourceData) diag.Diagnostics {
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") {
		tflog.Warn(ctx, "Notification target no longer exists, removing from state", map[string]interface{}{"subsystem": subsystem, "name": name})
		d.SetId("")
		return nil
	}
	if strings.Contains(errMsg, "there is no target") {
		// The same error is returned for a new target waiting on its restart and
		// for a target that no longer exists. restart_required tells the two
		// apart: once it is false the server has applied this target's config
		// at least once, so the absence means it was removed outside Terraform.
		if d.Get("restart_required").(bool) {
			tflog.Warn(ctx, "Notification target not yet active (server restart may be required)", map[string]interface{}{"subsystem": subsystem, "name": name})
			_ = d.Set("name", name)
			return nil
		}
		tflog.Warn(ctx, "Notification target no longer exists, removing from state", map[string]interface{}{"subsystem": subsystem, "name": name})
		d.SetId("")
		return nil
	}
	return NewResourceError(fmt.Sprintf("reading %s target", subsystem), name, err)
}

// notifyBuildCfgAddParam adds a key=value pair to parts, quoting values with spaces.
func notifyBuildCfgAddParam(parts *[]string, key, val string) {
	if val != "" {
		if strings.ContainsAny(val, " \t") {
			*parts = append(*parts, fmt.Sprintf("%s=%q", key, val))
		} else {
			*parts = append(*parts, fmt.Sprintf("%s=%s", key, val))
		}
	}
}

// notifyBuildCfgAddBool adds a bool field as on/off.
func notifyBuildCfgAddBool(parts *[]string, key string, val bool) {
	if val {
		*parts = append(*parts, key+"=on")
	} else {
		*parts = append(*parts, key+"=off")
	}
}

// notifyBuildCfgAddInt adds an int field if > 0.
func notifyBuildCfgAddInt(parts *[]string, key string, val int) {
	if val > 0 {
		*parts = append(*parts, fmt.Sprintf("%s=%d", key, val))
	}
}

func notifyCommonSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"name": {
			Type:        schema.TypeString,
			Required:    true,
			ForceNew:    true,
			Description: "Target name identifier.",
		},
		"enable": {
			Type:        schema.TypeBool,
			Optional:    true,
			Default:     true,
			Description: "Whether this notification target is enabled.",
		},
		"queue_dir": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Directory path for persistent event store when the target is offline.",
		},
		"queue_limit": {
			Type:        schema.TypeInt,
			Optional:    true,
			Computed:    true,
			Description: "Maximum number of undelivered messages to queue.",
		},
		"comment": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Comment or description for this notification target.",
		},
		"restart_required": {
			Type:        schema.TypeBool,
			Computed:    true,
			Description: "Indicates whether a MinIO server restart is required.",
		},
	}
}

func mergeSchemas(common, specific map[string]*schema.Schema) map[string]*schema.Schema {
	result := make(map[string]*schema.Schema, len(common)+len(specific))
	for k, v := range common {
		result[k] = v
	}
	for k, v := range specific {
		result[k] = v
	}
	return result
}

func notifyReadCommonFields(cfgMap map[string]string, d *schema.ResourceData) {
	if v, ok := cfgMap["enable"]; ok {
		_ = d.Set("enable", v == "on")
	} else {
		// The server prints enable only when it is off (SubsysInfo.WriteTo),
		// so a response without it means the target is on.
		_ = d.Set("enable", true)
	}
	if v, ok := cfgMap["queue_limit"]; ok {
		if n, err := parseInt(v); err == nil {
			_ = d.Set("queue_limit", n)
		}
	}
	if v, ok := cfgMap["queue_dir"]; ok {
		_ = d.Set("queue_dir", v)
	}
	if v, ok := cfgMap["comment"]; ok {
		_ = d.Set("comment", v)
	}
}

// notifyBuildCfgClearableParam adds key=value like notifyBuildCfgAddParam, and
// sends an empty key= when the value was removed from the configuration.
// MinIO merges set-config-kv into the target's current settings, so a key
// that is left out keeps its old value.
func notifyBuildCfgClearableParam(parts *[]string, d *schema.ResourceData, key string) {
	val := getOptionalField(d, key, "").(string)
	if val == "" && d.HasChange(key) {
		*parts = append(*parts, key+"=")
		return
	}
	notifyBuildCfgAddParam(parts, key, val)
}

func notifyBuildCommonCfg(parts *[]string, d *schema.ResourceData, meta interface{}) {
	notifyBuildCfgClearableParam(parts, d, "queue_dir")
	notifyBuildCfgClearableParam(parts, d, "comment")
	notifyBuildCfgAddInt(parts, "queue_limit", getOptionalField(d, "queue_limit", 0).(int))
	// d.Get, not getOptionalField: GetOk reads false as unset and would send
	// enable=on for a target the configuration disables.
	notifyBuildCfgAddBool(parts, "enable", d.Get("enable").(bool))
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
