package minio

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	madmin "github.com/aminueza/terraform-minio-provider/madmin"
	minio "github.com/minio/minio-go/v6"
)

//NewClient returns a new minio client
func (config *S3MinioConfig) NewClient() (interface{}, error) {

	minioClient := new(minio.Client)
	tlsConfig := &tls.Config{
		// Can't use SSLv3 because of POODLE and BEAST
		// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
		// Can't use TLSv1.1 because of RC4 cipher usage
		MinVersion: tls.VersionTLS12,
	}
	tr := http.DefaultTransport

	var err error
	if config.S3APISignature == "v2" {
		minioClient, err = minio.NewV2(config.S3HostPort, config.S3UserAccess, config.S3UserSecret, config.S3SSL)
	} else if config.S3APISignature == "v4" {
		minioClient, err = minio.NewV4(config.S3HostPort, config.S3UserAccess, config.S3UserSecret, config.S3SSL)
	} else {
		minioClient, err = minio.New(config.S3HostPort, config.S3UserAccess, config.S3UserSecret, config.S3SSL)
	}

	minioAdmin, _ := madmin.New(config.S3HostPort, config.S3UserAccess, config.S3UserSecret, config.S3SSL)
	//minioAdmin.TraceOn(nil)
	if err != nil {
		log.Println("[FATAL] Error connecting to S3 server.")
		return nil, err
	}

	if config.S3SSL {
		log.Printf("[DEBUG] S3 client initialized")
	}

	if config.S3SSLCACertFile != "" {
		minioCACert, err := ioutil.ReadFile(config.S3SSLCACertFile)
		if err != nil {
			return nil, err
		}

		if !isValidCertificate(minioCACert) {
			return nil, fmt.Errorf("Minio CA Cert is not a valid x509 certificate")
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

	minioClient.SetCustomTransport(tr)
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
	if err != nil {
		return false
	}
	return true
}
