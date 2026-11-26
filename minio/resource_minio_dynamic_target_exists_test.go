package minio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type dynamicTargetCase struct {
	resourceType string
	subsystem    string
	outsideCfg string
	config func(name string) string
}

func dynamicTargetCases() []dynamicTargetCase {
	return []dynamicTargetCase{
		{
			resourceType: "minio_logger_webhook",
			subsystem:    "logger_webhook",
			outsideCfg:   "endpoint=http://outside.example.com/log enable=off",
			config: func(name string) string {
				return fmt.Sprintf(`
resource "minio_logger_webhook" "test" {
  name     = %q
  endpoint = "http://terraform.example.com/log"
  enable   = false
}
`, name)
			},
		},
		{
			resourceType: "minio_audit_webhook",
			subsystem:    "audit_webhook",
			outsideCfg:   "endpoint=http://outside.example.com/audit enable=off",
			config: func(name string) string {
				return fmt.Sprintf(`
resource "minio_audit_webhook" "test" {
  name     = %q
  endpoint = "http://terraform.example.com/audit"
  enable   = false
}
`, name)
			},
		},
		{
			resourceType: "minio_audit_kafka",
			subsystem:    "audit_kafka",
			outsideCfg:   "brokers=outside.example.com:9092 topic=outside enable=off",
			config: func(name string) string {
				return fmt.Sprintf(`
resource "minio_audit_kafka" "test" {
  name    = %q
  brokers = "terraform.example.com:9092"
  topic   = "terraform"
  enable  = false
}
`, name)
			},
		},
	}
}

// testAccCheckDynamicTargetParam reads the target straight from the server.
// These subsystems apply without a restart, so no restart is needed first.
func testAccCheckDynamicTargetParam(subsystem, name, key, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		target := notifyConfigKey(subsystem, name)
		data, err := testAccClient().S3Admin.GetConfigKV(context.Background(), target)
		if err != nil {
			return fmt.Errorf("reading %s: %w", target, err)
		}
		params, found := notifyTargetConfigParams(string(data), target)
		if !found {
			return fmt.Errorf("%s is not on the server; response: %s", target, data)
		}
		if got := params[key]; got != want {
			return fmt.Errorf("%q of %s on the server is %q, want %q", key, target, got, want)
		}
		return nil
	}
}

func testAccCheckDynamicTargetGone(subsystem, name string) error {
	target := notifyConfigKey(subsystem, name)
	_, err := testAccClient().S3Admin.GetConfigKV(context.Background(), target)
	if err == nil {
		return fmt.Errorf("%s is still on the server", target)
	}
	if !isNotifyTargetMissingError(err) {
		return fmt.Errorf("reading %s: %w", target, err)
	}
	return nil
}

func TestAccMinioDynamicTargets_rejectExisting(t *testing.T) {
	for _, tc := range dynamicTargetCases() {
		t.Run(tc.resourceType, func(t *testing.T) {
			outside := "tfacc-outside-" + acctest.RandString(6)
			fresh := "tfacc-fresh-" + acctest.RandString(6)
			outsideKey := notifyConfigKey(tc.subsystem, outside)
			outsideParam, outsideValue, _ := strings.Cut(strings.Fields(tc.outsideCfg)[0], "=")

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:          func() { testAccPreCheck(t) },
				ProviderFactories: testAccProviders,
				CheckDestroy: func(*terraform.State) error {
					if _, err := testAccClient().S3Admin.DelConfigKV(context.Background(), outsideKey); err != nil && !isNotifyTargetMissingError(err) {
						return fmt.Errorf("removing %s: %w", outsideKey, err)
					}
					return testAccCheckDynamicTargetGone(tc.subsystem, fresh)
				},
				Steps: []resource.TestStep{
					{
						PreConfig: func() {
							if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), outsideKey+" "+tc.outsideCfg); err != nil {
								t.Fatalf("creating %s outside Terraform: %v", outsideKey, err)
							}
						},
						Config:      tc.config(outside),
						ExpectError: regexp.MustCompile(`already exists\. To manage it with Terraform, import it`),
					},
					{
						Config: tc.config(fresh),
						Check: resource.ComposeTestCheckFunc(
							testAccCheckDynamicTargetParam(tc.subsystem, outside, outsideParam, outsideValue),
							resource.TestCheckResourceAttr(tc.resourceType+".test", "name", fresh),
						),
					},
					{
						Config: tc.config(fresh),
						Taint:  []string{tc.resourceType + ".test"},
						Check:  resource.TestCheckResourceAttr(tc.resourceType+".test", "name", fresh),
					},
					{
						Config: `locals {}`,
						Check: func(*terraform.State) error {
							return testAccCheckDynamicTargetGone(tc.subsystem, fresh)
						},
					},
					{
						Config: tc.config(fresh),
						Check:  resource.TestCheckResourceAttr(tc.resourceType+".test", "name", fresh),
					},
				},
			})
		})
	}
}

func TestAccMinioLoggerWebhook_sameNameTwiceInOneApply(t *testing.T) {
	name := "tfacc-twice-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			return testAccCheckDynamicTargetGone("logger_webhook", name)
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "minio_logger_webhook" "first" {
  name     = %[1]q
  endpoint = "http://first.example.com/log"
  enable   = false
}

resource "minio_logger_webhook" "second" {
  name     = %[1]q
  endpoint = "http://second.example.com/log"
  enable   = false
}
`, name),
				ExpectError: regexp.MustCompile(`minio_logger_webhook "` + name + `" already exists`),
			},
		},
	})
}

func TestRejectExistingDynamicTarget(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		encrypt bool
		wantErr string
	}{
		{
			name:    "a target on the server is rejected with the import hint",
			status:  http.StatusOK,
			body:    "logger_webhook:alpha endpoint=http://outside/log enable=off",
			encrypt: true,
			wantErr: `minio_logger_webhook "alpha" already exists. To manage it with Terraform, import it: terraform import minio_logger_webhook.<name> alpha`,
		},
		{
			name:   "a missing target lets the create go on",
			status: http.StatusBadRequest,
			body:   `{"Code":"XMinioConfigError","Message":"there is no target ` + "`alpha`" + ` for subsystem ` + "`logger_webhook`" + `"}`,
		},
		{
			name:    "any other lookup error fails the create",
			status:  http.StatusForbidden,
			body:    `{"Code":"AccessDenied","Message":"Access Denied."}`,
			wantErr: `checking whether logger_webhook target "alpha" already exists: Access Denied.`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/get-config-kv") {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				body := []byte(tc.body)
				if tc.encrypt {
					enc, err := madmin.EncryptData("secretkey", body)
					if err != nil {
						t.Fatalf("encrypting: %v", err)
					}
					body = enc
				}
				w.WriteHeader(tc.status)
				_, _ = io.Copy(w, strings.NewReader(string(body)))
			}))
			defer srv.Close()

			admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
				Creds: credentials.NewStaticV4("accesskey", "secretkey", ""),
			})
			if err != nil {
				t.Fatalf("creating admin client: %v", err)
			}

			err = rejectExistingDynamicTarget(context.Background(), admin, "logger_webhook", "alpha")
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("err = %v, want nil", err)
			case tc.wantErr != "" && (err == nil || err.Error() != tc.wantErr):
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestDynamicTargetCreateFailureKeepsTheID(t *testing.T) {
	cases := []struct {
		name   string
		res    *schema.Resource
		config map[string]interface{}
	}{
		{"minio_logger_webhook", resourceMinioLoggerWebhook(), map[string]interface{}{"name": "primary", "endpoint": "http://x.example.com/log", "enable": false}},
		{"minio_audit_webhook", resourceMinioAuditWebhook(), map[string]interface{}{"name": "primary", "endpoint": "http://x.example.com/audit", "enable": false}},
		{"minio_audit_kafka", resourceMinioAuditKafka(), map[string]interface{}{"name": "primary", "brokers": "x.example.com:9092", "topic": "t", "enable": false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sets int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/get-config-kv"):
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"Code":"XMinioConfigError","Message":"there is no target primary"}`)
				case strings.HasSuffix(r.URL.Path, "/set-config-kv"):
					sets++
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"Code":"XMinioAdminConfigBadJSON","Message":"the response was lost"}`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
				Creds: credentials.NewStaticV4("accesskey", "secretkey", ""),
			})
			if err != nil {
				t.Fatalf("creating admin client: %v", err)
			}

			d := schema.TestResourceDataRaw(t, tc.res.Schema, tc.config)
			diags := tc.res.CreateContext(context.Background(), d, &S3MinioClient{S3Admin: admin})
			if !diags.HasError() {
				t.Fatal("create succeeded, want the set-config-kv error")
			}
			if sets != 1 {
				t.Errorf("set-config-kv calls = %d, want 1", sets)
			}
			if d.Id() != "primary" {
				t.Errorf("id after a failed write = %q, want %q so the failed create is saved as tainted", d.Id(), "primary")
			}
		})
	}
}
