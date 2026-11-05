package minio

import (
	"slices"
	"strings"
	"testing"

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
