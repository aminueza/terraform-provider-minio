package minio

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	MinTLSVersion = tls.VersionTLS12
)

// aistorEdition is the canonical spelling for licensed builds. Servers report `aistor` while
// the provider override is usually typed `AIStor`, so detection folds both into this constant
// instead of leaving callers with two strings that never match.
const aistorEdition = "AIStor"

func (config *S3MinioConfig) NewClient(ctx context.Context) (interface{}, error) {
	tr, err := config.customTransport(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to configure transport: %w", err)
	}

	var minioCredentials *credentials.Credentials
	switch config.S3APISignature {
	case "v2":
		minioCredentials = credentials.NewStaticV2(config.S3UserAccess, config.S3UserSecret, config.S3SessionToken)
	case "v4":
		minioCredentials = credentials.NewStaticV4(config.S3UserAccess, config.S3UserSecret, config.S3SessionToken)
	default:
		return nil, fmt.Errorf("unsupported S3 API signature version %q: must be v2 or v4", config.S3APISignature)
	}

	if config.AssumeRoleARN != "" || config.AssumeRoleSessionName != "" {
		scheme := "http"
		if config.S3SSL {
			scheme = "https"
		}
		stsEndpoint := fmt.Sprintf("%s://%s", scheme, config.S3HostPort)

		stsCreds, err := credentials.NewSTSAssumeRole(stsEndpoint, credentials.STSAssumeRoleOptions{
			AccessKey:       config.S3UserAccess,
			SecretKey:       config.S3UserSecret,
			SessionToken:    config.S3SessionToken,
			RoleARN:         config.AssumeRoleARN,
			RoleSessionName: config.AssumeRoleSessionName,
			DurationSeconds: config.AssumeRoleDuration,
			Policy:          config.AssumeRolePolicy,
			ExternalID:      config.AssumeRoleExternalID,
			Location:        config.S3Region,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to assume role: %w", err)
		}
		minioCredentials = stsCreds
		tflog.Debug(ctx, "Using STS AssumeRole credentials", map[string]interface{}{"role": config.AssumeRoleARN, "session": config.AssumeRoleSessionName})
	}

	if config.WebIdentityToken != "" || config.WebIdentityTokenFile != "" {
		scheme := "http"
		if config.S3SSL {
			scheme = "https"
		}
		stsEndpoint := fmt.Sprintf("%s://%s", scheme, config.S3HostPort)

		getToken := func() (*credentials.WebIdentityToken, error) {
			token := config.WebIdentityToken
			if token == "" && config.WebIdentityTokenFile != "" {
				data, err := os.ReadFile(config.WebIdentityTokenFile)
				if err != nil {
					return nil, fmt.Errorf("reading web identity token file: %w", err)
				}
				token = string(data)
			}
			return &credentials.WebIdentityToken{
				Token:  token,
				Expiry: config.WebIdentityDuration,
			}, nil
		}

		wiCreds, err := credentials.NewSTSWebIdentity(stsEndpoint, getToken)
		if err != nil {
			return nil, fmt.Errorf("failed to assume role with web identity: %w", err)
		}
		minioCredentials = wiCreds
		tflog.Debug(ctx, "Using STS WebIdentity credentials")
	}

	minioClient, err := minio.New(config.S3HostPort, &minio.Options{
		Creds:     minioCredentials,
		Secure:    config.S3SSL,
		Transport: tr,
		Region:    config.S3Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	minioAdmin, err := madmin.NewWithOptions(config.S3HostPort, &madmin.Options{
		Creds:     minioCredentials,
		Secure:    config.S3SSL,
		Transport: tr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create admin client: %w", err)
	}

	return &S3MinioClient{
		S3UserAccess:          config.S3UserAccess,
		S3Region:              config.S3Region,
		S3Client:              minioClient,
		S3Admin:               minioAdmin,
		S3Endpoint:            config.S3HostPort,
		S3UserSecret:          config.S3UserSecret,
		S3SSL:                 config.S3SSL,
		SkipBucketTagging:     config.SkipBucketTagging,
		S3CompatMode:          config.S3CompatMode,
		Edition:               detectEdition(ctx, minioAdmin, config.S3CompatMode, config.Edition),
		RequestTimeoutSeconds: config.RequestTimeoutSeconds,
		MaxRetries:            config.MaxRetries,
		RetryDelayMs:          config.RetryDelayMs,
	}, nil
}

func detectEdition(ctx context.Context, admin *madmin.AdminClient, s3CompatMode bool, override string) string {
	if override != "" {
		tflog.Info(ctx, "Edition: using provider override", map[string]interface{}{"edition": override})
		return normalizeEdition(override)
	}
	if s3CompatMode {
		tflog.Info(ctx, "Edition: detection skipped (s3_compat_mode=true)")
		return ""
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := admin.ServerInfo(probeCtx)
	if err != nil {
		tflog.Warn(ctx, "Edition: ServerInfo probe failed; falling back to legacy MinIO path", map[string]interface{}{"err": err.Error()})
		return ""
	}
	for _, srv := range info.Servers {
		hasLicense := srv.License != nil
		if srv.Edition != "" {
			tflog.Info(ctx, "Edition: detected from ServerInfo.Edition", map[string]interface{}{"edition": srv.Edition, "license_present": hasLicense})
			return normalizeEdition(srv.Edition)
		}
		if hasLicense {
			tflog.Info(ctx, "Edition: ServerInfo.Edition empty but License present; assuming AIStor", map[string]interface{}{"edition": aistorEdition})
			return aistorEdition
		}
	}
	tflog.Info(ctx, "Edition: no AIStor markers found; using legacy MinIO path")
	return ""
}

func normalizeEdition(edition string) string {
	if strings.EqualFold(edition, aistorEdition) {
		return aistorEdition
	}
	return edition
}

func isValidCertificate(certBytes []byte) bool {
	block, _ := pem.Decode(certBytes)
	if block == nil {
		return false
	}
	_, err := x509.ParseCertificates(block.Bytes)
	return err == nil
}

func (config *S3MinioConfig) customTransport(ctx context.Context) (*http.Transport, error) {
	timeout := time.Duration(config.RequestTimeoutSeconds) * time.Second

	if !config.S3SSL {
		tr, err := minio.DefaultTransport(config.S3SSL)
		if err != nil {
			return nil, fmt.Errorf("failed to create default transport: %w", err)
		}
		tr.DialContext = (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext
		tr.ResponseHeaderTimeout = timeout
		return tr, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: MinTLSVersion,
	}

	tr, err := minio.DefaultTransport(config.S3SSL)
	if err != nil {
		return nil, fmt.Errorf("failed to create default transport: %w", err)
	}

	if config.S3SSLCACertFile != "" {
		if err := config.configureCACert(tlsConfig); err != nil {
			return nil, err
		}
	}

	if config.S3SSLCertFile != "" && config.S3SSLKeyFile != "" {
		if err := config.configureClientCert(tlsConfig); err != nil {
			return nil, err
		}
	}

	tlsConfig.InsecureSkipVerify = config.S3SSLSkipVerify

	tr.TLSClientConfig = tlsConfig

	tr.DialContext = (&net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	tr.TLSHandshakeTimeout = timeout
	tr.ResponseHeaderTimeout = timeout

	tflog.Debug(ctx, "MinIO SSL client transport configured successfully")
	return tr, nil
}

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

func (config *S3MinioConfig) configureClientCert(tlsConfig *tls.Config) error {
	cert, err := tls.LoadX509KeyPair(config.S3SSLCertFile, config.S3SSLKeyFile)
	if err != nil {
		return fmt.Errorf("failed to load client certificate and key: %w", err)
	}

	tlsConfig.Certificates = []tls.Certificate{cert}
	return nil
}
