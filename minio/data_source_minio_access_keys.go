package minio

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/madmin-go/v4"
)

const (
	accessKeyProviderBuiltin = "builtin"
	accessKeyProviderLDAP    = "ldap"
	accessKeyProviderOpenID  = "openid"
)

func accessKeyAccountSchema(extra map[string]*schema.Schema) *schema.Resource {
	attributes := map[string]*schema.Schema{
		"access_key": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Access key identifier. The secret is never returned by these APIs and is not exposed here.",
		},
		"parent_user": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "User the access key belongs to.",
		},
		"name": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Display name recorded on the access key. Empty when none was set.",
		},
		"description": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Description recorded on the access key. Empty when none was set.",
		},
		"status": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Account status, `on` or `off`.",
		},
		"expiration": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Time the access key stops working, in RFC 3339 format. Empty when the key does not expire.",
		},
	}
	for name, attribute := range extra {
		attributes[name] = attribute
	}
	return &schema.Resource{Schema: attributes}
}

func dataSourceMinioAccessKeys() *schema.Resource {
	return &schema.Resource{
		Description: "Lists the access keys a MinIO server holds, grouped by the identity provider each one comes from. " +
			"Use it to audit keys that Terraform does not manage, such as those minted through OpenID or LDAP logins. " +
			"Only identifiers and metadata are returned: the MinIO APIs behind this data source never return secret material.",
		ReadContext: dataSourceMinioAccessKeysRead,
		Schema: map[string]*schema.Schema{
			"identity_providers": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validation.StringInSlice([]string{accessKeyProviderBuiltin, accessKeyProviderLDAP, accessKeyProviderOpenID}, false),
				},
				Description: "Identity providers to query: `builtin`, `ldap`, `openid`. Defaults to `builtin` alone. " +
					"Ask for a provider only when the server has it configured: MinIO answers a request for an identity provider it does not run with an error, not with an empty list.",
			},
			"users": {
				Type:     schema.TypeList,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Description: "Restrict the listing to these users. Leave empty to list every user. " +
					"Builtin and OpenID take user names, LDAP takes user DNs, so a mixed query is rarely useful.",
			},
			"builtin": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Access keys belonging to users MinIO stores itself. Empty unless `builtin` is in `identity_providers`.",
				Elem: accessKeyAccountSchema(map[string]*schema.Schema{
					"user": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "User name the server grouped this key under.",
					},
				}),
			},
			"ldap": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Access keys belonging to users that authenticated through LDAP. Empty unless `ldap` is in `identity_providers`.",
				Elem: accessKeyAccountSchema(map[string]*schema.Schema{
					"user": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "User DN the server grouped this key under.",
					},
				}),
			},
			"openid": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Access keys belonging to users that authenticated through OpenID. Empty unless `openid` is in `identity_providers`.",
				Elem: accessKeyAccountSchema(map[string]*schema.Schema{
					"config_name": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "Name of the OpenID configuration the user authenticated against.",
					},
					"user_id": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "Value of the configured ID claim for the user.",
					},
					"readable_name": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "Value of the configured readable claim for the user. Empty when the configuration sets none.",
					},
				}),
			},
		},
	}
}

func dataSourceMinioAccessKeysRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	providers := getStringList(d.Get("identity_providers").([]interface{}))
	if len(providers) == 0 {
		providers = []string{accessKeyProviderBuiltin}
	}
	users := getStringList(d.Get("users").([]interface{}))

	opts := madmin.ListAccessKeysOpts{All: len(users) == 0}

	groups := map[string][]map[string]interface{}{
		accessKeyProviderBuiltin: {},
		accessKeyProviderLDAP:    {},
		accessKeyProviderOpenID:  {},
	}

	for _, provider := range providers {
		tflog.Debug(ctx, fmt.Sprintf("Listing %s access keys", provider))

		switch provider {
		case accessKeyProviderBuiltin:
			listed, err := admin.ListAccessKeysBulk(ctx, users, opts)
			if err != nil {
				return NewResourceError("listing builtin access keys", accessKeyProviderBuiltin, err)
			}
			groups[accessKeyProviderBuiltin] = flattenAccessKeysByUser(listed)
		case accessKeyProviderLDAP:
			listed, err := admin.ListAccessKeysLDAPBulkWithOpts(ctx, users, opts)
			if err != nil {
				return NewResourceError("listing LDAP access keys", accessKeyProviderLDAP, err)
			}
			converted := make(map[string]madmin.ListAccessKeysResp, len(listed))
			for user, resp := range listed {
				converted[user] = madmin.ListAccessKeysResp(resp)
			}
			groups[accessKeyProviderLDAP] = flattenAccessKeysByUser(converted)
		case accessKeyProviderOpenID:
			listed, err := admin.ListAccessKeysOpenIDBulk(ctx, users, opts)
			if err != nil {
				return NewResourceError("listing OpenID access keys", accessKeyProviderOpenID, err)
			}
			groups[accessKeyProviderOpenID] = flattenOpenIDAccessKeys(listed)
		}
	}

	d.SetId(strings.Join(providers, ",") + "|" + strings.Join(users, ","))

	for _, provider := range []string{accessKeyProviderBuiltin, accessKeyProviderLDAP, accessKeyProviderOpenID} {
		if err := d.Set(provider, groups[provider]); err != nil {
			return NewResourceError("setting "+provider, provider, err)
		}
	}

	return nil
}

func flattenAccessKeysByUser(listed map[string]madmin.ListAccessKeysResp) []map[string]interface{} {
	flattened := []map[string]interface{}{}

	users := make([]string, 0, len(listed))
	for user := range listed {
		users = append(users, user)
	}
	sort.Strings(users)

	for _, user := range users {
		for _, account := range listed[user].ServiceAccounts {
			entry := flattenServiceAccount(account)
			entry["user"] = user
			flattened = append(flattened, entry)
		}
	}

	sortAccessKeyEntries(flattened)
	return flattened
}

func flattenOpenIDAccessKeys(listed []madmin.ListAccessKeysOpenIDResp) []map[string]interface{} {
	flattened := []map[string]interface{}{}

	for _, config := range listed {
		for _, user := range config.Users {
			for _, account := range user.ServiceAccounts {
				entry := flattenServiceAccount(account)
				entry["config_name"] = config.ConfigName
				entry["user_id"] = user.ID
				entry["readable_name"] = user.ReadableName
				flattened = append(flattened, entry)
			}
		}
	}

	sortAccessKeyEntries(flattened)
	return flattened
}

func flattenServiceAccount(account madmin.ServiceAccountInfo) map[string]interface{} {
	expiration := ""
	if account.Expiration != nil && !account.Expiration.IsZero() {
		expiration = account.Expiration.UTC().Format(time.RFC3339)
	}

	return map[string]interface{}{
		"access_key":  account.AccessKey,
		"parent_user": account.ParentUser,
		"name":        account.Name,
		"description": account.Description,
		"status":      account.AccountStatus,
		"expiration":  expiration,
	}
}

func sortAccessKeyEntries(entries []map[string]interface{}) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i]["access_key"].(string) < entries[j]["access_key"].(string)
	})
}
