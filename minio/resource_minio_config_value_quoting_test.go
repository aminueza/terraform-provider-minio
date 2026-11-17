package minio

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

const badValue = `say "hi" there`

func TestBuildConfigParam(t *testing.T) {
	cases := []struct {
		name string
		key  string
		val  string
		want string
	}{
		{
			name: "plain value",
			key:  "endpoint",
			val:  "http://hook:8080",
			want: "endpoint=http://hook:8080",
		},
		{
			name: "value with a space is quoted",
			key:  "comment",
			val:  "primary webhook",
			want: `comment="primary webhook"`,
		},
		{
			name: "value with a tab is quoted",
			key:  "comment",
			val:  "a\tb",
			want: "comment=\"a\tb\"",
		},
		{
			name: "windows path keeps single backslashes",
			key:  "comment",
			val:  `C:\path to\dir`,
			want: `comment="C:\path to\dir"`,
		},
		{
			name: "double quote without whitespace stays unquoted",
			key:  "comment",
			val:  `say"hi`,
			want: `comment=say"hi`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildConfigParam(tc.key, tc.val)
			if got != tc.want {
				t.Fatalf("buildConfigParam(%q, %q) = %q, want %q", tc.key, tc.val, got, tc.want)
			}
		})
	}
}

func TestBuildConfigParamRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{name: "value with spaces", val: "primary webhook"},
		{name: "windows path", val: `C:\path to\dir`},
		{name: "plain value", val: "plain"},
		{name: "double quote without whitespace", val: `say"hi`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseConfigParams(buildConfigParam("comment", tc.val))["comment"]
			if got != tc.val {
				t.Fatalf("round trip of %q = %q, want %q", tc.val, got, tc.val)
			}
		})
	}
}

func TestBuildNotifyWebhookCfgQuotesWindowsPath(t *testing.T) {
	d := resourceMinioNotifyWebhook().TestResourceData()
	d.SetId("primary")
	if err := d.Set("name", "primary"); err != nil {
		t.Fatalf("setting name: %v", err)
	}
	if err := d.Set("endpoint", "http://hook:8080"); err != nil {
		t.Fatalf("setting endpoint: %v", err)
	}
	if err := d.Set("comment", `C:\path to\dir`); err != nil {
		t.Fatalf("setting comment: %v", err)
	}

	got := buildNotifyWebhookCfg(d, nil)
	want := `comment="C:\path to\dir"`
	if !strings.Contains(got, want) {
		t.Fatalf("buildNotifyWebhookCfg() = %q, want it to contain %q", got, want)
	}
}

func TestValidateConfigValue(t *testing.T) {
	cases := []struct {
		name    string
		val     string
		wantErr bool
	}{
		{name: "double quote together with whitespace", val: badValue, wantErr: true},
		{name: "leading double quote", val: `"leading`, wantErr: true},
		{name: "line feed in value", val: "a\nb", wantErr: true},
		{name: "carriage return in value", val: "a\rb", wantErr: true},
		{name: "windows path", val: `C:\path to\dir`},
		{name: "double quote without whitespace", val: `say"hi`},
		{name: "value with spaces", val: "primary webhook"},
		{name: "empty value", val: ""},
		{name: "double quote inside a word", val: `a"b`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warns, errs := validateConfigValue(tc.val, "comment")
			if len(warns) != 0 {
				t.Errorf("warnings = %v, want none", warns)
			}
			if tc.wantErr {
				if len(errs) != 1 {
					t.Fatalf("errors = %v, want exactly 1", errs)
				}
				return
			}
			if len(errs) != 0 {
				t.Fatalf("errors = %v, want none", errs)
			}
		})
	}
}

func TestValidateConfigValueWired(t *testing.T) {
	cases := []struct {
		name    string
		res     *schema.Resource
		raw     map[string]interface{}
		want    string
		wantNot string
	}{
		{
			name: "notify webhook comment",
			res:  resourceMinioNotifyWebhook(),
			raw: map[string]interface{}{
				"name":     "primary",
				"endpoint": "http://hook:8080",
				"comment":  badValue,
			},
			want: `"comment" cannot contain a double quote together with whitespace`,
		},
		{
			name: "notify webhook queue_dir",
			res:  resourceMinioNotifyWebhook(),
			raw: map[string]interface{}{
				"name":      "primary",
				"endpoint":  "http://hook:8080",
				"queue_dir": badValue,
			},
			want: `"queue_dir" cannot contain a double quote together with whitespace`,
		},
		{
			name: "notify webhook endpoint",
			res:  resourceMinioNotifyWebhook(),
			raw: map[string]interface{}{
				"name":     "primary",
				"endpoint": badValue,
			},
			want: `"endpoint" cannot contain a double quote together with whitespace`,
		},
		{
			name: "audit webhook endpoint",
			res:  resourceMinioAuditWebhook(),
			raw: map[string]interface{}{
				"name":     "t",
				"endpoint": badValue,
			},
			want: `"endpoint" cannot contain a double quote together with whitespace`,
		},
		{
			name: "ldap lookup_bind_dn",
			res:  resourceMinioIAMIdpLdap(),
			raw: map[string]interface{}{
				"server_addr":    "ldap:389",
				"lookup_bind_dn": badValue,
			},
			want: `"lookup_bind_dn" cannot contain a double quote together with whitespace`,
		},
		{
			name: "openid config_url",
			res:  resourceMinioIAMIdpOpenId(),
			raw: map[string]interface{}{
				"config_url": badValue,
			},
			want: `"config_url" cannot contain a double quote together with whitespace`,
		},
		{
			name: "openid client_secret_wo",
			res:  resourceMinioIAMIdpOpenId(),
			raw: map[string]interface{}{
				"config_url":               "https://idp/.well-known/openid-configuration",
				"client_id":                "cid",
				"client_secret_wo":         badValue,
				"client_secret_wo_version": 1,
			},
			want: `"client_secret_wo" cannot contain a double quote together with whitespace`,
		},
		{
			name: "region comment",
			res:  resourceMinioServerConfigRegion(),
			raw: map[string]interface{}{
				"name":    "r",
				"comment": badValue,
			},
			want: `"comment" cannot contain a double quote together with whitespace`,
		},
		{
			name: "etcd comment",
			res:  resourceMinioServerConfigEtcd(),
			raw: map[string]interface{}{
				"endpoints": "http://e:2379",
				"comment":   badValue,
			},
			want: `"comment" cannot contain a double quote together with whitespace`,
		},
		{
			name: "notify webhook name is not a config value",
			res:  resourceMinioNotifyWebhook(),
			raw: map[string]interface{}{
				"name":     badValue,
				"endpoint": "http://hook:8080",
			},
			wantNot: `"name"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := tc.res.Validate(terraform.NewResourceConfigRaw(tc.raw))
			var parts []string
			for _, d := range diags {
				parts = append(parts, d.Summary, d.Detail)
			}
			joined := strings.Join(parts, " ")
			if tc.wantNot != "" {
				if strings.Contains(joined, tc.wantNot) {
					t.Fatalf("diagnostics = %q, must not mention %q", joined, tc.wantNot)
				}
				return
			}
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("diagnostics = %q, want it to contain %q", joined, tc.want)
			}
		})
	}
}
