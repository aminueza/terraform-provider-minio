package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// notifyResourceConfig holds the type-specific configuration for a notification resource.
type notifyResourceConfig struct {
	subsystem   string // e.g., "notify_amqp", "notify_kafka"
	buildCfg    func(*schema.ResourceData, interface{}) string
	readFields  func(map[string]string, *schema.ResourceData) diag.Diagnostics
}

func notifyConfigKey(subsystem, name string) string {
	return fmt.Sprintf("%s:%s", subsystem, name)
}

func notifyCreate(nrc notifyResourceConfig) schema.CreateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		admin := meta.(*S3MinioClient).S3Admin
		name := d.Get("name").(string)

		log.Printf("[DEBUG] Creating %s: %s", nrc.subsystem, name)

		cfgData := nrc.buildCfg(d, meta)
		configString := fmt.Sprintf("%s %s", notifyConfigKey(nrc.subsystem, name), cfgData)
		restart, err := admin.SetConfigKV(ctx, configString)
		if err != nil {
			return NewResourceError(fmt.Sprintf("creating %s target", nrc.subsystem), name, err)
		}

		d.SetId(name)
		_ = d.Set("restart_required", restart)

		log.Printf("[DEBUG] Created %s: %s (restart_required=%v)", nrc.subsystem, name, restart)

		return notifyRead(nrc)(ctx, d, meta)
	}
}

func notifyRead(nrc notifyResourceConfig) schema.ReadContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		admin := meta.(*S3MinioClient).S3Admin
		name := d.Id()

		log.Printf("[DEBUG] Reading %s: %s", nrc.subsystem, name)

		configKey := notifyConfigKey(nrc.subsystem, name)
		configData, err := admin.GetConfigKV(ctx, configKey)
		if err != nil {
			return handleNotifyReadError(err, nrc.subsystem, name, d)
		}

		configStr := strings.TrimSpace(string(configData))
		log.Printf("[DEBUG] Raw config data for %s %s: %s", nrc.subsystem, name, configStr)

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

		if nrc.readFields != nil {
			if diags := nrc.readFields(cfgMap, d); diags != nil {
				return diags
			}
		}

		return nil
	}
}

func notifyUpdate(nrc notifyResourceConfig) schema.UpdateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		admin := meta.(*S3MinioClient).S3Admin
		name := d.Get("name").(string)

		log.Printf("[DEBUG] Updating %s: %s", nrc.subsystem, name)

		cfgData := nrc.buildCfg(d, meta)
		configString := fmt.Sprintf("%s %s", notifyConfigKey(nrc.subsystem, name), cfgData)
		restart, err := admin.SetConfigKV(ctx, configString)
		if err != nil {
			return NewResourceError(fmt.Sprintf("updating %s target", nrc.subsystem), name, err)
		}

		_ = d.Set("restart_required", restart)

		log.Printf("[DEBUG] Updated %s: %s (restart_required=%v)", nrc.subsystem, name, restart)

		return notifyRead(nrc)(ctx, d, meta)
	}
}

func notifyDelete(nrc notifyResourceConfig) schema.DeleteContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		admin := meta.(*S3MinioClient).S3Admin
		name := d.Id()

		log.Printf("[DEBUG] Deleting %s: %s", nrc.subsystem, name)

		configKey := notifyConfigKey(nrc.subsystem, name)
		_, err := admin.DelConfigKV(ctx, configKey)
		if err != nil {
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") ||
				strings.Contains(errMsg, "there is no target") {
				log.Printf("[WARN] %s %s already removed", nrc.subsystem, name)
				d.SetId("")
				return nil
			}
			return NewResourceError(fmt.Sprintf("deleting %s target", nrc.subsystem), name, err)
		}

		d.SetId("")
		log.Printf("[DEBUG] Deleted %s: %s", nrc.subsystem, name)

		return nil
	}
}

func handleNotifyReadError(err error, subsystem, name string, d *schema.ResourceData) diag.Diagnostics {
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") {
		log.Printf("[WARN] %s %s no longer exists, removing from state", subsystem, name)
		d.SetId("")
		return nil
	}
	if strings.Contains(errMsg, "there is no target") {
		log.Printf("[WARN] %s %s not yet active (server restart may be required)", subsystem, name)
		_ = d.Set("name", name)
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

// notifyCommonSchema returns schema fields shared by all notification targets.
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

// mergeSchemas merges common and type-specific schemas.
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

// notifyReadCommonFields reads queue_dir, queue_limit, and comment from config.
func notifyReadCommonFields(cfgMap map[string]string, d *schema.ResourceData) {
	if v, ok := cfgMap["queue_limit"]; ok {
		if n, err := parseInt(v); err == nil {
			_ = d.Set("queue_limit", n)
		}
	}
	if v, ok := cfgMap["queue_dir"]; ok && v != "" {
		_ = d.Set("queue_dir", v)
	}
	if v, ok := cfgMap["comment"]; ok && v != "" {
		_ = d.Set("comment", v)
	}
}

// notifyBuildCommonCfg appends common fields (queue_dir, queue_limit, comment, enable).
func notifyBuildCommonCfg(parts *[]string, d *schema.ResourceData, meta interface{}) {
	notifyBuildCfgAddParam(parts, "queue_dir", getOptionalField(d, "queue_dir", "").(string))
	notifyBuildCfgAddParam(parts, "comment", getOptionalField(d, "comment", "").(string))
	notifyBuildCfgAddInt(parts, "queue_limit", getOptionalField(d, "queue_limit", 0).(int))
	notifyBuildCfgAddBool(parts, "enable", getOptionalField(d, "enable", true).(bool))
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
