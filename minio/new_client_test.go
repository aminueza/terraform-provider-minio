package minio

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCustomTransport_DefaultTimeout(t *testing.T) {
	config := &S3MinioConfig{
		RequestTimeoutSeconds: 30,
	}

	tr, err := config.customTransport(context.Background())
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

	tr, err := config.customTransport(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if tr.ResponseHeaderTimeout != 60*time.Second {
		t.Errorf("expected ResponseHeaderTimeout 60s, got %v", tr.ResponseHeaderTimeout)
	}
}

func TestCustomTransport_NonPositiveTimeoutFallsBackToTheDefault(t *testing.T) {
	for _, seconds := range []int{0, -1} {
		config := &S3MinioConfig{
			RequestTimeoutSeconds: seconds,
		}

		tr, err := config.customTransport(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if tr.ResponseHeaderTimeout != defaultRequestTimeoutSeconds*time.Second {
			t.Errorf("RequestTimeoutSeconds %d gives ResponseHeaderTimeout %v, want the default %ds", seconds, tr.ResponseHeaderTimeout, defaultRequestTimeoutSeconds)
		}
	}
}

func TestCustomTransport_SSLWithTimeout(t *testing.T) {
	config := &S3MinioConfig{
		S3SSL:                 true,
		S3SSLSkipVerify:       true,
		RequestTimeoutSeconds: 45,
	}

	tr, err := config.customTransport(context.Background())
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

func TestNewClient_PropagatesRegion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		region string
	}{
		{"default us-east-1", "us-east-1"},
		{"custom region", "eu-central-1"},
		{"arbitrary S3-compat region", "my-custom-region"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &S3MinioConfig{
				S3HostPort:     "localhost:9000",
				S3UserAccess:   "minioadmin",
				S3UserSecret:   "minioadmin",
				S3APISignature: "v4",
				S3Region:       tc.region,
			}

			client, err := config.NewClient(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			mc := client.(*S3MinioClient)
			if mc.S3Region != tc.region {
				t.Errorf("expected S3Region %q, got %q", tc.region, mc.S3Region)
			}
		})
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

	client, err := config.NewClient(context.Background())
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

func newTestCertificateAuthority(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating the CA key: %s", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "terraform-provider-minio test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating the CA certificate: %s", err)
	}

	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing the generated CA certificate: %s", err)
	}
	return ca, key
}

func issueTestCertificate(t *testing.T, serial int64, commonName string, usage x509.ExtKeyUsage, ca *x509.Certificate, caKey *ecdsa.PrivateKey) (certPEM []byte, keyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating the %q key: %s", commonName, err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	if usage == x509.ExtKeyUsageServerAuth {
		template.DNSNames = []string{"localhost"}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("creating the %q certificate: %s", commonName, err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("encoding the %q key: %s", commonName, err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func writeTestPEMFile(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing %s: %s", name, err)
	}
	return path
}

func TestConfigureCACertDoesNotSetClientAuth(t *testing.T) {
	ca, _ := newTestCertificateAuthority(t)
	caFile := writeTestPEMFile(t, "ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}))

	tlsConfig := &tls.Config{MinVersion: MinTLSVersion}
	config := &S3MinioConfig{S3SSLCACertFile: caFile}

	if err := config.configureCACert(tlsConfig); err != nil {
		t.Fatalf("configuring the CA certificate: %s", err)
	}

	if tlsConfig.RootCAs == nil {
		t.Error("RootCAs is not set: minio_cacert_file must put the authority in the client root pool")
	}
	if tlsConfig.ClientAuth != tls.NoClientCert {
		t.Errorf("ClientAuth = %d: the field is a server-side policy, and on a client configuration it is never read", tlsConfig.ClientAuth)
	}
}

func TestCustomTransportVerifiesTheServerAgainstTheConfiguredCA(t *testing.T) {
	ca, caKey := newTestCertificateAuthority(t)
	serverCertPEM, serverKeyPEM := issueTestCertificate(t, 2, "minio test server", x509.ExtKeyUsageServerAuth, ca, caKey)
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("loading the server certificate: %s", err)
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		MinVersion:   MinTLSVersion,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	caFile := writeTestPEMFile(t, "ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}))

	for _, tc := range []struct {
		name    string
		caFile  string
		wantErr string
	}{
		{"minio_cacert_file trusts a private certificate authority", caFile, ""},
		{"no CA file rejects a private certificate authority", "", "certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &S3MinioConfig{S3SSL: true, S3SSLCACertFile: tc.caFile}
			tr, err := config.customTransport(context.Background())
			if err != nil {
				t.Fatalf("building the transport: %s", err)
			}

			resp, err := (&http.Client{Transport: tr}).Get(srv.URL)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("the request succeeded against a certificate authority the provider does not trust")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want it to mention %q", err, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("the configured CA certificate did not verify the server: %s", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
		})
	}
}

func TestCustomTransportPresentsTheConfiguredClientCertificate(t *testing.T) {
	ca, caKey := newTestCertificateAuthority(t)
	serverCertPEM, serverKeyPEM := issueTestCertificate(t, 2, "minio test server", x509.ExtKeyUsageServerAuth, ca, caKey)
	clientCertPEM, clientKeyPEM := issueTestCertificate(t, 3, "minio test client", x509.ExtKeyUsageClientAuth, ca, caKey)

	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("loading the server certificate: %s", err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(ca)

	var peerCN string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) > 0 {
			peerCN = r.TLS.PeerCertificates[0].Subject.CommonName
		}
		fmt.Fprintln(w, "ok")
	}))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   MinTLSVersion,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	caFile := writeTestPEMFile(t, "ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}))
	certFile := writeTestPEMFile(t, "client.pem", clientCertPEM)
	keyFile := writeTestPEMFile(t, "client-key.pem", clientKeyPEM)

	t.Run("minio_cert_file and minio_key_file present the client certificate", func(t *testing.T) {
		config := &S3MinioConfig{
			S3SSL:           true,
			S3SSLCACertFile: caFile,
			S3SSLCertFile:   certFile,
			S3SSLKeyFile:    keyFile,
		}
		tr, err := config.customTransport(context.Background())
		if err != nil {
			t.Fatalf("building the transport: %s", err)
		}

		resp, err := (&http.Client{Transport: tr}).Get(srv.URL)
		if err != nil {
			t.Fatalf("the server rejected the configured client certificate: %s", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if peerCN != "minio test client" {
			t.Errorf("the server saw client certificate %q, want %q", peerCN, "minio test client")
		}
	})

	t.Run("a server requiring a client certificate rejects a client without one", func(t *testing.T) {
		config := &S3MinioConfig{S3SSL: true, S3SSLCACertFile: caFile}
		tr, err := config.customTransport(context.Background())
		if err != nil {
			t.Fatalf("building the transport: %s", err)
		}

		resp, err := (&http.Client{Transport: tr}).Get(srv.URL)
		if err == nil {
			_ = resp.Body.Close()
			t.Fatal("the request succeeded without a client certificate against a server that requires one")
		}
	})
}
