package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// resourceMinioServerConfigEtcdUpgradeV0 resets restart_required in v0
// states: the pre-gate provider set the flag but never cleared it, so the
// stored true refers to no pending restart and the read gate must not trust
// it. After the reset the first refresh reads the server's real values, and
// a removal that the old provider could not clear shows up in the plan.
func resourceMinioServerConfigEtcdUpgradeV0(ctx context.Context, rawState map[string]interface{}, meta interface{}) (map[string]interface{}, error) {
	rawState["restart_required"] = false
	return rawState, nil
}

func resourceMinioServerConfigEtcd() *schema.Resource {
	r := &schema.Resource{
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
			// MinIO only uses the client certificate together with its key, so
			// RequiredWith keeps a cert-only configuration out of the plan. The
			// key alone stays allowed: dropping the pair from the configuration
			// sends both keys empty, which is how the server clears them
			// (issue #1439).
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
				Description: "Whether a MinIO server restart is required to apply the configuration. While it is true the read cannot tell a pending restart from an external change that was already restarted, so etcd config changed outside Terraform is only reported after the flag clears.",
			},
		},
		// v0 states carry a restart_required that the pre-gate provider set
		// but never cleared; the upgrade resets it (see
		// resourceMinioServerConfigEtcdUpgradeV0). The v0 type below is the
		// live schema: snapshot it into its own function before changing any
		// field of this resource, or "v0" silently starts meaning something
		// else.
		SchemaVersion: 1,
	}
	r.StateUpgraders = []schema.StateUpgrader{
		{
			Type:    r.CoreConfigSchema().ImpliedType(),
			Version: 0,
			Upgrade: resourceMinioServerConfigEtcdUpgradeV0,
		},
	}
	return r
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
	// MinIO merges set-config-kv into the subsystem's current settings, so a
	// key left out keeps its old value on the server. Always send the
	// clearable pair, empty included: an empty key= clears the stored value
	// (the documented clear mechanism mc uses), and the read gate can then
	// trust that the server copy of the pair matches the configuration once
	// the restart lands.
	clearableParam := func(key string) {
		val := d.Get(key).(string)
		if val == "" {
			parts = append(parts, key+"=")
			return
		}
		parts = append(parts, buildConfigParam(key, val))
	}
	clearableParam("client_cert")
	clearableParam("client_cert_key")
	if v, ok := d.GetOk("comment"); ok {
		addParam("comment", v.(string))
	}

	configString := "etcd " + strings.Join(parts, " ")
	restart, err := admin.SetConfigKV(ctx, configString)
	if err != nil {
		return NewResourceError("setting etcd configuration", "etcd", err)
	}

	d.SetId("etcd")
	if diags := setResourceField(d, "restart_required", restart, "etcd"); diags != nil {
		return diags
	}
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

	// get-config-kv prints the values on a line starting with "etcd ", and
	// when any MINIO_ETCD_* environment variable is set it prints a
	// "# MINIO_ETCD_...=..." line before it, so the values are located line
	// by line the way the notify targets read theirs.
	cfgMap, _ := notifyTargetConfigParams(string(configData), "etcd")
	d.SetId("etcd")

	// get-config-kv serves the pre-restart config while a restart is pending,
	// so a response that does not match the configured values is stale and the
	// state keeps them. Once the server has restarted the response matches and
	// the flag is cleared. The site and policy plugin configs gate on the same
	// signal. The gate cannot tell a stale pre-restart response from an etcd
	// config changed outside Terraform and restarted before the next refresh:
	// that drift stays hidden, and restart_required stays true, until the
	// server is restarted again. Site, LDAP and policy plugin accept the same
	// trade-off.
	if d.Get("restart_required").(bool) {
		if !etcdRunningConfigMatchesState(d, cfgMap) {
			tflog.Warn(ctx, "etcd still shows the pre-restart config, keeping configured values in state")
			return nil
		}
		if err := d.Set("restart_required", false); err != nil {
			return NewResourceError("clearing restart_required", "etcd", err)
		}
	}

	for _, field := range []string{"endpoints", "path_prefix", "coredns_path"} {
		if v, ok := cfgMap[field]; ok && v != "" {
			if diags := setResourceField(d, field, v, d.Id()); diags != nil {
				return diags
			}
		}
	}
	// client_cert is read even when empty: the gate above returns early on a
	// stale pre-restart response, so an empty value here is the server's real
	// answer and the state must converge to it, including for clears made
	// outside Terraform.
	if v, ok := cfgMap["client_cert"]; ok {
		if diags := setResourceField(d, "client_cert", v, d.Id()); diags != nil {
			return diags
		}
	}
	if v, ok := cfgMap["client_cert_key"]; ok {
		if diags := setResourceField(d, "client_cert_key", v, d.Id()); diags != nil {
			return diags
		}
	}
	if v, ok := cfgMap["comment"]; ok && v != "" {
		if diags := setResourceField(d, "comment", v, d.Id()); diags != nil {
			return diags
		}
	}

	return nil
}

func etcdRunningConfigMatchesState(d *schema.ResourceData, cfgMap map[string]string) bool {
	// Keys the builder leaves out (an empty path_prefix, coredns_path or
	// comment) have nothing to compare. The clearable certificate pair must
	// also come back empty when the state holds it empty, which is how a
	// removal converges once the restart lands.
	for _, key := range []string{"endpoints", "path_prefix", "coredns_path", "comment"} {
		want := d.Get(key).(string)
		if want == "" {
			continue
		}
		if got, ok := cfgMap[key]; !ok || got != want {
			return false
		}
	}
	for _, key := range []string{"client_cert", "client_cert_key"} {
		if cfgMap[key] != d.Get(key).(string) {
			return false
		}
	}
	return true
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
