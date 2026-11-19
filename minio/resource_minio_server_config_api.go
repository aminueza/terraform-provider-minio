package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceMinioServerConfigApi() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages MinIO API server configuration including request throttling, CORS, transition workers, stale upload cleanup, delete cleanup, list quorum, object version limits, and replication tuning. Removing an attribute from the configuration leaves the server's last value; only destroying the resource resets the subsystem to MinIO's defaults.",
		CreateContext: minioServerConfigApiSet,
		ReadContext:   minioServerConfigApiRead,
		UpdateContext: minioServerConfigApiSet,
		DeleteContext: minioServerConfigApiDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"requests_max": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Maximum concurrent API requests. Use 0 or empty for auto.",
			},
			"cors_allow_origin": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Comma-separated list of allowed CORS origins (e.g., \"https://app.example.com\").",
			},
			"transition_workers": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Number of ILM transition workers.",
			},
			"stale_uploads_expiry": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Duration after which incomplete multipart uploads are cleaned up (e.g., \"24h\").",
			},
			"stale_uploads_cleanup_interval": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Interval between stale upload cleanup runs (e.g., \"6h\").",
			},
			"cluster_deadline": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Deadline for cluster read operations (e.g., \"10s\").",
			},
			"remote_transport_deadline": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Deadline for remote transport operations (e.g., \"2h\").",
			},
			"root_access": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Whether root user (access/secret key) access is enabled for S3 API.",
			},
			"sync_events": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Enable synchronous bucket notification events.",
			},
			"delete_cleanup_interval": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				ValidateDiagFunc: validateServerConfigAPIDeleteCleanupInterval,
				Description:      "Interval between delete cleanup runs (e.g., \"5m\"). Omit to leave the server value unchanged.",
			},
			"list_quorum": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ValidateFunc: validation.StringInSlice([]string{
					"strict", "reduced", "disk", "optimal", "auto",
				}, false),
				Description: "Quorum behavior for list operations: strict, reduced, disk, optimal or auto.",
			},
			"object_max_versions": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				ValidateDiagFunc: validateServerConfigAPIObjectMaxVersions,
				Description:      "Maximum object versions per object. Omit to leave the server value unchanged.",
			},
			"odirect": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Whether reads and writes use direct I/O (O_DIRECT). The provider writes both `odirect` and `disable_odirect`, since MinIO keeps O_DIRECT on unless `disable_odirect` is `on`.",
			},
			"gzip_objects": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Whether GetObject responses may be gzip-compressed for clients that accept it.",
			},
			"replication_max_workers": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				ValidateDiagFunc: validateServerConfigAPIReplicationMaxWorkers,
				Description:      "Maximum number of concurrent replication workers. Must be between 1 and 500.",
			},
			"replication_max_lrg_workers": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				ValidateDiagFunc: validateServerConfigAPIReplicationMaxLrgWorkers,
				Description:      "Maximum number of replication workers per node for objects of 128 MiB or more. Must be between 1 and 10.",
			},
			"replication_priority": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ValidateFunc: validation.StringInSlice([]string{
					"slow", "fast", "auto",
				}, false),
				Description: "Replication priority: slow, fast or auto.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether a MinIO server restart is required.",
			},
		},
	}
}

func minioServerConfigApiSet(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	var parts []string
	setStr := func(key, attr string) {
		if v, ok := d.GetOk(attr); ok {
			parts = append(parts, fmt.Sprintf("%s=%s", key, v.(string)))
		}
	}
	setBool := func(key, attr string) {
		if d.HasChange(attr) || d.Get(attr) != nil {
			if v, ok := d.GetOk(attr); ok || d.HasChange(attr) {
				if v.(bool) {
					parts = append(parts, key+"=on")
				} else {
					parts = append(parts, key+"=off")
				}
			}
		}
	}

	raw := d.GetRawConfig()
	// rawConfigString reports the configuration's own value for attr, so the
	// write is driven by what the user wrote, not by HasChange (false on
	// create) or by a value the read computed. An unset attribute (null) or an
	// empty one returns set=false, so the server keeps its own value and the
	// Optional+Computed attribute plans no change.
	rawConfigString := func(attr string) (val string, set bool) {
		if raw.IsNull() || !raw.IsKnown() {
			return "", false
		}
		v := raw.GetAttr(attr)
		if v.IsNull() || !v.IsKnown() || v.Type() != cty.String {
			return "", false
		}
		s := v.AsString()
		if s == "" {
			return "", false
		}
		return s, true
	}
	setConfiguredStr := func(key, attr string) {
		if val, set := rawConfigString(attr); set {
			parts = append(parts, fmt.Sprintf("%s=%s", key, val))
		}
	}
	rawConfigBool := func(attr string) (val bool, set bool) {
		if raw.IsNull() || !raw.IsKnown() {
			return false, false
		}
		v := raw.GetAttr(attr)
		if v.IsNull() || !v.IsKnown() || v.Type() != cty.Bool {
			return false, false
		}
		return v.True(), true
	}
	// MinIO computes O_DIRECT as `odirect=on || disable_odirect!=on`, and
	// disable_odirect defaults to empty, so odirect=off alone leaves O_DIRECT
	// on. Write both keys so the server honors the configured value. odirect
	// must come first: MinIO's kvFields finds each key with the first match of
	// "key=", so a leading disable_odirect= would be split at its "odirect=".
	if on, set := rawConfigBool("odirect"); set {
		if on {
			parts = append(parts, "odirect=on", "disable_odirect=off")
		} else {
			parts = append(parts, "odirect=off", "disable_odirect=on")
		}
	}
	if on, set := rawConfigBool("gzip_objects"); set {
		if on {
			parts = append(parts, "gzip_objects=on")
		} else {
			parts = append(parts, "gzip_objects=off")
		}
	}

	setStr("requests_max", "requests_max")
	setStr("cors_allow_origin", "cors_allow_origin")
	setStr("transition_workers", "transition_workers")
	setStr("stale_uploads_expiry", "stale_uploads_expiry")
	setStr("stale_uploads_cleanup_interval", "stale_uploads_cleanup_interval")
	setStr("cluster_deadline", "cluster_deadline")
	setStr("remote_transport_deadline", "remote_transport_deadline")
	setBool("root_access", "root_access")
	setBool("sync_events", "sync_events")
	setConfiguredStr("delete_cleanup_interval", "delete_cleanup_interval")
	setConfiguredStr("list_quorum", "list_quorum")
	setConfiguredStr("object_max_versions", "object_max_versions")
	setConfiguredStr("replication_max_workers", "replication_max_workers")
	setConfiguredStr("replication_max_lrg_workers", "replication_max_lrg_workers")
	setConfiguredStr("replication_priority", "replication_priority")

	if len(parts) == 0 {
		d.SetId("api")
		return minioServerConfigApiRead(ctx, d, meta)
	}

	configString := "api " + strings.Join(parts, " ")
	restart, err := admin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("setting api configuration", "api", err)
	}

	d.SetId("api")
	_ = d.Set("restart_required", restart)
	tflog.Debug(ctx, fmt.Sprintf("Set api config (restart_required=%v)", restart))

	return minioServerConfigApiRead(ctx, d, meta)
}

func minioServerConfigApiRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	configData, err := admin.GetConfigKV(ctx, "api")
	if err != nil {
		return NewResourceError("reading api configuration", "api", err)
	}

	configStr := strings.TrimSpace(string(configData))
	var valueStr string
	if strings.HasPrefix(configStr, "api ") {
		parts := strings.SplitN(configStr, " ", 2)
		if len(parts) == 2 {
			valueStr = strings.TrimSpace(parts[1])
		}
	}

	cfgMap := parseConfigParams(valueStr)

	d.SetId("api")

	strFields := []string{
		"requests_max", "cors_allow_origin", "transition_workers",
		"stale_uploads_expiry", "stale_uploads_cleanup_interval",
		"cluster_deadline", "remote_transport_deadline",
		"delete_cleanup_interval", "list_quorum", "object_max_versions",
		"replication_max_workers", "replication_max_lrg_workers",
		"replication_priority",
	}
	for _, f := range strFields {
		if v, ok := cfgMap[f]; ok {
			if err := d.Set(f, v); err != nil {
				return NewResourceError("setting "+f, d.Id(), err)
			}
		}
	}

	boolFields := []string{"root_access", "sync_events", "gzip_objects"}
	for _, f := range boolFields {
		if v, ok := cfgMap[f]; ok {
			if err := d.Set(f, v == "on"); err != nil {
				return NewResourceError("setting "+f, d.Id(), err)
			}
		}
	}

	if _, ok := cfgMap["odirect"]; ok {
		if err := d.Set("odirect", serverConfigAPIODirectEnabled(cfgMap)); err != nil {
			return NewResourceError("setting odirect", d.Id(), err)
		}
	}

	return nil
}

// serverConfigAPIODirectEnabled mirrors MinIO's own rule in
// internal/config/api/api.go (`enableODirect || !disableODirect`), so the
// state reports whether O_DIRECT is in effect rather than the raw odirect key.
func serverConfigAPIODirectEnabled(cfgMap map[string]string) bool {
	return cfgMap["odirect"] == "on" || cfgMap["disable_odirect"] != "on"
}

func validateServerConfigAPIDeleteCleanupInterval(v interface{}, p cty.Path) diag.Diagnostics {
	value := v.(string)
	if value == "" {
		return diag.Errorf("delete_cleanup_interval must be a non-empty duration (e.g., \"5m\")")
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return diag.Errorf("delete_cleanup_interval must be a valid duration (e.g., \"5m\"), got: %s", value)
	}
	// MinIO stores 0s and negative intervals without complaint.
	if d <= 0 {
		return diag.Errorf("delete_cleanup_interval must be a positive duration (e.g., \"5m\"), got: %s", value)
	}
	return nil
}

func validateServerConfigAPIObjectMaxVersions(v interface{}, p cty.Path) diag.Diagnostics {
	value := v.(string)
	if value == "" {
		return diag.Errorf("object_max_versions must be a positive integer")
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return diag.Errorf("object_max_versions must be a positive integer, got: %s", value)
	}
	if n < 1 {
		return diag.Errorf("object_max_versions must be a positive integer, got: %d", n)
	}
	return nil
}

func validateServerConfigAPIReplicationMaxWorkers(v interface{}, p cty.Path) diag.Diagnostics {
	value := v.(string)
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return diag.Errorf("replication_max_workers must be an integer between 1 and 500, got: %s", value)
	}
	if n < 1 || n > 500 {
		return diag.Errorf("replication_max_workers must be between 1 and 500, got: %d", n)
	}
	return nil
}

func validateServerConfigAPIReplicationMaxLrgWorkers(v interface{}, p cty.Path) diag.Diagnostics {
	value := v.(string)
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return diag.Errorf("replication_max_lrg_workers must be an integer between 1 and 10, got: %s", value)
	}
	if n < 1 || n > 10 {
		return diag.Errorf("replication_max_lrg_workers must be between 1 and 10, got: %d", n)
	}
	return nil
}

func minioServerConfigApiDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin
	_, err := admin.DelConfigKV(ctx, "api")
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") {
			return NewResourceError("resetting api configuration", "api", err)
		}
	}
	d.SetId("")
	return nil
}
