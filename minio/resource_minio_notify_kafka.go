package minio

import (
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceMinioNotifyKafka() *schema.Resource {
	nrc := notifyResourceConfig{
		subsystem:  "notify_kafka",
		buildCfg:   buildNotifyKafkaCfg,
		readFields: readNotifyKafkaFields,
	}

	return &schema.Resource{
		Description:   "Manages a Kafka notification target for MinIO bucket event notifications.",
		CreateContext: notifyCreate(nrc),
		ReadContext:   notifyRead(nrc),
		UpdateContext: notifyUpdate(nrc),
		DeleteContext: notifyDelete(nrc),
		CustomizeDiff: customizeDiffKafka,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: mergeSchemas(notifyCommonSchema(), map[string]*schema.Schema{
			"brokers": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Comma-separated list of Kafka broker addresses (e.g., 'host1:9092,host2:9092').",
			},
			"topic": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Kafka topic to publish event notifications to.",
			},
			"sasl": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to enable SASL authentication for the Kafka connection.",
			},
			"sasl_username": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "SASL username for Kafka authentication. Only used when `sasl` is `true`.",
			},
			"sasl_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "SASL password for Kafka authentication. Only used when `sasl` is `true`. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"sasl_mechanism": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "SASL authentication mechanism: 'plain', 'scram-sha-256', or 'scram-sha-512'. Only used when `sasl` is `true`.",
			},
			"tls": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to enable TLS for the Kafka connection.",
			},
			"tls_skip_verify": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether to skip TLS certificate verification.",
			},
			"tls_client_auth": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "TLS client authentication type (0=NoClientCert, 1=RequestClientCert, etc.).",
			},
			// RequiredWith keeps a cert-only configuration out of the plan: the
			// readback would put the server's key path into state and the empty
			// config value would then clear it, which MinIO rejects as a broken
			// pair. The key alone stays allowed: removing the certificate while
			// the key remains is how the pair gets cleared.
			"client_tls_cert": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"client_tls_key"},
				Description:  "Path to the client TLS certificate for mTLS authentication.",
			},
			"client_tls_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "Path to the client TLS private key.",
			},
			"version": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Kafka cluster version (e.g., '2.8.0').",
			},
			"batch_size": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "Number of messages to batch before sending to Kafka.",
			},
			"compression_codec": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validation.StringInSlice([]string{"none", "snappy", "gzip", "lz4", "zstd"}, false),
				Description:  "Producer compression codec: `none`, `snappy`, `gzip`, `lz4` or `zstd`. First available in MinIO RELEASE.2023-12-02T10-51-33Z. Set `compression_level` with it: a codec without a level means level 0, which for gzip is no compression.",
			},
			"compression_level": {
				Type:         schema.TypeInt,
				Optional:     true,
				RequiredWith: []string{"compression_codec"},
				Description:  "Compression level for `compression_codec`. MinIO turns a value it cannot parse into 0. First available in MinIO RELEASE.2023-12-02T10-51-33Z.",
			},
			"batch_commit_timeout": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					v := val.(string)
					if dur, err := time.ParseDuration(v); err != nil || dur < 0 {
						errs = append(errs, fmt.Errorf("%q must be a non-negative golang duration like '10s' or '0s', got %q", key, v))
					}
					return
				},
				Description: "How long a partly filled batch waits before it is committed; `0s` means 30s. Only used when `queue_dir` is set and `batch_size` is above 1; the plan rejects a value above `0s` without `queue_dir` or with `batch_size` of 1 or less. First available in MinIO RELEASE.2024-09-09T16-59-28Z.",
			},
		}),
	}
}

func buildNotifyKafkaCfg(d *schema.ResourceData, meta interface{}) string {
	var parts []string

	notifyBuildCfgAddParam(&parts, "brokers", d.Get("brokers").(string))
	notifyBuildCfgAddParam(&parts, "topic", d.Get("topic").(string))
	notifyBuildCfgAddBool(&parts, "sasl", d.Get("sasl").(bool))
	notifyBuildCfgClearablePair(&parts, d, "sasl_username", "sasl_password")
	notifyBuildCfgAddParam(&parts, "sasl_mechanism", getOptionalField(d, "sasl_mechanism", "").(string))
	notifyBuildCfgAddBool(&parts, "tls", getOptionalField(d, "tls", false).(bool))
	notifyBuildCfgAddBool(&parts, "tls_skip_verify", getOptionalField(d, "tls_skip_verify", false).(bool))
	notifyBuildCfgAddInt(&parts, "tls_client_auth", getOptionalField(d, "tls_client_auth", 0).(int))
	notifyBuildCfgClearablePair(&parts, d, "client_tls_cert", "client_tls_key")
	notifyBuildCfgClearableParam(&parts, d, "version")
	notifyBuildCfgAddInt(&parts, "batch_size", getOptionalField(d, "batch_size", 0).(int))
	notifyBuildCfgClearableParam(&parts, d, "compression_codec")
	notifyBuildCfgClearableIntParam(&parts, d, "compression_level")
	notifyBuildCfgClearableComputedString(&parts, d, "batch_commit_timeout", "0s")

	notifyBuildCommonCfg(&parts, d, meta)
	return strings.Join(parts, " ")
}

func readNotifyKafkaFields(cfgMap map[string]string, d *schema.ResourceData) diag.Diagnostics {
	if diags := notifySetPresentStrings(d, cfgMap, "brokers", "topic", "sasl_username", "sasl_mechanism"); diags != nil {
		return diags
	}
	if diags := notifySetOnBools(d, cfgMap, "sasl", "tls", "tls_skip_verify"); diags != nil {
		return diags
	}
	if diags := notifySetInt(d, cfgMap, "tls_client_auth", "tls_client_auth"); diags != nil {
		return diags
	}
	if diags := notifySetPresentStrings(d, cfgMap, "client_tls_cert", "client_tls_key", "version"); diags != nil {
		return diags
	}
	if diags := notifySetInt(d, cfgMap, "batch_size", "batch_size"); diags != nil {
		return diags
	}
	if diags := notifySetPresentStrings(d, cfgMap, "compression_codec", "batch_commit_timeout"); diags != nil {
		return diags
	}
	if v, ok := cfgMap["compression_level"]; ok && v == "" {
		// A cleared level is printed empty and MinIO parses it as 0;
		// notifySetInt would skip it and leave a stale level in state.
		if diags := setResourceField(d, "compression_level", 0, d.Id()); diags != nil {
			return diags
		}
	} else if diags := notifySetInt(d, cfgMap, "compression_level", "compression_level"); diags != nil {
		return diags
	}
	return notifyReadCommonFields(cfgMap, d)
}
