package minio

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioIAMIdpOpenId() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages an OpenID Connect (OIDC) identity provider configuration for MinIO SSO.",
		CreateContext: minioCreateIdpOpenId,
		ReadContext:   minioReadIdpOpenId,
		UpdateContext: minioUpdateIdpOpenId,
		DeleteContext: minioDeleteIdpOpenId,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "_",
				ForceNew:    true,
				Description: "Name for this OIDC configuration. Use '_' (default) for the primary configuration or any identifier for named configurations.",
			},
			"config_url": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "URL to the OpenID Connect discovery document (e.g., https://accounts.example.com/.well-known/openid-configuration).",
			},
			"client_id": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "OAuth2 client ID registered with the identity provider.",
			},
			"client_secret": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
				// MinIO never returns the real secret (always "REDACTED"), so Read
				// deliberately skips d.Set for this field. Terraform retains whatever
				// the user configured, which means secret rotation produces a real diff.
				Description: "OAuth2 client secret registered with the identity provider. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"claim_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "JWT claim attribute used to identify the policy for the authenticated user. Defaults to 'policy'. Cannot be set together with role_policy.",
			},
			"claim_prefix": {
				Type:          schema.TypeString,
				Optional:      true,
				Default:       "",
				ConflictsWith: []string{"role_policy"},
				Description:   "Prefix to apply to JWT claim values when looking up policies. Cannot be set together with role_policy.",
			},
			"scopes": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "Comma-separated list of OAuth2 scopes to request (e.g., 'openid,email,profile').",
			},
			"redirect_uri": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "Redirect URI registered with the identity provider for the OAuth2 callback.",
			},
			"display_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "Display name for this identity provider shown on the MinIO login screen.",
			},
			"comment": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "Comment or description for this OIDC configuration.",
			},
			"role_policy": {
				Type:          schema.TypeString,
				Optional:      true,
				Default:       "",
				ConflictsWith: []string{"claim_name", "claim_prefix"},
				Description:   "Policy for role-based OIDC access. When set, MinIO uses a role ARN approach and ignores claim_name/claim_prefix. Cannot be set together with claim_name or claim_prefix.",
			},
			"enable": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Whether this OIDC configuration is enabled.",
			},
			"restart_required": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Indicates whether a MinIO server restart is required for the configuration to take effect.",
			},
		},
	}
}

func minioCreateIdpOpenId(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := IdpOpenIdConfig(d, meta)

	log.Printf("[DEBUG] Creating OIDC IDP configuration: %s", config.Name)

	cfgData := buildIdpOpenIdCfgData(config)
	restart, err := config.MinioAdmin.AddOrUpdateIDPConfig(ctx, madmin.OpenidIDPCfg, config.Name, cfgData, false)
	if err != nil {
		return NewResourceError("creating OIDC IDP configuration", config.Name, err)
	}

	d.SetId(config.Name)
	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", config.Name, setErr)
	}

	log.Printf("[DEBUG] Created OIDC IDP configuration: %s (restart_required=%v)", config.Name, restart)

	return minioReadIdpOpenId(ctx, d, meta)
}

func minioReadIdpOpenId(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin
	cfgName := d.Id()

	log.Printf("[DEBUG] Reading OIDC IDP configuration: %s", cfgName)

	cfg, err := minioAdmin.GetIDPConfig(ctx, madmin.OpenidIDPCfg, cfgName)
	if err != nil {
		if isIDPConfigNotFound(err) {
			log.Printf("[WARN] OIDC IDP configuration %s no longer exists, removing from state", cfgName)
			d.SetId("")
			return nil
		}
		return NewResourceError("reading OIDC IDP configuration", cfgName, err)
	}

	cfgMap := idpCfgInfoToMap(cfg.Info)

	if setErr := d.Set("name", cfgName); setErr != nil {
		return NewResourceError("setting name", cfgName, setErr)
	}

	if v, ok := cfgMap["config_url"]; ok {
		if setErr := d.Set("config_url", v); setErr != nil {
			return NewResourceError("setting config_url", cfgName, setErr)
		}
	}

	if v, ok := cfgMap["client_id"]; ok {
		if setErr := d.Set("client_id", v); setErr != nil {
			return NewResourceError("setting client_id", cfgName, setErr)
		}
	}

	// client_secret is never set in Read: MinIO always returns "REDACTED" and never
	// the actual value. Following the AWS RDS password pattern, we leave the field
	// untouched so Terraform state retains whatever the user configured and secret
	// rotation triggers a real plan diff.

	if v, ok := cfgMap["claim_name"]; ok {
		if setErr := d.Set("claim_name", v); setErr != nil {
			return NewResourceError("setting claim_name", cfgName, setErr)
		}
	}

	if v, ok := cfgMap["claim_prefix"]; ok {
		if setErr := d.Set("claim_prefix", v); setErr != nil {
			return NewResourceError("setting claim_prefix", cfgName, setErr)
		}
	}

	if v, ok := cfgMap["scopes"]; ok {
		if setErr := d.Set("scopes", v); setErr != nil {
			return NewResourceError("setting scopes", cfgName, setErr)
		}
	}

	if v, ok := cfgMap["redirect_uri"]; ok {
		if setErr := d.Set("redirect_uri", v); setErr != nil {
			return NewResourceError("setting redirect_uri", cfgName, setErr)
		}
	}

	if v, ok := cfgMap["display_name"]; ok {
		if setErr := d.Set("display_name", v); setErr != nil {
			return NewResourceError("setting display_name", cfgName, setErr)
		}
	}

	if v, ok := cfgMap["comment"]; ok {
		if setErr := d.Set("comment", v); setErr != nil {
			return NewResourceError("setting comment", cfgName, setErr)
		}
	}

	if v, ok := cfgMap["role_policy"]; ok {
		if setErr := d.Set("role_policy", v); setErr != nil {
			return NewResourceError("setting role_policy", cfgName, setErr)
		}
	}

	if v, ok := cfgMap["enable"]; ok {
		if setErr := d.Set("enable", v == "on"); setErr != nil {
			return NewResourceError("setting enable", cfgName, setErr)
		}
	}

	return nil
}

func minioUpdateIdpOpenId(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := IdpOpenIdConfig(d, meta)

	log.Printf("[DEBUG] Updating OIDC IDP configuration: %s", config.Name)

	cfgData := buildIdpOpenIdCfgData(config)
	restart, err := config.MinioAdmin.AddOrUpdateIDPConfig(ctx, madmin.OpenidIDPCfg, config.Name, cfgData, true)
	if err != nil {
		return NewResourceError("updating OIDC IDP configuration", config.Name, err)
	}

	if setErr := d.Set("restart_required", restart); setErr != nil {
		return NewResourceError("setting restart_required", config.Name, setErr)
	}

	log.Printf("[DEBUG] Updated OIDC IDP configuration: %s (restart_required=%v)", config.Name, restart)

	return minioReadIdpOpenId(ctx, d, meta)
}

func minioDeleteIdpOpenId(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	minioAdmin := meta.(*S3MinioClient).S3Admin
	cfgName := d.Id()

	log.Printf("[DEBUG] Deleting OIDC IDP configuration: %s", cfgName)

	_, err := minioAdmin.DeleteIDPConfig(ctx, madmin.OpenidIDPCfg, cfgName)
	if err != nil {
		if isIDPConfigNotFound(err) {
			log.Printf("[WARN] OIDC IDP configuration %s already removed", cfgName)
			d.SetId("")
			return nil
		}
		return NewResourceError("deleting OIDC IDP configuration", cfgName, err)
	}

	d.SetId("")
	log.Printf("[DEBUG] Deleted OIDC IDP configuration: %s", cfgName)

	return nil
}

// buildIdpOpenIdCfgData constructs the space-separated key=value configuration
// string that the MinIO IDP config API expects. Values containing whitespace are
// quoted to prevent the MinIO config parser from misinterpreting them as separate keys.
func buildIdpOpenIdCfgData(config *S3MinioIdpOpenId) string {
	var parts []string

	addParam := func(key, val string) {
		if val != "" {
			if strings.ContainsAny(val, " \t") {
				parts = append(parts, fmt.Sprintf("%s=%q", key, val))
			} else {
				parts = append(parts, fmt.Sprintf("%s=%s", key, val))
			}
		}
	}

	addParam("config_url", config.ConfigURL)
	addParam("client_id", config.ClientID)
	addParam("client_secret", config.ClientSecret)
	addParam("claim_name", config.ClaimName)
	addParam("claim_prefix", config.ClaimPrefix)
	addParam("scopes", config.Scopes)
	addParam("redirect_uri", config.RedirectURI)
	addParam("display_name", config.DisplayName)
	addParam("comment", config.Comment)
	addParam("role_policy", config.RolePolicy)

	if config.Enable {
		parts = append(parts, "enable=on")
	} else {
		parts = append(parts, "enable=off")
	}

	return strings.Join(parts, " ")
}

// idpCfgInfoToMap converts a slice of IDPCfgInfo into a simple key→value map,
// including only entries that are actual config values (IsCfg=true).
func idpCfgInfoToMap(info []madmin.IDPCfgInfo) map[string]string {
	m := make(map[string]string, len(info))
	for _, item := range info {
		if item.IsCfg {
			m[item.Key] = item.Value
		}
	}
	return m
}

// isIDPConfigNotFound returns true when the error indicates the IDP configuration
// does not exist on the server. "invalid config type" is intentionally excluded:
// it signals an unsupported cfgType or an older MinIO that doesn't implement the
// IDP config API at all, which should surface as a real error rather than
// silently clearing state.
func isIDPConfigNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no such")
}
