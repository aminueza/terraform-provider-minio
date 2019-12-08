package mconfig

import (
	"log"
	"strconv"

	"github.com/minio/minio-go"
)

//NewClient returns a new minio client
func (config *MinioConfig) NewClient() (interface{}, error) {

	minioClient := new(minio.Client)

	sslbool, errSSL := strconv.ParseBool(config.S3SSL)
	if errSSL != nil {
		log.Print(errSSL)
	}

	debugbool, errDebug := strconv.ParseBool(config.S3Debug)
	if errDebug != nil {
		log.Print(errDebug)
	}

	var err error
	if config.S3APISignature == "v2" {
		minioClient, err = minio.NewV2(config.S3HostPort, config.S3UserAccess, config.S3UserSecret, sslbool)
	} else if config.S3APISignature == "v4" {
		minioClient, err = minio.NewV4(config.S3HostPort, config.S3UserAccess, config.S3UserSecret, sslbool)
	} else {
		minioClient, err = minio.New(config.S3HostPort, config.S3UserAccess, config.S3UserSecret, sslbool)
	}
	if err != nil {
		log.Println("[FATAL] Error connecting to S3 server.")
		return nil, err
	} else {
		if debugbool {
			log.Printf("[DEBUG] S3 client initialized")
		}
	}

	return &S3MinioClient{
		S3UserAccess: config.S3UserAccess,
		S3Region:     config.S3Region,
		S3Client:     minioClient,
	}, nil

}
