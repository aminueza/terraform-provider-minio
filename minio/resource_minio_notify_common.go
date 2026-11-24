package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func notifyConfigKey(subsystem, name string) string {
	return fmt.Sprintf("%s:%s", subsystem, name)
}

var notifyConfigCreateLock = NewMutexKV()

func notifyCreate(nrc notifyResourceConfig) schema.CreateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		admin := meta.(*S3MinioClient).S3Admin
		name := d.Get("name").(string)
		configKey := notifyConfigKey(nrc.subsystem, name)

		notifyConfigCreateLock.Lock(configKey)
		defer notifyConfigCreateLock.Unlock(configKey)

		// SetConfigKV merges into an existing target, overwriting its keys and
		// keeping untouched ones, so reject when the target already exists.
		_, err := admin.GetConfigKV(ctx, configKey)
		if err == nil {
			return NewResourceError(
				fmt.Sprintf("creating %s target", nrc.subsystem),
				name,
				alreadyExistsError("minio_"+nrc.subsystem, name, name),
			)
		}
		if !isNotifyTargetMissingError(err) {
			return NewResourceError(fmt.Sprintf("checking for existing %s target", nrc.subsystem), name, err)
		}

		tflog.Debug(ctx, "Creating notification target", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})

		cfgData := nrc.buildCfg(d, meta)
		configString := fmt.Sprintf("%s %s", configKey, cfgData)
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
			return handleNotifyReadError(ctx, nrc, err, name, d)
		}

		configStr := strings.TrimSpace(string(configData))
		tflog.Debug(ctx, "Raw notification target config", map[string]interface{}{"subsystem": nrc.subsystem, "name": name, "raw": configStr})

		cfgMap, _ := notifyTargetConfigParams(configStr, configKey)

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

// notifyTargetConfigParams extracts the target's own key=value pairs from a
// get-config-kv response. Any single-line response counts as the target's
// own value, as the read always did; among several lines the target's line
// is matched by its "key " prefix.
func notifyTargetConfigParams(response, configKey string) (cfgMap map[string]string, ok bool) {
	configStr := strings.TrimSpace(response)
	for _, line := range strings.Split(configStr, "\n") {
		line = strings.TrimSpace(line)
		if value, found := strings.CutPrefix(line, configKey+" "); found {
			return parseConfigParams(strings.TrimSpace(value)), true
		}
	}
	if !strings.Contains(configStr, "\n") {
		return parseConfigParams(configStr), true
	}
	return map[string]string{}, false
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
// the stored value is always printed: a cleared key listed by
// isClearableNotifyConfigKey must come back empty, and enable=off must come
// back as off, because the server prints enable only when it is off. Secret
// keys are absent from the response and are never compared. See issue #1299.
func notifyRunningConfigMatchesState(nrc notifyResourceConfig, cfgMap map[string]string, d *schema.ResourceData, meta interface{}) bool {
	intended := parseConfigParams(nrc.buildCfg(d, meta))
	for key, serverValue := range cfgMap {
		want, sent := intended[key]
		if !sent {
			if isClearableNotifyConfigKey(key) {
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

func handleNotifyReadError(ctx context.Context, nrc notifyResourceConfig, err error, name string, d *schema.ResourceData) diag.Diagnostics {
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") {
		tflog.Warn(ctx, "Notification target no longer exists, removing from state", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})
		d.SetId("")
		return nil
	}
	if strings.Contains(errMsg, "there is no target") {
		// The same error is returned for a new target waiting on its restart and
		// for a target that no longer exists. restart_required tells the two
		// apart: once it is false the server has applied this target's config
		// at least once, so the absence means it was removed outside Terraform.
		// Data sources carry no such state (keepStateOnMissingTarget), so for
		// them the absence only ever means the restart is still pending.
		if nrc.keepStateOnMissingTarget || d.Get("restart_required").(bool) {
			tflog.Warn(ctx, "Notification target not yet active (server restart may be required)", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})
			_ = d.Set("name", name)
			return nil
		}
		tflog.Warn(ctx, "Notification target no longer exists, removing from state", map[string]interface{}{"subsystem": nrc.subsystem, "name": name})
		d.SetId("")
		return nil
	}
	return NewResourceError(fmt.Sprintf("reading %s target", nrc.subsystem), name, err)
}

// notifyBuildCfgAddParam adds a key=value pair to parts, quoting values with
// whitespace the way buildConfigParam does (no escaping, issue #1300).
func notifyBuildCfgAddParam(parts *[]string, key, val string) {
	if val != "" {
		*parts = append(*parts, buildConfigParam(key, val))
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
	// Every string field of a notification target is written as a MinIO
	// config KV value, so each must be representable; this covers all
	// notify-family resources, including the common comment and queue_dir.
	for k, v := range result {
		if v.Type != schema.TypeString || k == "name" || v.ValidateDiagFunc != nil {
			continue
		}
		if v.ValidateFunc != nil {
			v.ValidateFunc = validation.All(v.ValidateFunc, validateConfigValue)
		} else {
			v.ValidateFunc = validateConfigValue
		}
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

// notifyBuildCfgClearablePair adds readableKey and secretKey via
// notifyBuildCfgClearableParam, and also clears secretKey when readableKey was
// removed while secretKey is not in state (for example after import, where a
// leftover secret would fail target pair validation or linger stale on MinIO).
func notifyBuildCfgClearablePair(parts *[]string, d *schema.ResourceData, readableKey, secretKey string) {
	notifyBuildCfgClearableParam(parts, d, readableKey)
	secretVal := getOptionalField(d, secretKey, "").(string)
	if secretVal == "" && (d.HasChange(secretKey) || (d.HasChange(readableKey) && getOptionalField(d, readableKey, "").(string) == "")) {
		*parts = append(*parts, secretKey+"=")
		return
	}
	notifyBuildCfgAddParam(parts, secretKey, secretVal)
}

// isClearableNotifyConfigKey lists the non-secret keys the builders can clear
// with notifyBuildCfgClearableParam. Every such key must be listed here too,
// otherwise a cleared value that has not reached the server yet (pending
// restart) is not recognized as stale by notifyRunningConfigMatchesState.
func isClearableNotifyConfigKey(key string) bool {
	switch key {
	case "comment", "queue_dir",
		"client_cert", "proxy", "username", "streaming_cluster_id",
		"cert_authority", "sasl_username", "client_tls_cert", "version",
		"user", "exchange", "exchange_type", "routing_key":
		return true
	default:
		return false
	}
}

// notifyWithQueueSize replaces queue_limit for logger_webhook and
// audit_kafka. Those subsystems size their queue with queue_size and have no
// queue_limit key, and MinIO glues an unknown key onto the value written
// before it, so a queue_limit sent there corrupted the comment, queue_dir or
// topic.
func notifyWithQueueSize(specific map[string]*schema.Schema) map[string]*schema.Schema {
	for k, v := range map[string]*schema.Schema{
		"queue_size": {
			Type:          schema.TypeInt,
			Optional:      true,
			Computed:      true,
			ConflictsWith: []string{"queue_limit"},
			Description:   "Maximum number of undelivered events to queue.",
		},
		"queue_limit": {
			Type:          schema.TypeInt,
			Optional:      true,
			Computed:      true,
			ConflictsWith: []string{"queue_size"},
			Deprecated:    "Use 'queue_size' instead. This target has no queue_limit setting; the value is sent as queue_size. This attribute will be removed in a future major version.",
			Description:   "Deprecated: use 'queue_size' instead.",
		},
	} {
		specific[k] = v
	}
	return specific
}

// notifyBuildQueueSizeCommonCfg is notifyBuildCommonCfg for the subsystems
// that use queue_size. A queue_limit set without queue_size in the
// configuration is sent as queue_size.
func notifyBuildQueueSizeCommonCfg(parts *[]string, d *schema.ResourceData) {
	notifyBuildCfgClearableParam(parts, d, "queue_dir")
	notifyBuildCfgClearableParam(parts, d, "comment")
	notifyBuildCfgAddInt(parts, "queue_size", notifyQueueSize(d))
	// d.Get, not getOptionalField: GetOk reads false as unset and would send
	// enable=on for a target the configuration disables.
	notifyBuildCfgAddBool(parts, "enable", d.Get("enable").(bool))
}

// notifyReadQueueSize stores the server's queue_size in queue_size and in
// the deprecated queue_limit. Mirroring it into queue_limit makes a target an
// older provider corrupted show up as a diff: that provider glued queue_limit
// onto the value written before it, so the server kept the default
// queue_size, and only a diff on queue_limit triggers the update that writes
// every key again.
func notifyReadQueueSize(cfgMap map[string]string, d *schema.ResourceData) {
	v, ok := cfgMap["queue_size"]
	if !ok {
		return
	}
	if n, err := parseInt(v); err == nil {
		_ = d.Set("queue_size", n)
		_ = d.Set("queue_limit", n)
	}
}

func notifyQueueSize(d *schema.ResourceData) int {
	if raw := d.GetRawConfig(); !raw.IsNull() && raw.GetAttr("queue_size").IsNull() {
		if limit, ok := d.GetOk("queue_limit"); ok {
			return limit.(int)
		}
	}
	return d.Get("queue_size").(int)
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

func customizeDiffKafkaSasl(_ context.Context, d *schema.ResourceDiff, _ interface{}) error {
	if !d.NewValueKnown("sasl") || d.Get("sasl").(bool) {
		return nil
	}
	for _, k := range []string{"sasl_username", "sasl_password"} {
		// A credential unknown at plan time resolves at apply, when
		// CustomizeDiff does not run again, so treat it as set.
		if !d.NewValueKnown(k) || d.Get(k).(string) != "" {
			return fmt.Errorf("sasl_username and sasl_password are ignored by MinIO when sasl is false; add sasl = true to enable SASL authentication")
		}
	}
	return nil
}
