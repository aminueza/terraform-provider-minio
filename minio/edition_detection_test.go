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
