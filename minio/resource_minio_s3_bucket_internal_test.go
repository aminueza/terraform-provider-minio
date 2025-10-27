package minio

import (
	"context"
	"testing"
	"time"
)

func TestWaitForBucketVisibleImmediate(t *testing.T) {
	calls := 0
	exists := func(ctx context.Context) (bool, error) {
		calls++
		return true, nil
	}
	cfg := RetryConfig{MaxRetries: 3, MaxBackoff: time.Millisecond, BackoffBase: 2.0}
	found, err := waitForBucketVisible(context.Background(), exists, cfg, "bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatalf("expected found=true")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestWaitForBucketVisibleDelayed(t *testing.T) {
	calls := 0
	exists := func(ctx context.Context) (bool, error) {
		calls++
		if calls < 3 {
			return false, nil
		}
		return true, nil
	}
	cfg := RetryConfig{MaxRetries: 5, MaxBackoff: time.Millisecond, BackoffBase: 2.0}
	found, err := waitForBucketVisible(context.Background(), exists, cfg, "bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatalf("expected found=true after retries")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls (2 false then true), got %d", calls)
	}
}

func TestWaitForBucketVisibleNever(t *testing.T) {
	calls := 0
	exists := func(ctx context.Context) (bool, error) {
		calls++
		return false, nil
	}
	cfg := RetryConfig{MaxRetries: 4, MaxBackoff: time.Millisecond, BackoffBase: 2.0}
	found, err := waitForBucketVisible(context.Background(), exists, cfg, "bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatalf("expected found=false after exhausting retries")
	}
	if calls != cfg.MaxRetries {
		t.Fatalf("expected %d calls, got %d", cfg.MaxRetries, calls)
	}
}
