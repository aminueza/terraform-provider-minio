package mconfig

import "github.com/minio/minio-go"

//MinioConfig defines variable for minio
type MinioConfig struct {
	S3HostPort     string
	S3UserAccess   string
	S3UserSecret   string
	S3Region       string
	S3APISignature string
	S3SSL          string
	S3Debug        string
}

//S3MinioClient defines default minio
type S3MinioClient struct {
	S3UserAccess string
	S3Region     string
	S3Client     *minio.Client
}
