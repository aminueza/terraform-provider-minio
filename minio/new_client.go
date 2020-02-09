package minio

import (
	"log"

	madmin "github.com/aminueza/terraform-minio-provider/madmin"
	minio "github.com/minio/minio-go/v6"
)

//NewClient returns a new minio client
func (config *S3MinioConfig) NewClient() (interface{}, error) {

	minioClient := new(minio.Client)

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

	return &S3MinioClient{
		S3UserAccess: config.S3UserAccess,
		S3Region:     config.S3Region,
		S3Client:     minioClient,
		S3Admin:      minioAdmin,
	}, nil

}
