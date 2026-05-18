package minio

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v3"
)

func TestAccMinioPoolDecommission_basic(t *testing.T) {
	if os.Getenv("RUN_POOL_DECOMMISSION_ACC") != "1" {
		t.Skip("skipping pool decommission test; set RUN_POOL_DECOMMISSION_ACC=1 to enable")
	}

	resourceName := "minio_pool_decommission.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
		},
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPoolDecommissionConfig(0),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "started_at"),
					resource.TestCheckResourceAttrSet(resourceName, "state"),
					resource.TestCheckResourceAttrSet(resourceName, "status"),
					testCheckPoolDecommissionStatusIsValidJSON(resourceName),
				),
			},
		},
	})
}

func testCheckPoolDecommissionStatusIsValidJSON(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}
		raw := rs.Primary.Attributes["status"]
		if raw == "" {
			return nil
		}
		var info madmin.PoolDecommissionInfo
		if err := json.Unmarshal([]byte(raw), &info); err != nil {
			return fmt.Errorf("status is not a valid PoolDecommissionInfo JSON document: %w (raw: %q)", err, raw)
		}
		return nil
	}
}

func testAccMinioPoolDecommissionConfig(poolIndex int) string {
	return fmt.Sprintf(`
resource "minio_pool_decommission" "test" {
  pool_index = %d
}
`, poolIndex)
}

func TestDecommissionState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		info *madmin.PoolDecommissionInfo
		want string
	}{
		{name: "nil_means_decommissioning", info: nil, want: poolStateDecommissioning},
		{name: "in_progress", info: &madmin.PoolDecommissionInfo{}, want: poolStateDecommissioning},
		{name: "complete", info: &madmin.PoolDecommissionInfo{Complete: true}, want: poolStateDecommissioned},
		{name: "canceled", info: &madmin.PoolDecommissionInfo{Canceled: true}, want: poolStateCanceled},
		{name: "failed", info: &madmin.PoolDecommissionInfo{Failed: true}, want: poolStateFailed},
		{name: "canceled_wins_over_failed", info: &madmin.PoolDecommissionInfo{Canceled: true, Failed: true}, want: poolStateCanceled},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := decommissionState(tc.info); got != tc.want {
				t.Fatalf("decommissionState(%+v) = %q, want %q", tc.info, got, tc.want)
			}
		})
	}
}

func TestIsDecommissionCancelError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "unrelated", err: fmt.Errorf("boom"), want: false},
		{name: "not_in_progress", err: fmt.Errorf("decommission is Not In Progress"), want: true},
		{name: "already_complete", err: fmt.Errorf("decommission already complete"), want: true},
		{name: "canceled", err: fmt.Errorf("decommission was canceled"), want: true},
		{name: "cancelled_british", err: fmt.Errorf("already cancelled"), want: true},
		{name: "pool_not_found", err: fmt.Errorf("pool not found"), want: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isDecommissionCancelError(tc.err); got != tc.want {
				t.Fatalf("isDecommissionCancelError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
