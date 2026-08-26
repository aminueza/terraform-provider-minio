package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestAccDataSourceMinioLicenseInfo_basic(t *testing.T) {
	dataSourceName := "data.minio_license_info.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `data "minio_license_info" "test" {}`,
				Check: resource.ComposeTestCheckFunc(
					// The test fixture runs community MinIO, which has no
					// license subsystem, so the read reports the unlicensed
					// fallback instead of failing.
					resource.TestCheckResourceAttr(dataSourceName, "id", "unlicensed"),
					resource.TestCheckResourceAttr(dataSourceName, "plan", ""),
				),
			},
		},
	})
}

func TestDataSourceLicenseInfo_unsupportedServerIsBounded(t *testing.T) {
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

	d := schema.TestResourceDataRaw(t, dataSourceMinioLicenseInfo().Schema, map[string]interface{}{})
	start := time.Now()
	diags := dataSourceMinioLicenseInfoRead(context.Background(), d, &S3MinioClient{S3Admin: adminClient})
	elapsed := time.Since(start)

	if elapsed > 30*time.Second {
		t.Fatalf("read took %s; the unsupported-API deadline is not bounding the retry loop", elapsed)
	}
	if len(diags) != 0 {
		t.Fatalf("expected the unlicensed fallback to succeed, got %+v", diags)
	}
	if d.Id() != "unlicensed" {
		t.Errorf("expected id %q, got %q", "unlicensed", d.Id())
	}
}
