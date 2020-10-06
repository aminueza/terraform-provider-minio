package minio

import (
	"github.com/minio/minio-go/v7/pkg/credentials"
	"log"

	madmin "github.com/aminueza/terraform-provider-minio/madmin"
	minio "github.com/minio/minio-go/v7"
)

//NewClient returns a new minio client
func (config *S3MinioConfig) NewClient() (interface{}, error) {

	minioClient := new(minio.Client)

	var err error
	if config.S3APISignature == "v2" {
		minioClient, err = minio.New(config.S3HostPort, &minio.Options{
			// config.S3UserAccess, config.S3UserSecret, config.S3SSL
			Creds:  credentials.NewStaticV4(config.S3UserAccess, config.S3UserSecret, ""),
			Secure: config.S3SSL,
		})
	} else if config.S3APISignature == "v4" {
		minioClient, err = minio.New(config.S3HostPort, &minio.Options{
			// config.S3UserAccess, config.S3UserSecret, config.S3SSL
			Creds:  credentials.NewStaticV4(config.S3UserAccess, config.S3UserSecret, ""),
			Secure: config.S3SSL,
		})
	} else {
		minioClient, err = minio.New(config.S3HostPort, &minio.Options{
			// config.S3UserAccess, config.S3UserSecret, config.S3SSL
			Creds:  credentials.NewStaticV4(config.S3UserAccess, config.S3UserSecret, ""),
			Secure: config.S3SSL,
		})
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
