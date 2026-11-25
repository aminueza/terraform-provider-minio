package minio

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TestBuildNotifyKafkaCfgCompressionFields shows that the compression and
// batch commit keys reach the built config only when the configuration sets
// them. MinIO accepts a key its subsystem does not have and glues it onto the
// previous value, so an older server must never receive them unasked.
// See issue #1369.
func TestBuildNotifyKafkaCfgCompressionFields(t *testing.T) {
	s := resourceMinioNotifyKafka().Schema

	unset := buildNotifyKafkaCfg(schema.TestResourceDataRaw(t, s, map[string]interface{}{
		"name": "n", "brokers": "localhost:9092", "topic": "events",
	}), nil)
	for _, bad := range []string{"compression_codec=", "compression_level=", "batch_commit_timeout="} {
		if strings.Contains(unset, bad) {
			t.Errorf("config %q sends %q, a field the configuration does not set", unset, bad)
		}
	}

	set := buildNotifyKafkaCfg(schema.TestResourceDataRaw(t, s, map[string]interface{}{
		"name":                 "n",
		"brokers":              "localhost:9092",
		"topic":                "events",
		"queue_dir":            "/tmp/minio-queue",
		"batch_size":           10,
		"compression_codec":    "gzip",
		"compression_level":    6,
		"batch_commit_timeout": "10s",
	}), nil)
	for _, want := range []string{"compression_codec=gzip", "compression_level=6", "batch_commit_timeout=10s"} {
		if !strings.Contains(set, want) {
			t.Errorf("config %q does not contain %q", set, want)
		}
	}

	// MinIO passes compression_level straight to the sarama producer, where
	// a negative level is meaningful: for gzip only -1 means the library
	// default.
	negative := buildNotifyKafkaCfg(schema.TestResourceDataRaw(t, s, map[string]interface{}{
		"name":              "n",
		"brokers":           "localhost:9092",
		"topic":             "events",
		"compression_codec": "gzip",
		"compression_level": -1,
	}), nil)
	if !strings.Contains(negative, "compression_level=-1") {
		t.Errorf("config %q drops compression_level=-1, a level sarama accepts", negative)
	}
}

func TestReadNotifyKafkaFieldsCompression(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceMinioNotifyKafka().Schema, map[string]interface{}{
		"name": "n", "brokers": "localhost:9092", "topic": "events",
	})

	if diags := readNotifyKafkaFields(map[string]string{
		"brokers": "localhost:9092", "topic": "events",
		"compression_codec": "gzip", "compression_level": "6", "batch_commit_timeout": "10s",
	}, d); diags.HasError() {
		t.Fatalf("read failed: %v", diags)
	}

	if got := d.Get("compression_codec").(string); got != "gzip" {
		t.Errorf("compression_codec = %q, want gzip", got)
	}
	if got := d.Get("compression_level").(int); got != 6 {
		t.Errorf("compression_level = %d, want 6", got)
	}
	if got := d.Get("batch_commit_timeout").(string); got != "10s" {
		t.Errorf("batch_commit_timeout = %q, want 10s", got)
	}

	// A level cleared on the server is printed empty, and notifySetInt skips
	// values it cannot parse: the read must still bring the state back to 0.
	cleared := schema.TestResourceDataRaw(t, resourceMinioNotifyKafka().Schema, map[string]interface{}{
		"name": "n", "brokers": "localhost:9092", "topic": "events", "compression_level": 6,
	})
	if diags := readNotifyKafkaFields(map[string]string{
		"brokers": "localhost:9092", "topic": "events",
		"compression_codec": "gzip", "compression_level": "",
	}, cleared); diags.HasError() {
		t.Fatalf("read of a cleared level failed: %v", diags)
	}
	if got := cleared.Get("compression_level").(int); got != 0 {
		t.Errorf("compression_level = %d after an empty server value, want 0", got)
	}
}

// The fields are set, read back, imported, changed outside Terraform, and
// finally removed: a removal must write MinIO's documented defaults (empty
// codec, level 0, 0s) back, not an empty value, because an empty duration
// fails to parse once the target is enabled. The target stays disabled, so no
// broker is needed. Sequential on purpose: the server checks restart the
// primary test server. See issue #1369.
func TestAccMinioNotifyKafka_compressionAndBatchCommit(t *testing.T) {
	resourceName := "minio_notify_kafka.test"
	dataSourceName := "data.minio_notify_kafka.test"
	name := "tfacc-" + acctest.RandString(6)
	config := func(fields string) string {
		return testAccMinioNotifyDisabledTarget("kafka", name, `  brokers   = "localhost:9092"
  topic     = "events"
  queue_dir = "/tmp/minio-queue"
  batch_size = 10
`+fields)
	}
	withFields := config(`  compression_codec    = "gzip"
  compression_level    = 6
  batch_commit_timeout = "10s"`)
	withDataSource := withFields + `
data "minio_notify_kafka" "test" {
  name = minio_notify_kafka.test.name
}
`
	withoutFields := config("")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("notify_kafka", name),
		Steps: []resource.TestStep{
			{
				Config: withFields,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "compression_codec", "gzip"),
					resource.TestCheckResourceAttr(resourceName, "compression_level", "6"),
					resource.TestCheckResourceAttr(resourceName, "batch_commit_timeout", "10s"),
					resource.TestCheckResourceAttr(resourceName, "restart_required", "true"),
					testAccCheckServerStoredTarget("notify_kafka", name, map[string]string{
						"compression_codec":    "gzip",
						"compression_level":    "6",
						"batch_commit_timeout": "10s",
					}),
				),
			},
			{Config: withFields, PlanOnly: true},
			{
				Config: withDataSource,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "compression_codec", "gzip"),
					resource.TestCheckResourceAttr(dataSourceName, "compression_level", "6"),
					resource.TestCheckResourceAttr(dataSourceName, "batch_commit_timeout", "10s"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"restart_required"},
			},
			{
				PreConfig: func() {
					changed := notifyConfigKey("notify_kafka", name) + " compression_codec=zstd enable=off"
					if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), changed); err != nil {
						t.Fatalf("changing compression_codec of %s outside Terraform: %v", name, err)
					}
					if err := testAccRestartPrimaryServer(); err != nil {
						t.Fatalf("restarting the server: %v", err)
					}
				},
				Config:             withFields,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: withFields,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "compression_codec", "gzip"),
					testAccCheckServerStoredTarget("notify_kafka", name, map[string]string{"compression_codec": "gzip"}),
				),
			},
			{
				// Removing the fields writes MinIO's documented defaults back, not
				// an empty value that fails to parse once the target is enabled.
				Config: withoutFields,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "compression_codec", ""),
					resource.TestCheckResourceAttr(resourceName, "compression_level", "0"),
					resource.TestCheckResourceAttr(resourceName, "batch_commit_timeout", "0s"),
					testAccCheckServerStoredTarget("notify_kafka", name, map[string]string{
						"compression_codec":    "",
						"compression_level":    "",
						"batch_commit_timeout": "0s",
					}),
				),
			},
			{Config: withoutFields, PlanOnly: true},
		},
	})
}

func TestAccMinioNotifyKafka_compressionPlanChecks(t *testing.T) {
	cases := []struct {
		name  string
		extra string
		want  string
	}{
		{
			name:  "batch_commit_timeout without queue_dir",
			extra: "  batch_commit_timeout = \"10s\"\n",
			want:  `batch_commit_timeout has no effect without queue_dir`,
		},
		{
			name: "batch_commit_timeout with batch_size of 1",
			extra: "  queue_dir            = \"/tmp/minio-queue\"\n  batch_size           = 1\n" +
				"  batch_commit_timeout = \"10s\"\n",
			want: `batch_commit_timeout has no effect with batch_size of 1 or less`,
		},
		{
			name:  "compression_level without compression_codec",
			extra: "  compression_level    = 6\n",
			want:  `compression_codec`,
		},
		{
			name:  "unknown compression_codec",
			extra: "  compression_codec    = \"deflate\"\n",
			want:  `expected compression_codec to be one of`,
		},
		{
			name:  "unparseable batch_commit_timeout",
			extra: "  queue_dir            = \"/tmp/minio-queue\"\n  batch_size           = 10\n  batch_commit_timeout = \"ten\"\n",
			want:  `must be a non-negative golang duration`,
		},
		{
			// A create that leaves batch_size out sends nothing and MinIO
			// stores 0: no batching, so the timeout would never fire. The
			// diff cannot see the 0 yet, only the raw config can.
			name: "batch_commit_timeout without batch_size",
			extra: "  queue_dir            = \"/tmp/minio-queue\"\n" +
				"  batch_commit_timeout = \"10s\"\n",
			want: `batch_commit_timeout has no effect with batch_size of 1 or less`,
		},
		{
			// An empty duration is a silent no-op: MinIO refuses to parse
			// it once the target is enabled.
			name:  "empty batch_commit_timeout",
			extra: "  queue_dir            = \"/tmp/minio-queue\"\n  batch_size           = 10\n  batch_commit_timeout = \"\"\n",
			want:  `must be a non-negative golang duration`,
		},
		{
			// sarama hands the level to gzip.NewWriterLevel, which only
			// accepts -2..9; MinIO surfaces that as an apply failure.
			name:  "gzip compression_level above the range",
			extra: "  compression_codec    = \"gzip\"\n  compression_level    = 42\n",
			want:  `is not valid for gzip`,
		},
		{
			name:  "gzip compression_level below the range",
			extra: "  compression_codec    = \"gzip\"\n  compression_level    = -50\n",
			want:  `is not valid for gzip`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:          func() { testAccPreCheck(t) },
				ProviderFactories: testAccProviders,
				Steps: []resource.TestStep{
					{
						Config: testAccMinioNotifyDisabledTarget("kafka", "tfacc-"+acctest.RandString(6),
							fmt.Sprintf("  brokers = \"localhost:9092\"\n  topic   = \"events\"\n%s", tc.extra)),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(tc.want),
					},
				},
			})
		})
	}
}

// A queue_dir taken from another resource is unknown at plan time and reads
// as empty: the batch_commit_timeout check must not reject a valid
// configuration over a value the plan cannot see yet. See issue #1369.
func TestAccMinioNotifyKafka_batchCommitWithUnknownQueueDir(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	config := testAccMinioNotifyDisabledTarget("kafka", name, `  brokers              = "localhost:9092"
  topic                = "events"
  queue_dir            = "/tmp/${minio_notify_kafka.dir.id}"
  batch_size           = 10
  batch_commit_timeout = "10s"`) + `

resource "minio_notify_kafka" "dir" {
  name    = "` + name + `-dir"
  enable  = false
  brokers = "localhost:9092"
  topic   = "events"
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy: resource.ComposeTestCheckFunc(
			testAccCheckServerRemovedTarget("notify_kafka", name),
			testAccCheckServerRemovedTarget("notify_kafka", name+"-dir"),
		),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_notify_kafka.test", "batch_commit_timeout", "10s"),
					resource.TestCheckResourceAttrWith("minio_notify_kafka.test", "queue_dir", func(v string) error {
						if !strings.HasPrefix(v, "/tmp/"+name+"-dir") {
							return fmt.Errorf("queue_dir = %q, want an absolute path built from the other target's id", v)
						}
						return nil
					}),
				),
			},
		},
	})
}

// MinIO only checks the timeout when it is above 0, so "0s" must not demand
// queue_dir or batch_size: a module that passes "0s" by default must be able
// to create a target.
func TestAccMinioNotifyKafka_batchCommitZeroWithoutQueueDir(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	config := testAccMinioNotifyDisabledTarget("kafka", name, `  brokers              = "localhost:9092"
  topic                = "events"
  batch_commit_timeout = "0s"`)
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("notify_kafka", name),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_notify_kafka.test", "batch_commit_timeout", "0s"),
					testAccCheckServerStoredTarget("notify_kafka", name, map[string]string{"batch_commit_timeout": "0s"}),
				),
			},
			{Config: config, PlanOnly: true},
		},
	})
}
