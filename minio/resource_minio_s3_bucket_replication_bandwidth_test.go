package minio

import (
	"testing"
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
