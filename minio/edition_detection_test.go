package minio

import (
	"context"
	"testing"
)

func TestDetectEdition_overrideWins(t *testing.T) {
	if got := detectEdition(context.Background(), nil, false, "AIStor"); got != "AIStor" {
		t.Fatalf("expected override to win, got %q", got)
	}
}

// Servers report `aistor` while the override is usually typed `AIStor`; both have to end up as
// the same string or comparisons against the detected edition silently miss.
func TestDetectEdition_normalizesTheAistorSpelling(t *testing.T) {
	if got := detectEdition(context.Background(), nil, false, "aistor"); got != aistorEdition {
		t.Fatalf("expected %q, got %q", aistorEdition, got)
	}
}

func TestDetectEdition_s3CompatModeSkipsProbe(t *testing.T) {
	if got := detectEdition(context.Background(), nil, true, ""); got != "" {
		t.Fatalf("expected empty edition in s3_compat_mode, got %q", got)
	}
}

func TestDetectEdition_overrideWinsEvenInS3CompatMode(t *testing.T) {
	if got := detectEdition(context.Background(), nil, true, "AIStor"); got != "AIStor" {
		t.Fatalf("override should bypass s3_compat_mode shortcut, got %q", got)
	}
}
