package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestAccDataSourceMinioConfigHistory_basic(t *testing.T) {
	resourceName := "data.minio_config_history.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccConfigHistoryConfig_basic(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "entries.#"),
				),
			},
		},
	})
}

func testAccConfigHistoryConfig_basic() string {
	return `
data "minio_config_history" "test" {}
`
}

func TestDataSourceConfigHistory_unsupportedServerIsBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"Code":"XMinioAdminNotSupported","Message":"This 'admin' API is not supported by server in 'mode-server-xl'"}`)
	}))
	defer srv.Close()

	adminClient, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}

	d := schema.TestResourceDataRaw(t, dataSourceMinioConfigHistory().Schema, map[string]interface{}{})
	start := time.Now()
	diags := dataSourceMinioConfigHistoryRead(context.Background(), d, &S3MinioClient{S3Admin: adminClient})
	elapsed := time.Since(start)

	if elapsed > 30*time.Second {
		t.Fatalf("read took %s; the unsupported-API deadline is not bounding the retry loop", elapsed)
	}
	if len(diags) != 1 || diags[0].Severity != diag.Warning {
		t.Fatalf("expected exactly one warning diagnostic, got %+v", diags)
	}
	if d.Id() != "config_history" {
		t.Errorf("expected id %q, got %q", "config_history", d.Id())
	}
}
