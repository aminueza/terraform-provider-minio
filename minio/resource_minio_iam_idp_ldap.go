package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/madmin-go/v3"
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
				ValidateFunc: validation.StringIsNotEmpty,
				Description:  "LDAP server address in host:port format (e.g., 'ldap.example.com:389').",
			},
			"lookup_bind_dn": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "Distinguished name used to bind to the LDAP server for user/group lookups (e.g., 'cn=admin,dc=example,dc=com').",
			},
			"lookup_bind_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Password for the lookup bind DN. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"user_dn_search_base_dn": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "Base DN for searching user entries (e.g., 'ou=users,dc=example,dc=com').",
			},
			"user_dn_search_filter": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "LDAP filter to locate a user by username (e.g., '(uid=%s)').",
			},
			"group_search_base_dn": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "Base DN for searching group entries (e.g., 'ou=groups,dc=example,dc=com').",
			},
			"group_search_filter": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "LDAP filter for group membership lookup (e.g., '(&(objectclass=groupOfNames)(member=%d))').",
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

	log.Printf("[DEBUG] Creating LDAP IDP configuration")

	cfgData := buildIdpLdapCfgData(config)
	restart, err := config.MinioAdmin.AddOrUpdateIDPConfig(ctx, madmin.LDAPIDPCfg, madmin.Default, cfgData, false)
	if err != nil {
		return NewResourceError("creating LDAP IDP configuration", "ldap", err)
	}

	d.SetId("ldap")
	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", "ldap", setErr)
	}

	log.Printf("[DEBUG] Created LDAP IDP configuration (restart_required=%v)", restart)

	return minioReadIdpLdap(ctx, d, meta)
}

func minioReadIdpLdap(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	log.Printf("[DEBUG] Reading LDAP IDP configuration")

	cfg, err := minioAdmin.GetIDPConfig(ctx, madmin.LDAPIDPCfg, madmin.Default)
	if err != nil {
		if isIDPConfigNotFound(err) {
			log.Printf("[WARN] LDAP IDP configuration no longer exists, removing from state")
			d.SetId("")
			return nil
		}
		return NewResourceError("reading LDAP IDP configuration", "ldap", err)
	}

	cfgMap := idpCfgInfoToMap(cfg.Info)

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

	for _, field := range []string{"tls_skip_verify", "server_insecure", "starttls"} {
		val := false
		if v, ok := cfgMap[field]; ok {
			val = v == "on"
		}
		if setErr := d.Set(field, val); setErr != nil {
			return NewResourceError("setting "+field, "ldap", setErr)
		}
	}

	enableVal := true
	if v, ok := cfgMap["enable"]; ok {
		enableVal = v == "on"
	}
	if setErr := d.Set("enable", enableVal); setErr != nil {
		return NewResourceError("setting enable", "ldap", setErr)
	}

	return nil
}

func minioUpdateIdpLdap(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := IdpLdapConfig(d, meta)

	log.Printf("[DEBUG] Updating LDAP IDP configuration")

	cfgData := buildIdpLdapCfgData(config)
	restart, err := config.MinioAdmin.AddOrUpdateIDPConfig(ctx, madmin.LDAPIDPCfg, madmin.Default, cfgData, true)
	if err != nil {
		return NewResourceError("updating LDAP IDP configuration", "ldap", err)
	}

	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", "ldap", setErr)
	}

	log.Printf("[DEBUG] Updated LDAP IDP configuration (restart_required=%v)", restart)

	return minioReadIdpLdap(ctx, d, meta)
}

func minioDeleteIdpLdap(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	log.Printf("[DEBUG] Deleting LDAP IDP configuration")

	_, err := minioAdmin.DeleteIDPConfig(ctx, madmin.LDAPIDPCfg, madmin.Default)
	if err != nil {
		if isIDPConfigNotFound(err) {
			log.Printf("[WARN] LDAP IDP configuration already removed")
			d.SetId("")
			return nil
		}
		return NewResourceError("deleting LDAP IDP configuration", "ldap", err)
	}

	d.SetId("")
	log.Printf("[DEBUG] Deleted LDAP IDP configuration")

	return nil
}

func buildIdpLdapCfgData(config *S3MinioIdpLdap) string {
	var parts []string

	addStr := func(key, val string) {
		if val != "" {
			if strings.ContainsAny(val, " \t") {
				parts = append(parts, fmt.Sprintf("%s=%q", key, val))
			} else {
				parts = append(parts, fmt.Sprintf("%s=%s", key, val))
			}
		}
	}
	addBool := func(key string, val bool) {
		if val {
			parts = append(parts, key+"=on")
		} else {
			parts = append(parts, key+"=off")
		}
	}

	addStr("server_addr", config.ServerAddr)
	addStr("lookup_bind_dn", config.LookupBindDN)
	addStr("lookup_bind_password", config.LookupBindPassword)
	addStr("user_dn_search_base_dn", config.UserDNSearchBaseDN)
	addStr("user_dn_search_filter", config.UserDNSearchFilter)
	addStr("group_search_base_dn", config.GroupSearchBaseDN)
	addStr("group_search_filter", config.GroupSearchFilter)
	addBool("tls_skip_verify", config.TLSSkipVerify)
	addBool("server_insecure", config.ServerInsecure)
	addBool("starttls", config.StartTLS)
	addBool("enable", config.Enable)

	return strings.Join(parts, " ")
}
