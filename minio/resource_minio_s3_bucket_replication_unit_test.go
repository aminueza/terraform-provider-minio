package minio

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
)

func TestSuppressBandwidthLimitDiffComparesBytesNotRenderings(t *testing.T) {
	cases := []struct {
		name     string
		state    string
		config   string
		suppress bool
	}{
		{name: "exact bytes in state match a decimal suffix", state: "10500000000", config: "10.5GB", suppress: true},
		{name: "exact bytes in state match a short suffix", state: "100000000", config: "100M", suppress: true},
		{name: "zero in state matches the default", state: "0", config: "0", suppress: true},
		{name: "a small change is not hidden by a shared rendering", state: "1000000000", config: "1040MB", suppress: false},
		{name: "a state rendered by the old provider does not match a value it rounded away", state: "10 GB", config: "10.5GB", suppress: false},
		{name: "a state rendered by the old provider matches the value it stands for", state: "10 GB", config: "10000MB", suppress: true},
		{name: "an unparseable config is never suppressed", state: "100000000", config: "lots", suppress: false},
		{name: "an unparseable state is never suppressed", state: "", config: "100M", suppress: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := suppressBandwidthLimitDiff("bandwidth_limit", tc.state, tc.config, nil); got != tc.suppress {
				t.Errorf("suppress(%q, %q) = %v, want %v", tc.state, tc.config, got, tc.suppress)
			}
		})
	}
}

func TestFindRemoteTargetMatchesEndpointAndTargetBucket(t *testing.T) {
	targets := []madmin.BucketTarget{
		{Arn: "arn:minio:replication::b:bucket-b", Endpoint: "minio-b:9000", TargetBucket: "bucket-b"},
		{Arn: "arn:minio:replication::c:bucket-c", Endpoint: "MinIO-C:9000", TargetBucket: "prefix/bucket-c"},
	}

	cases := []struct {
		name         string
		endpoint     string
		targetBucket string
		wantArn      string
	}{
		{name: "same endpoint and bucket", endpoint: "minio-b:9000", targetBucket: "bucket-b", wantArn: "arn:minio:replication::b:bucket-b"},
		{name: "endpoint differing only in case", endpoint: "minio-c:9000", targetBucket: "prefix/bucket-c", wantArn: "arn:minio:replication::c:bucket-c"},
		{name: "same endpoint, other bucket", endpoint: "minio-b:9000", targetBucket: "bucket-c", wantArn: ""},
		{name: "bucket without its path", endpoint: "minio-c:9000", targetBucket: "bucket-c", wantArn: ""},
		{name: "unknown endpoint", endpoint: "minio-d:9000", targetBucket: "bucket-b", wantArn: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findRemoteTarget(targets, tc.endpoint, tc.targetBucket)
			gotArn := ""
			if got != nil {
				gotArn = got.Arn
			}
			if gotArn != tc.wantArn {
				t.Errorf("findRemoteTarget(%q, %q) = %q, want %q", tc.endpoint, tc.targetBucket, gotArn, tc.wantArn)
			}
		})
	}
}

func TestValidateReplicationRuleTargetsRejectsSharedTargets(t *testing.T) {
	rule := func(host string, bucket string, path string) S3MinioBucketReplicationRule {
		return S3MinioBucketReplicationRule{Target: S3MinioBucketReplicationRuleTarget{Host: host, Bucket: bucket, Path: path}}
	}

	cases := []struct {
		name    string
		rules   []S3MinioBucketReplicationRule
		wantErr string
	}{
		{name: "distinct endpoints", rules: []S3MinioBucketReplicationRule{rule("minio-b:9000", "bucket", ""), rule("minio-c:9000", "bucket", "")}},
		{name: "same endpoint, distinct buckets", rules: []S3MinioBucketReplicationRule{rule("minio-b:9000", "bucket-1", ""), rule("minio-b:9000", "bucket-2", "")}, wantErr: `rule[0] and rule[1] both replicate to "minio-b:9000"`},
		{name: "same endpoint, same bucket, distinct paths", rules: []S3MinioBucketReplicationRule{rule("minio-b:9000", "bucket", "a"), rule("minio-b:9000", "bucket", "b")}, wantErr: `rule[0] and rule[1] both replicate to "minio-b:9000"`},
		{name: "same endpoint and bucket", rules: []S3MinioBucketReplicationRule{rule("minio-b:9000", "bucket", ""), rule("minio-c:9000", "other", ""), rule("minio-b:9000", "bucket", "")}, wantErr: `rule[0] and rule[2] both replicate to "minio-b:9000"`},
		{name: "endpoint differing only in case", rules: []S3MinioBucketReplicationRule{rule("minio-b:9000", "bucket", ""), rule("MINIO-B:9000", "other", "")}, wantErr: `rule[0] and rule[1] both replicate to "MINIO-B:9000"`},
		{name: "invalid bucket name", rules: []S3MinioBucketReplicationRule{rule("minio-b:9000", "bucket", ""), rule("minio-c:9000", "ab", "")}, wantErr: "rule[1].target.bucket: "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateReplicationRuleTargets(tc.rules)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("expected an error containing %q, got none", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("expected an error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestSplitReferencedRemoteTargetsKeepsOnlyTargetsOfRules(t *testing.T) {
	targets := []madmin.BucketTarget{
		{Arn: "arn:minio:replication::b:bucket-b"},
		{Arn: "arn:minio:replication::stale:bucket-x"},
		{Arn: "arn:minio:replication::d:bucket-d"},
	}
	ruleArnMap := map[string]int{
		"arn:minio:replication::b:bucket-b": 0,
		"arn:minio:replication::d:bucket-d": 1,
	}

	referenced, unreferenced := splitReferencedRemoteTargets(targets, ruleArnMap)

	var referencedARNs []string
	for _, target := range referenced {
		referencedARNs = append(referencedARNs, target.Arn)
	}
	if want := []string{"arn:minio:replication::b:bucket-b", "arn:minio:replication::d:bucket-d"}; !slices.Equal(referencedARNs, want) {
		t.Errorf("referenced = %v, want %v", referencedARNs, want)
	}
	if want := []string{"arn:minio:replication::stale:bucket-x"}; !slices.Equal(unreferenced, want) {
		t.Errorf("unreferenced = %v, want %v", unreferenced, want)
	}
}

func replicationValidationConfig(target map[string]interface{}) *terraform.ResourceConfig {
	return terraform.NewResourceConfigRaw(map[string]interface{}{
		"bucket": "source",
		"rule": []interface{}{
			map[string]interface{}{
				"priority": 1,
				"target":   []interface{}{target},
			},
		},
	})
}

func TestValidateReplicationHealthCheckPeriod(t *testing.T) {
	cases := []struct {
		value   string
		wantErr bool
	}{
		{value: "5|", wantErr: true},
		{value: "5 s", wantErr: true},
		{value: "9999999999h", wantErr: true},
		{value: "0s", wantErr: true},
		{value: "", wantErr: true},
		{value: "abc", wantErr: true},
		{value: "5s", wantErr: false},
		{value: "90s", wantErr: false},
		{value: "2m", wantErr: false},
		{value: "1m30s", wantErr: false},
		{value: "1h30m", wantErr: false},
	}

	res := resourceMinioBucketReplication()
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%q", tc.value), func(t *testing.T) {
			diags := validateReplicationHealthCheckPeriod(tc.value, cty.Path{})
			if diags.HasError() != tc.wantErr {
				t.Fatalf("validator(%q) HasError = %v, want %v (%v)", tc.value, diags.HasError(), tc.wantErr, diags)
			}

			cfgDiags := res.Validate(replicationValidationConfig(map[string]interface{}{
				"bucket":              "dest",
				"host":                "minio-b:9000",
				"access_key":          "replication",
				"secret_key":          "replication-secret",
				"health_check_period": tc.value,
			}))
			if cfgDiags.HasError() != tc.wantErr {
				t.Fatalf("res.Validate(%q) HasError = %v, want %v (%v)", tc.value, cfgDiags.HasError(), tc.wantErr, cfgDiags)
			}
		})
	}
}

func TestValidateReplicationLegacyBandwidthLimit(t *testing.T) {
	cases := []struct {
		value   string
		wantErr bool
	}{
		{value: "abc", wantErr: true},
		{value: "1k", wantErr: true},
		{value: "200M", wantErr: false},
	}

	res := resourceMinioBucketReplication()
	validator := validateReplicationBandwidthLimitField("bandwidth_limt")
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			diags := validator(tc.value, cty.Path{})
			if diags.HasError() != tc.wantErr {
				t.Fatalf("validator(%q) HasError = %v, want %v (%v)", tc.value, diags.HasError(), tc.wantErr, diags)
			}
			if tc.wantErr && !strings.Contains(fmt.Sprint(diags), "bandwidth_limt") {
				t.Fatalf("validator(%q) error = %v, want error naming bandwidth_limt", tc.value, diags)
			}

			cfgDiags := res.Validate(replicationValidationConfig(map[string]interface{}{
				"bucket":         "dest",
				"host":           "minio-b:9000",
				"access_key":     "replication",
				"secret_key":     "replication-secret",
				"bandwidth_limt": tc.value,
			}))
			if cfgDiags.HasError() != tc.wantErr {
				t.Fatalf("res.Validate(%q) HasError = %v, want %v (%v)", tc.value, cfgDiags.HasError(), tc.wantErr, cfgDiags)
			}
		})
	}
}

func TestValidateReplicationTargetSecureWarning(t *testing.T) {
	res := resourceMinioBucketReplication()

	diags := res.Validate(replicationValidationConfig(map[string]interface{}{
		"bucket":     "dest",
		"host":       "minio-b:9000",
		"secure":     false,
		"access_key": "replication",
		"secret_key": "replication-secret",
	}))
	if diags.HasError() {
		t.Fatalf("res.Validate(secure=false) unexpected error: %v", diags)
	}
	foundWarning := false
	for _, d := range diags {
		if d.Severity == diag.Warning && strings.Contains(d.Summary, "secure is false") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("res.Validate(secure=false) diagnostics = %v, want warning mentioning secure is false", diags)
	}

	okDiags := res.Validate(replicationValidationConfig(map[string]interface{}{
		"bucket":     "dest",
		"host":       "minio-b:9000",
		"secure":     true,
		"access_key": "replication",
		"secret_key": "replication-secret",
	}))
	for _, d := range okDiags {
		if d.Severity == diag.Warning && strings.Contains(d.Summary, "secure is false") {
			t.Fatalf("res.Validate(secure=true) unexpectedly warned about secure=false: %v", okDiags)
		}
	}
}

func TestDiffBucketReplicationLegacyBandwidthConverges(t *testing.T) {
	res := resourceMinioBucketReplication()
	state := &terraform.InstanceState{
		ID: "source",
		Attributes: map[string]string{
			"id":                                  "source",
			"bucket":                              "source",
			"resync_version":                      "0",
			"rule.#":                              "1",
			"rule.0.enabled":                      "true",
			"rule.0.priority":                     "1",
			"rule.0.target.#":                     "1",
			"rule.0.target.0.bucket":              "dest",
			"rule.0.target.0.host":                "minio-b:9000",
			"rule.0.target.0.secure":              "true",
			"rule.0.target.0.path_style":          "auto",
			"rule.0.target.0.disable_proxy":       "false",
			"rule.0.target.0.synchronous":         "false",
			"rule.0.target.0.syncronous":          "false",
			"rule.0.target.0.health_check_period": "30s",
			"rule.0.target.0.bandwidth_limit":     "0",
			"rule.0.target.0.bandwidth_limt":      "200000000",
			"rule.0.target.0.access_key":          "replication",
			"rule.0.target.0.secret_key":          "replication-secret",
		},
	}
	cfg := replicationValidationConfig(map[string]interface{}{
		"bucket":         "dest",
		"host":           "minio-b:9000",
		"bandwidth_limt": "200M",
		"access_key":     "replication",
		"secret_key":     "replication-secret",
	})
	diff, err := res.Diff(context.Background(), state, cfg, nil)
	if err != nil {
		t.Fatalf("Diff returned unexpected error: %v", err)
	}
	if !diff.Empty() {
		t.Fatalf("expected empty diff after Read with bandwidth_limt=200M, got %#v", diff.Attributes)
	}
}

func TestDiffBucketReplicationBothBandwidthFieldsError(t *testing.T) {
	res := resourceMinioBucketReplication()
	cfg := replicationValidationConfig(map[string]interface{}{
		"bucket":          "dest",
		"host":            "minio-b:9000",
		"bandwidth_limit": "200M",
		"bandwidth_limt":  "200M",
		"access_key":      "replication",
		"secret_key":      "replication-secret",
	})
	_, err := res.Diff(context.Background(), nil, cfg, nil)
	if err == nil {
		t.Fatal("expected error when both bandwidth_limit and bandwidth_limt are set, got nil")
	}
	if !strings.Contains(err.Error(), "bandwidth_limit") || !strings.Contains(err.Error(), "bandwidth_limt") {
		t.Fatalf("error = %q, want error naming both bandwidth_limit and bandwidth_limt", err.Error())
	}
}

func TestExtractReplicationTargetKeepsLegacyZeroHealthCheckPeriod(t *testing.T) {
	res := resourceMinioBucketReplication()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{})
	cases := []struct {
		value   string
		wantErr bool
	}{
		// "0s" passed the old schema regex, so it can already be in state.
		// Read and Delete parse state through extract, so extract must keep
		// accepting it; validateReplicationHealthCheckPeriod is what rejects
		// it in new configurations.
		{value: "0s", wantErr: false},
		{value: "abc", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			rule := map[string]interface{}{
				"enabled":  true,
				"priority": 1,
				"tags":     map[string]interface{}{},
				"target": []interface{}{map[string]interface{}{
					"bucket":              "dest",
					"host":                "minio-b:9000",
					"access_key":          "replication",
					"secret_key":          "replication-secret",
					"secure":              true,
					"path_style":          "auto",
					"health_check_period": tc.value,
				}},
			}
			_, diags := extractBucketReplicationRule(context.Background(), 0, 1, rule, d)
			if diags.HasError() != tc.wantErr {
				t.Fatalf("extract with health_check_period=%q: HasError = %v, want %v (%v)", tc.value, diags.HasError(), tc.wantErr, diags)
			}
		})
	}
}
