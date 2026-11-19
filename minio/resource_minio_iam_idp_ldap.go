package minio

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceMinioIAMIdpLdap() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages an LDAP/Active Directory identity provider configuration for MinIO.",
		CreateContext: minioCreateIdpLdap,
		ReadContext:   minioReadIdpLdap,
		UpdateContext: minioUpdateIdpLdap,
		DeleteContext: minioDeleteIdpLdap,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"server_addr": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.All(validation.StringIsNotEmpty, validateConfigValue),
				Description:  "LDAP server address in host:port format (e.g., 'ldap.example.com:389').",
			},
			"lookup_bind_dn": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "",
				ValidateFunc: validateConfigValue,
				Description:  "Distinguished name used to bind to the LDAP server for user/group lookups (e.g., 'cn=admin,dc=example,dc=com').",
			},
			"lookup_bind_password": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				ValidateFunc: validateConfigValue,
				Description:  "Password for the lookup bind DN. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"user_dn_search_base_dn": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "",
				ValidateFunc: validateConfigValue,
				Description:  "Base DN for searching user entries (e.g., 'ou=users,dc=example,dc=com').",
			},
			"user_dn_search_filter": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "",
				ValidateFunc: validateConfigValue,
				Description:  "LDAP filter to locate a user by username (e.g., '(uid=%s)').",
			},
			"group_search_base_dn": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "",
				ValidateFunc: validateConfigValue,
				Description:  "Base DN for searching group entries (e.g., 'ou=groups,dc=example,dc=com').",
			},
			"group_search_filter": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "",
				ValidateFunc: validateConfigValue,
				Description:  "LDAP filter for group membership lookup (e.g., '(&(objectclass=groupOfNames)(member=%d))').",
			},
			"tls_skip_verify": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Disable TLS certificate verification. Not recommended for production.",
			},
			"server_insecure": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Allow plain (non-TLS) LDAP connections. Not recommended for production.",
			},
			"starttls": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Use STARTTLS to upgrade a plain LDAP connection to TLS.",
			},
			"enable": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Whether this LDAP configuration is enabled.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Indicates whether a MinIO server restart is required for the configuration to take effect.",
			},
		},
	}
}

func minioCreateIdpLdap(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := IdpLdapConfig(d, meta)

	tflog.Debug(ctx, "Creating LDAP IDP configuration")

	restart, err := config.MinioAdmin.SetConfigKV(ctx, "identity_ldap "+buildIdpLdapCfgData(config))
	if err != nil {
		return NewResourceError("creating LDAP IDP configuration", "ldap", err)
	}

	d.SetId("ldap")
	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", "ldap", setErr)
	}

	tflog.Debug(ctx, fmt.Sprintf("Created LDAP IDP configuration (restart_required=%v)", restart))

	return minioReadIdpLdap(ctx, d, meta)
}

func minioReadIdpLdap(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	tflog.Debug(ctx, "Reading LDAP IDP configuration")

	rawKV, err := minioAdmin.GetConfigKV(ctx, "identity_ldap")
	if err != nil {
		if isIDPConfigNotFound(err) {
			tflog.Warn(ctx, "LDAP IDP configuration no longer exists, removing from state")
			d.SetId("")
			return nil
		}
		return NewResourceError("reading LDAP IDP configuration", "ldap", err)
	}

	cfgStr := strings.TrimPrefix(strings.TrimSpace(string(rawKV)), "identity_ldap ")
	cfgMap := parseConfigParams(cfgStr)

	if d.Get("restart_required").(bool) {
		if !idpLdapRunningConfigMatchesState(IdpLdapConfig(d, meta), cfgMap) {
			// get-config-kv serves the pre-restart config while a restart is
			// pending, so loading it would store stale empty values; keep the
			// configured values in state. The notify targets use the same gate
			// (notifyRunningConfigMatchesState, issue #1304).
			tflog.Warn(ctx, "identity_ldap still shows the pre-restart config, keeping configured values in state")
			return nil
		}
		if err := d.Set("restart_required", false); err != nil {
			return NewResourceError("clearing restart_required", "ldap", err)
		}
		tflog.Debug(ctx, "identity_ldap config took effect after restart")
	}

	stringFields := []string{
		"server_addr",
		"lookup_bind_dn",
		"user_dn_search_base_dn",
		"user_dn_search_filter",
		"group_search_base_dn",
		"group_search_filter",
	}
	for _, field := range stringFields {
		if v, ok := cfgMap[field]; ok {
			if setErr := d.Set(field, v); setErr != nil {
				return NewResourceError("setting "+field, "ldap", setErr)
			}
		}
	}

	for _, m := range []struct {
		wire, schemaAttr string
	}{
		{"tls_skip_verify", "tls_skip_verify"},
		{"server_insecure", "server_insecure"},
		{"server_starttls", "starttls"},
	} {
		val := false
		if v, ok := cfgMap[m.wire]; ok {
			val = v == "on"
		}
		if setErr := d.Set(m.schemaAttr, val); setErr != nil {
			return NewResourceError("setting "+m.schemaAttr, "ldap", setErr)
		}
	}

	enableVal := true
	if v, ok := cfgMap["enable"]; ok && v != "" {
		enableVal = v == "on"
	}
	if setErr := d.Set("enable", enableVal); setErr != nil {
		return NewResourceError("setting enable", "ldap", setErr)
	}

	return nil
}

// idpLdapRunningConfigMatchesState compares the identity_ldap values the
// server reports with what the provider would write, using only the keys the
// provider sends. lookup_bind_password is never printed by the server, so a
// change to it alone cannot be seen here. enable is printed only when it is
// off, so the check is whether the server reports enable=off exactly when the
// config turns LDAP off. A key missing from the response counts as empty, so a
// cleared key matches even on a server that leaves empty keys out.
func idpLdapRunningConfigMatchesState(config *S3MinioIdpLdap, cfgMap map[string]string) bool {
	intended := parseConfigParams(buildIdpLdapCfgData(config))
	for key, want := range intended {
		if key == "lookup_bind_password" || key == "enable" {
			continue
		}
		if cfgMap[key] != want {
			return false
		}
	}
	return (intended["enable"] == "off") == (cfgMap["enable"] == "off")
}

func minioUpdateIdpLdap(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := IdpLdapConfig(d, meta)

	tflog.Debug(ctx, "Updating LDAP IDP configuration")

	restart, err := config.MinioAdmin.SetConfigKV(ctx, "identity_ldap "+buildIdpLdapCfgData(config))
	if err != nil {
		return NewResourceError("updating LDAP IDP configuration", "ldap", err)
	}

	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", "ldap", setErr)
	}

	tflog.Debug(ctx, fmt.Sprintf("Updated LDAP IDP configuration (restart_required=%v)", restart))

	return minioReadIdpLdap(ctx, d, meta)
}

func minioDeleteIdpLdap(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	tflog.Debug(ctx, "Deleting LDAP IDP configuration")

	_, err := minioAdmin.DelConfigKV(ctx, "identity_ldap")
	if err != nil {
		if isIDPConfigNotFound(err) {
			tflog.Warn(ctx, "LDAP IDP configuration already removed")
			d.SetId("")
			return nil
		}
		return NewResourceError("deleting LDAP IDP configuration", "ldap", err)
	}

	d.SetId("")
	tflog.Debug(ctx, "Deleted LDAP IDP configuration")

	return nil
}

func buildIdpLdapCfgData(config *S3MinioIdpLdap) string {
	var parts []string

	addStr := func(key, val string) {
		if val != "" {
			parts = append(parts, buildConfigParam(key, val))
		}
	}
	// set-config-kv merges into the stored config, so an optional string the
	// configuration removed is sent empty, or the server keeps the old value.
	// See issue #1348.
	addStrOrClear := func(key, val string) {
		parts = append(parts, buildConfigParam(key, val))
	}
	addBool := func(key string, val bool) {
		if val {
			parts = append(parts, key+"=on")
		} else {
			parts = append(parts, key+"=off")
		}
	}

	addStr("server_addr", config.ServerAddr)
	addStrOrClear("lookup_bind_dn", config.LookupBindDN)
	// The server never returns the password, so it is only sent when set:
	// an empty write would erase a password the provider cannot read back.
	addStr("lookup_bind_password", config.LookupBindPassword)
	addStrOrClear("user_dn_search_base_dn", config.UserDNSearchBaseDN)
	addStrOrClear("user_dn_search_filter", config.UserDNSearchFilter)
	addStrOrClear("group_search_base_dn", config.GroupSearchBaseDN)
	addStrOrClear("group_search_filter", config.GroupSearchFilter)
	addBool("tls_skip_verify", config.TLSSkipVerify)
	addBool("server_insecure", config.ServerInsecure)
	addBool("server_starttls", config.StartTLS)
	addBool("enable", config.Enable)

	return strings.Join(parts, " ")
}
