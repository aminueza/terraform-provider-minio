package minio

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	// MinTLSVersion is the minimum TLS version supported
	MinTLSVersion = tls.VersionTLS12
)

// NewClient creates and configures both S3 and admin clients for MinIO
// It handles the setup of credentials, SSL/TLS configuration, and custom transport options
func (config *S3MinioConfig) NewClient() (interface{}, error) {
	// Set up custom transport with SSL/TLS configuration
	tr, err := config.customTransport()
	if err != nil {
		return nil, fmt.Errorf("failed to configure transport: %w", err)
	}

	// Initialize credentials based on API signature version
	var minioCredentials *credentials.Credentials
	switch config.S3APISignature {
	case "v2":
		minioCredentials = credentials.NewStaticV2(config.S3UserAccess, config.S3UserSecret, config.S3SessionToken)
	case "v4":
		minioCredentials = credentials.NewStaticV4(config.S3UserAccess, config.S3UserSecret, config.S3SessionToken)
	default:
		return nil, fmt.Errorf("unsupported S3 API signature version %q: must be v2 or v4", config.S3APISignature)
	}

	// Initialize S3 client
	minioClient, err := minio.New(config.S3HostPort, &minio.Options{
		Creds:     minioCredentials,
		Secure:    config.S3SSL,
		Transport: tr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Initialize admin client
	minioAdmin, err := madmin.NewWithOptions(config.S3HostPort, &madmin.Options{
		Creds:  minioCredentials,
		Secure: config.S3SSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create admin client: %w", err)
	}
	minioAdmin.SetCustomTransport(tr)

	return &S3MinioClient{
		S3UserAccess:      config.S3UserAccess,
		S3Region:          config.S3Region,
		S3Client:          minioClient,
		S3Admin:           minioAdmin,
		S3Endpoint:        config.S3HostPort,
		S3UserSecret:      config.S3UserSecret,
		S3SSL:             config.S3SSL,
		SkipBucketTagging: config.SkipBucketTagging,
	}, nil
}

// isValidCertificate checks if the provided bytes represent a valid x509 certificate in PEM format
func isValidCertificate(certBytes []byte) bool {
	block, _ := pem.Decode(certBytes)
	if block == nil {
		return false
	}
	_, err := x509.ParseCertificates(block.Bytes)
	return err == nil
}

// customTransport creates and configures an HTTP transport with SSL/TLS settings
// It handles CA certificates, client certificates, and verification settings
func (config *S3MinioConfig) customTransport() (*http.Transport, error) {
	// If SSL is disabled, return default transport
	if !config.S3SSL {
		return minio.DefaultTransport(config.S3SSL)
	}

	// Initialize TLS config with minimum version
	tlsConfig := &tls.Config{
		MinVersion: MinTLSVersion, // Minimum TLS 1.2 for security
	}

	// Get default transport
	tr, err := minio.DefaultTransport(config.S3SSL)
	if err != nil {
		return nil, fmt.Errorf("failed to create default transport: %w", err)
	}

	// Configure CA certificate if provided
	if config.S3SSLCACertFile != "" {
		if err := config.configureCACert(tlsConfig); err != nil {
			return nil, err
		}
	}

	// Configure client certificates if both cert and key are provided
	if config.S3SSLCertFile != "" && config.S3SSLKeyFile != "" {
		if err := config.configureClientCert(tlsConfig); err != nil {
			return nil, err
		}
	}

	// Configure SSL verification
	tlsConfig.InsecureSkipVerify = config.S3SSLSkipVerify

	// Set TLS config on transport
	tr.TLSClientConfig = tlsConfig

	log.Printf("[DEBUG] MinIO SSL client transport configured successfully")
	return tr, nil
}

// configureCACert loads and configures the CA certificate for TLS verification
func (config *S3MinioConfig) configureCACert(tlsConfig *tls.Config) error {
	caCert, err := os.ReadFile(config.S3SSLCACertFile)
	if err != nil {
		return fmt.Errorf("failed to read CA certificate file: %w", err)
	}

	if !isValidCertificate(caCert) {
		return fmt.Errorf("invalid CA certificate: not a valid x509 certificate")
	}

	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		// Some systems don't support system cert pool
		rootCAs = x509.NewCertPool()
	}

	if !rootCAs.AppendCertsFromPEM(caCert) {
		return fmt.Errorf("failed to append CA certificate to cert pool")
	}

	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	tlsConfig.RootCAs = rootCAs
	return nil
}

// configureClientCert loads and configures client certificates for mutual TLS
func (config *S3MinioConfig) configureClientCert(tlsConfig *tls.Config) error {
	cert, err := tls.LoadX509KeyPair(config.S3SSLCertFile, config.S3SSLKeyFile)
	if err != nil {
		return fmt.Errorf("failed to load client certificate and key: %w", err)
	}

	tlsConfig.Certificates = []tls.Certificate{cert}
	return nil
}
