package minio

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// testAccRestartPrimaryServer restarts the primary MinIO test server and
// waits until it answers again.
//
// get-config-kv only serves a created or changed target after a restart, so
// a check straight after apply reads the pre-restart config (issue #1299).
// The restart takes the shared server down for a few seconds, so the helper
// must only run inside sequential tests: a top-level test that calls
// t.Parallel starts only after every sequential top-level test in the
// package has finished, and CI runs the suite with the default -parallel
// (no flag in docker-compose.yml), so the restart cannot overlap a test
// that is still talking to the server.
func testAccRestartPrimaryServer() error {
	client := testAccClient()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := client.S3Admin.ServiceRestart(ctx); err != nil {
		return fmt.Errorf("restarting the primary test server: %w", err)
	}
	for {
		_, err := client.S3Client.ListBuckets(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("the primary test server did not answer after the restart: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// testAccCheckServerStoredTarget restarts the primary test server and checks
// that get-config-kv reports the stored target with the given key=value
// pairs. The values in want are explicit literals from the test's own
// configuration: they assert what the server persisted, not what the
// provider would have written. MinIO omits empty values and enable=on from
// a stored line, so the map only carries keys the server is expected to
// print.
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
// change, is only visible after the restart, so the check polls until the
// server either answers or the deadline is hit.
func testAccCheckServerRemovedTarget(subsystem, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := testAccRestartPrimaryServer(); err != nil {
			return err
		}

		client := testAccClient()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		target := notifyConfigKey(subsystem, name)
		for {
			data, err := client.S3Admin.GetConfigKV(ctx, target)
			if err == nil {
				if _, found := notifyTargetConfigParams(string(data), target); !found {
					return nil
				}
				return fmt.Errorf("%s is still stored on the server after destroy: %s", target, string(data))
			}
			if notifyConfigTargetMissing(err) {
				return nil
			}
			if ctx.Err() != nil {
				return fmt.Errorf("checking that %s is gone from the server: %w", target, err)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// notifyConfigTargetMissing reports whether a get-config-kv error says the
// target does not exist, the same phrases the resource's read and delete
// handle in resource_minio_notify_common.go.
func notifyConfigTargetMissing(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "there is no target") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "does not exist")
}

func TestNotifyTargetConfigParams(t *testing.T) {
	tests := []struct {
		name     string
		response string
		key      string
		want     map[string]string
		wantOK   bool
	}{
		{
			name:     "bare single line",
			response: "endpoint=http://example.com enable=off",
			key:      "notify_webhook:x",
			want:     map[string]string{"endpoint": "http://example.com", "enable": "off"},
			wantOK:   true,
		},
		{
			name:     "named line among other targets",
			response: "notify_webhook:x endpoint=http://x enable=off\nnotify_webhook:y endpoint=http://y",
			key:      "notify_webhook:x",
			want:     map[string]string{"endpoint": "http://x", "enable": "off"},
			wantOK:   true,
		},
		{
			name:     "key with no value",
			response: "notify_webhook:x",
			key:      "notify_webhook:x",
			want:     map[string]string{},
			wantOK:   true,
		},
		{
			name:     "several lines without the target",
			response: "notify_webhook:y endpoint=http://y\nnotify_webhook:z endpoint=http://z",
			key:      "notify_webhook:x",
			want:     map[string]string{},
			wantOK:   false,
		},
		{
			name:     "quoted value with a space",
			response: `comment="primary webhook" enable=off`,
			key:      "notify_webhook:x",
			want:     map[string]string{"comment": "primary webhook", "enable": "off"},
			wantOK:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := notifyTargetConfigParams(tc.response, tc.key)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("params = %v, want %v", got, tc.want)
			}
		})
	}
}
