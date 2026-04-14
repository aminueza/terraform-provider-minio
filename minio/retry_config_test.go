package minio

import (
	"testing"
	"time"
)

func TestGetRetryConfig_Defaults(t *testing.T) {
	client := &S3MinioClient{}
	rc := getRetryConfig(client)

	if rc.MaxRetries != 6 {
		t.Errorf("expected default MaxRetries 6, got %d", rc.MaxRetries)
	}
	if rc.MaxBackoff != 20*time.Second {
		t.Errorf("expected default MaxBackoff 20s, got %v", rc.MaxBackoff)
	}
	if rc.BackoffBase != 2.0 {
		t.Errorf("expected default BackoffBase 2.0, got %f", rc.BackoffBase)
	}
}

func TestGetRetryConfig_CustomValues(t *testing.T) {
	client := &S3MinioClient{
		MaxRetries:   10,
		RetryDelayMs: 2000,
	}
	rc := getRetryConfig(client)

	if rc.MaxRetries != 10 {
		t.Errorf("expected MaxRetries 10, got %d", rc.MaxRetries)
	}
	if rc.MaxBackoff != 40*time.Second {
		t.Errorf("expected MaxBackoff 40s, got %v", rc.MaxBackoff)
	}
}

func TestGetRetryConfig_NilClient(t *testing.T) {
	rc := getRetryConfig(nil)

	if rc.MaxRetries != 6 {
		t.Errorf("expected default MaxRetries 6, got %d", rc.MaxRetries)
	}
	if rc.MaxBackoff != 20*time.Second {
		t.Errorf("expected default MaxBackoff 20s, got %v", rc.MaxBackoff)
	}
}

func TestGetRetryConfig_HighRetryDelay(t *testing.T) {
	client := &S3MinioClient{
		MaxRetries:   3,
		RetryDelayMs: 5000,
	}
	rc := getRetryConfig(client)

	if rc.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", rc.MaxRetries)
	}
	if rc.MaxBackoff != 100*time.Second {
		t.Errorf("expected MaxBackoff 100s, got %v", rc.MaxBackoff)
	}
}
