package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/minio/madmin-go/v3"
)

// notifyFrameworkConfig holds the type-specific configuration for a notification resource in framework.
type notifyFrameworkConfig struct {
	Subsystem  string
	BuildCfg   func(*notifyFrameworkResourceData) string
	ReadFields func(map[string]string, *notifyFrameworkResourceData) diag.Diagnostics
}

// notifyFrameworkResourceData represents resource data for framework notify resources.
type notifyFrameworkResourceData struct {
	Name            types.String
	Enable          types.Bool
	QueueDir        types.String
	QueueLimit      types.Int64
	Comment         types.String
	RestartRequired types.Bool
	ExtraFields     map[string]interface{}
}

func (d *notifyFrameworkResourceData) SetStringField(key string, value types.String) {
	if d.ExtraFields == nil {
		d.ExtraFields = make(map[string]interface{})
	}
	d.ExtraFields[key] = value
}

func (d *notifyFrameworkResourceData) GetStringField(key string) types.String {
	if d.ExtraFields == nil {
		return types.StringNull()
	}
	if v, ok := d.ExtraFields[key]; ok {
		if str, ok := v.(types.String); ok {
			return str
		}
	}
	return types.StringNull()
}

func (d *notifyFrameworkResourceData) SetBoolField(key string, value types.Bool) {
	if d.ExtraFields == nil {
		d.ExtraFields = make(map[string]interface{})
	}
	d.ExtraFields[key] = value
}

func (d *notifyFrameworkResourceData) GetBoolField(key string) types.Bool {
	if d.ExtraFields == nil {
		return types.BoolNull()
	}
	if v, ok := d.ExtraFields[key]; ok {
		if b, ok := v.(types.Bool); ok {
			return b
		}
	}
	return types.BoolNull()
}

func (d *notifyFrameworkResourceData) SetInt64Field(key string, value types.Int64) {
	if d.ExtraFields == nil {
		d.ExtraFields = make(map[string]interface{})
	}
	d.ExtraFields[key] = value
}

func (d *notifyFrameworkResourceData) GetInt64Field(key string) types.Int64 {
	if d.ExtraFields == nil {
		return types.Int64Null()
	}
	if v, ok := d.ExtraFields[key]; ok {
		if i, ok := v.(types.Int64); ok {
			return i
		}
	}
	return types.Int64Null()
}

func notifyFrameworkConfigKey(subsystem, name string) string {
	return fmt.Sprintf("%s:%s", subsystem, name)
}

func notifyFrameworkCreate(ctx context.Context, client *madmin.AdminClient, config notifyFrameworkConfig, plan *notifyFrameworkResourceData) diag.Diagnostics {
	var diags diag.Diagnostics

	name := plan.Name.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Creating %s: %s", config.Subsystem, name))

	cfgData := config.BuildCfg(plan)
	configString := fmt.Sprintf("%s %s", notifyFrameworkConfigKey(config.Subsystem, name), cfgData)
	restart, err := client.SetConfigKV(ctx, configString)
	if err != nil {
		diags.AddError(fmt.Sprintf("creating %s target", config.Subsystem), fmt.Sprintf("Failed to create %s: %s", config.Subsystem, err))
		return diags
	}

	plan.RestartRequired = types.BoolValue(restart)

	tflog.Info(ctx, fmt.Sprintf("Created %s: %s (restart_required=%v)", config.Subsystem, name, restart))

	return diags
}

func notifyFrameworkRead(ctx context.Context, client *madmin.AdminClient, config notifyFrameworkConfig, state *notifyFrameworkResourceData) diag.Diagnostics {
	var diags diag.Diagnostics

	name := state.Name.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Reading %s: %s", config.Subsystem, name))

	configKey := notifyFrameworkConfigKey(config.Subsystem, name)
	configData, err := client.GetConfigKV(ctx, configKey)
	if err != nil {
		diags.Append(handleNotifyFrameworkReadError(ctx, err, config.Subsystem, name, state)...)
		return diags
	}

	configStr := strings.TrimSpace(string(configData))
	tflog.Debug(ctx, fmt.Sprintf("Raw config data for %s %s: %s", config.Subsystem, name, configStr))

	var valueStr string
	if strings.HasPrefix(configStr, configKey+" ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	} else {
		valueStr = configStr
	}

	cfgMap := parseConfigParams(valueStr)

	state.Name = types.StringValue(name)

	if config.ReadFields != nil {
		if readDiags := config.ReadFields(cfgMap, state); readDiags.HasError() {
			diags.Append(readDiags...)
			return diags
		}
	}

	return diags
}

func notifyFrameworkUpdate(ctx context.Context, client *madmin.AdminClient, config notifyFrameworkConfig, plan *notifyFrameworkResourceData) diag.Diagnostics {
	var diags diag.Diagnostics

	name := plan.Name.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Updating %s: %s", config.Subsystem, name))

	cfgData := config.BuildCfg(plan)
	configString := fmt.Sprintf("%s %s", notifyFrameworkConfigKey(config.Subsystem, name), cfgData)
	restart, err := client.SetConfigKV(ctx, configString)
	if err != nil {
		diags.AddError(fmt.Sprintf("updating %s target", config.Subsystem), fmt.Sprintf("Failed to update %s: %s", config.Subsystem, err))
		return diags
	}

	plan.RestartRequired = types.BoolValue(restart)

	tflog.Info(ctx, fmt.Sprintf("Updated %s: %s (restart_required=%v)", config.Subsystem, name, restart))

	return diags
}

func notifyFrameworkDelete(ctx context.Context, client *madmin.AdminClient, subsystem, name string, state *notifyFrameworkResourceData) diag.Diagnostics {
	var diags diag.Diagnostics

	tflog.Info(ctx, fmt.Sprintf("Deleting %s: %s", subsystem, name))

	configKey := notifyFrameworkConfigKey(subsystem, name)
	_, err := client.DelConfigKV(ctx, configKey)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") ||
			strings.Contains(errMsg, "there is no target") {
			tflog.Warn(ctx, fmt.Sprintf("%s %s already removed", subsystem, name))
			state.Name = types.StringNull()
			return diags
		}
		diags.AddError(fmt.Sprintf("deleting %s target", subsystem), fmt.Sprintf("Failed to delete %s: %s", subsystem, err))
		return diags
	}

	state.Name = types.StringNull()
	tflog.Info(ctx, fmt.Sprintf("Deleted %s: %s", subsystem, name))

	return diags
}

func handleNotifyFrameworkReadError(ctx context.Context, err error, subsystem, name string, state *notifyFrameworkResourceData) diag.Diagnostics {
	var diags diag.Diagnostics

	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") {
		tflog.Warn(ctx, fmt.Sprintf("%s %s no longer exists, removing from state", subsystem, name))
		state.Name = types.StringNull()
		return diags
	}
	if strings.Contains(errMsg, "there is no target") {
		tflog.Warn(ctx, fmt.Sprintf("%s %s not yet active (server restart may be required)", subsystem, name))
		return diags
	}
	diags.AddError(fmt.Sprintf("reading %s target", subsystem), fmt.Sprintf("Failed to read %s: %s", subsystem, err))
	return diags
}

// notifyFrameworkBuildCfgAddParam adds a key=value pair to parts, quoting values with spaces.
func notifyFrameworkBuildCfgAddParam(parts *[]string, key string, val string) {
	if val != "" {
		if strings.ContainsAny(val, " \t") {
			*parts = append(*parts, fmt.Sprintf("%s=%q", key, val))
		} else {
			*parts = append(*parts, fmt.Sprintf("%s=%s", key, val))
		}
	}
}

// notifyFrameworkBuildCfgAddBool adds a bool field as on/off.
func notifyFrameworkBuildCfgAddBool(parts *[]string, key string, val bool) {
	if val {
		*parts = append(*parts, key+"=on")
	} else {
		*parts = append(*parts, key+"=off")
	}
}

// notifyFrameworkBuildCfgAddInt adds an int field if > 0.
func notifyFrameworkBuildCfgAddInt(parts *[]string, key string, val int64) {
	if val > 0 {
		*parts = append(*parts, fmt.Sprintf("%s=%d", key, val))
	}
}

// notifyFrameworkReadCommonFields reads queue_dir, queue_limit, and comment from config.
func notifyFrameworkReadCommonFields(cfgMap map[string]string, state *notifyFrameworkResourceData) {
	if v, ok := cfgMap["queue_limit"]; ok {
		if n, err := parseInt(v); err == nil {
			state.QueueLimit = types.Int64Value(int64(n))
		}
	}
	if v, ok := cfgMap["queue_dir"]; ok && v != "" {
		state.QueueDir = types.StringValue(v)
	}
	if v, ok := cfgMap["comment"]; ok && v != "" {
		state.Comment = types.StringValue(v)
	}
}

// notifyFrameworkBuildCommonCfg appends common fields (queue_dir, queue_limit, comment, enable).
func notifyFrameworkBuildCommonCfg(parts *[]string, data *notifyFrameworkResourceData) {
	if !data.QueueDir.IsNull() && !data.QueueDir.IsUnknown() {
		notifyFrameworkBuildCfgAddParam(parts, "queue_dir", data.QueueDir.ValueString())
	}
	if !data.Comment.IsNull() && !data.Comment.IsUnknown() {
		notifyFrameworkBuildCfgAddParam(parts, "comment", data.Comment.ValueString())
	}
	if !data.QueueLimit.IsNull() && !data.QueueLimit.IsUnknown() && data.QueueLimit.ValueInt64() > 0 {
		notifyFrameworkBuildCfgAddInt(parts, "queue_limit", data.QueueLimit.ValueInt64())
	}
	if !data.Enable.IsNull() && !data.Enable.IsUnknown() {
		notifyFrameworkBuildCfgAddBool(parts, "enable", data.Enable.ValueBool())
	}
}

// notifyFrameworkBuildExtraCfg appends extra fields from ExtraFields map.
func notifyFrameworkBuildExtraCfg(parts *[]string, data *notifyFrameworkResourceData) {
	for key, val := range data.ExtraFields {
		switch v := val.(type) {
		case types.String:
			if !v.IsNull() && !v.IsUnknown() {
				notifyFrameworkBuildCfgAddParam(parts, key, v.ValueString())
			}
		case types.Bool:
			if !v.IsNull() && !v.IsUnknown() {
				notifyFrameworkBuildCfgAddBool(parts, key, v.ValueBool())
			}
		case types.Int64:
			if !v.IsNull() && !v.IsUnknown() && v.ValueInt64() > 0 {
				notifyFrameworkBuildCfgAddInt(parts, key, v.ValueInt64())
			}
		}
	}
}
