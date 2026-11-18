package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// testAccRestartPrimaryServer restarts the primary test server and waits
// until it is back. get-config-kv only serves a created or changed target
// after a restart, so the server checks below need it. It must only run in
// sequential tests: the restart takes the shared server down for a few
// seconds, and parallel tests must not run against it.
func testAccRestartPrimaryServer() error {
	client := testAccClient()
	return testAccOIDCRestartMinio(context.Background(), client.S3Admin)
}

// testAccCheckServerStoredTarget restarts the primary test server and checks
// that get-config-kv reports the stored target with the given key=value
// pairs. The values in want are explicit literals from the test's own
// configuration: they assert what the server persisted, not what the
// provider would have written. MinIO prints a target's keys even when they
// are empty (auth_token=, queue_dir=) and omits enable=on, so the map
// carries the keys the server is expected to print and a cleared key is
// checked against "".
func testAccCheckServerStoredTarget(subsystem, name string, want map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := testAccRestartPrimaryServer(); err != nil {
			return err
		}

		client := testAccClient()
		ctx := context.Background()
		target := notifyConfigKey(subsystem, name)
		data, err := client.S3Admin.GetConfigKV(ctx, target)
		if err != nil {
			return fmt.Errorf("reading %s from the server after the restart: %w", target, err)
		}
		stored, found := notifyTargetConfigParams(string(data), target)
		if !found {
			return fmt.Errorf("%s is not stored on the server after the restart; response: %s", target, string(data))
		}
		for key, value := range want {
			got, ok := stored[key]
			if !ok {
				return fmt.Errorf("%s stored on the server has no %q key; response: %s", target, key, string(data))
			}
			if got != value {
				return fmt.Errorf("%q of %s stored on the server is %q, want %q", key, target, got, value)
			}
		}
		return nil
	}
}

// testAccCheckServerRemovedTarget restarts the primary test server and
// checks that get-config-kv no longer reports the target. A removal, like a
// change, is only visible after the restart.
func testAccCheckServerRemovedTarget(subsystem, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := testAccRestartPrimaryServer(); err != nil {
			return err
		}

		client := testAccClient()
		target := notifyConfigKey(subsystem, name)
		data, err := client.S3Admin.GetConfigKV(context.Background(), target)
		if err == nil {
			if _, found := notifyTargetConfigParams(string(data), target); !found {
				return nil
			}
			return fmt.Errorf("%s is still stored on the server after destroy: %s", target, string(data))
		}
		if notifyConfigTargetMissing(err) {
			return nil
		}
		return fmt.Errorf("checking that %s is gone from the server: %w", target, err)
	}
}

// notifyConfigTargetMissing reports whether a get-config-kv error says the
// target does not exist, the phrase the server returns after a target is
// deleted and the server has restarted.
func notifyConfigTargetMissing(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "there is no target")
}
