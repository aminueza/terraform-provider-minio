package minio

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/minio/madmin-go"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// NewClient returns a new minio client
func (config *S3MinioConfig) NewClient() (client interface{}, err error) {

	var minioClient *minio.Client
	var minioCredentials *credentials.Credentials

	tr, err := config.customTransport()
	if err != nil {
		log.Println("[FATAL] Error configuring S3 client transport.")
		return nil, err
	}

	if config.S3APISignature == "v2" {
		minioCredentials = credentials.NewStaticV2(config.S3UserAccess, config.S3UserSecret, config.S3SessionToken)
		minioClient, err = minio.New(config.S3HostPort, &minio.Options{
			Creds:     minioCredentials,
			Secure:    config.S3SSL,
			Transport: tr,
		})
	} else if config.S3APISignature == "v4" {
		minioCredentials = credentials.NewStaticV4(config.S3UserAccess, config.S3UserSecret, config.S3SessionToken)
		minioClient, err = minio.New(config.S3HostPort, &minio.Options{
			Creds:     minioCredentials,
			Secure:    config.S3SSL,
			Transport: tr,
		})
	} else {
		return nil, fmt.Errorf("unknown S3 API signature: %s, must be v2 or v4", config.S3APISignature)
	}
	if err != nil {
		log.Println("[FATAL] Error building client for S3 server.")
		return nil, err
	}

	minioAdmin, err := madmin.NewWithOptions(config.S3HostPort, &madmin.Options{
		Creds:  minioCredentials,
		Secure: config.S3SSL,
	})
	//minioAdmin.TraceOn(nil)
	if err != nil {
		log.Println("[FATAL] Error building admin client for S3 server.")
		return nil, err
	}
	minioAdmin.SetCustomTransport(tr)

	return &S3MinioClient{
		S3UserAccess: config.S3UserAccess,
		S3Region:     config.S3Region,
		S3Client:     minioClient,
		S3Admin:      minioAdmin,
	}, nil
}

func isValidCertificate(c []byte) bool {
	p, _ := pem.Decode(c)
	if p == nil {
		return false
	}
	_, err := x509.ParseCertificates(p.Bytes)
	return err == nil
}

func (config *S3MinioConfig) customTransport() (*http.Transport, error) {

	if !config.S3SSL {
		return minio.DefaultTransport(config.S3SSL)
	}

	tlsConfig := &tls.Config{
		// Can't use SSLv3 because of POODLE and BEAST
		// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
		// Can't use TLSv1.1 because of RC4 cipher usage
		MinVersion: tls.VersionTLS12,
	}

	tr, err := minio.DefaultTransport(config.S3SSL)
	if err != nil {
		return nil, err
	}

	if config.S3SSLCACertFile != "" {
		minioCACert, err := os.ReadFile(config.S3SSLCACertFile)
		if err != nil {
			return nil, err
		}

		if !isValidCertificate(minioCACert) {
			return nil, fmt.Errorf("minio CA Cert is not a valid x509 certificate")
		}

		rootCAs, _ := x509.SystemCertPool()
		if rootCAs == nil {
			// In some systems (like Windows) system cert pool is
			// not supported or no certificates are present on the
			// system - so we create a new cert pool.
			rootCAs = x509.NewCertPool()
		}
		rootCAs.AppendCertsFromPEM(minioCACert)
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.RootCAs = rootCAs
	}

	if config.S3SSLCertFile != "" && config.S3SSLKeyFile != "" {
		minioPair, err := tls.LoadX509KeyPair(config.S3SSLCertFile, config.S3SSLKeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{minioPair}
	}

	if config.S3SSLSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	tr.TLSClientConfig = tlsConfig

	log.Printf("[DEBUG] S3 SSL client initialized")

	return tr, nil
}
