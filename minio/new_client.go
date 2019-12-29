package minio

import (
	"log"

	madmin "github.com/aminueza/terraform-minio-provider/madmin"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"
	minio "github.com/minio/minio-go/v6"
)

//NewClient returns a new minio client
func (config *MinioConfig) NewClient() (interface{}, error) {

	minioClient := new(minio.Client)
	minioSession := new(session.Session)
	minioAwsS3Client := new(s3.S3)
	minioAwsIam := new(iam.IAM)

	var err error
	if config.S3APISignature == "v2" {
		minioClient, err = minio.NewV2(config.S3HostPort, config.S3UserAccess, config.S3UserSecret, config.S3SSL)
	} else if config.S3APISignature == "v4" {
		minioClient, err = minio.NewV4(config.S3HostPort, config.S3UserAccess, config.S3UserSecret, config.S3SSL)
	} else {
		minioClient, err = minio.New(config.S3HostPort, config.S3UserAccess, config.S3UserSecret, config.S3SSL)
	}

	minioAdmin, _ := madmin.New(config.S3HostPort, config.S3UserAccess, config.S3UserSecret, config.S3SSL)
	minioAdmin.TraceOn(nil)
	if err != nil {
		log.Println("[FATAL] Error connecting to S3 server.")
		return nil, err
	} else {
		if config.S3SSL {
			log.Printf("[DEBUG] S3 client initialized")
		}
	}

	if config.S3AWS {
		s3Config := &aws.Config{
			Credentials:      credentials.NewStaticCredentials(config.S3UserAccess, config.S3UserSecret, ""),
			Endpoint:         aws.String(config.S3HostPort),
			Region:           aws.String(config.S3Region),
			DisableSSL:       aws.Bool(true),
			S3ForcePathStyle: aws.Bool(false),
		}

		minioSession = session.New(s3Config)

		minioAwsS3Client = s3.New(minioSession)

		minioAwsIam = iam.New(minioSession)
	}

	return &S3MinioClient{
		S3UserAccess: config.S3UserAccess,
		S3Region:     config.S3Region,
		S3Client:     minioClient,
		S3Admin:      minioAdmin,
		S3Session:    minioSession,
		S3AwsClient:  minioAwsS3Client,
		S3AwsIam:     minioAwsIam,
	}, nil

}
