package minio

import (
	"testing"
	"time"
)

func TestCustomTransport_DefaultTimeout(t *testing.T) {
	config := &S3MinioConfig{
		RequestTimeoutSeconds: 30,
	}

	tr, err := config.customTransport()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if tr.ResponseHeaderTimeout != 30*time.Second {
		t.Errorf("expected ResponseHeaderTimeout 30s, got %v", tr.ResponseHeaderTimeout)
	}
}

func TestCustomTransport_CustomTimeout(t *testing.T) {
	config := &S3MinioConfig{
		RequestTimeoutSeconds: 60,
	}

	tr, err := config.customTransport()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if tr.ResponseHeaderTimeout != 60*time.Second {
		t.Errorf("expected ResponseHeaderTimeout 60s, got %v", tr.ResponseHeaderTimeout)
	}
}

func TestCustomTransport_ZeroTimeout(t *testing.T) {
	config := &S3MinioConfig{
		RequestTimeoutSeconds: 0,
	}

	tr, err := config.customTransport()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if tr.ResponseHeaderTimeout != 0 {
		t.Errorf("expected ResponseHeaderTimeout 0, got %v", tr.ResponseHeaderTimeout)
	}
}

func TestCustomTransport_SSLWithTimeout(t *testing.T) {
	config := &S3MinioConfig{
		S3SSL:                 true,
		S3SSLSkipVerify:       true,
		RequestTimeoutSeconds: 45,
	}

	tr, err := config.customTransport()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if tr.ResponseHeaderTimeout != 45*time.Second {
		t.Errorf("expected ResponseHeaderTimeout 45s, got %v", tr.ResponseHeaderTimeout)
	}
	if tr.TLSHandshakeTimeout != 45*time.Second {
		t.Errorf("expected TLSHandshakeTimeout 45s, got %v", tr.TLSHandshakeTimeout)
	}
}

func TestNewClient_PropagatesRetryConfig(t *testing.T) {
	config := &S3MinioConfig{
		S3HostPort:            "localhost:9000",
		S3UserAccess:          "minioadmin",
		S3UserSecret:          "minioadmin",
		S3APISignature:        "v4",
		RequestTimeoutSeconds: 60,
		MaxRetries:            10,
		RetryDelayMs:          2000,
	}

	client, err := config.NewClient()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	mc := client.(*S3MinioClient)
	if mc.RequestTimeoutSeconds != 60 {
		t.Errorf("expected RequestTimeoutSeconds 60, got %d", mc.RequestTimeoutSeconds)
	}
	if mc.MaxRetries != 10 {
		t.Errorf("expected MaxRetries 10, got %d", mc.MaxRetries)
	}
	if mc.RetryDelayMs != 2000 {
		t.Errorf("expected RetryDelayMs 2000, got %d", mc.RetryDelayMs)
	}
}
