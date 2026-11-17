package minio

import (
	"strings"
	"testing"
)

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
		{name: "double quote together with whitespace", val: `say "hi" there`, wantErr: true},
		{name: "leading double quote", val: `"leading`, wantErr: true},
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
