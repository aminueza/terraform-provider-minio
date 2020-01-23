package minio

import (
	madmin "github.com/aminueza/terraform-minio-provider/madmin"
	minio "github.com/minio/minio-go/v6"
	"github.com/minio/minio-go/v6/pkg/set"
)

//S3MinioConfig defines variable for minio
type S3MinioConfig struct {
	S3HostPort     string
	S3UserAccess   string
	S3UserSecret   string
	S3Region       string
	S3APISignature string
	S3SSL          bool
}

//S3MinioClient defines default minio
type S3MinioClient struct {
	S3UserAccess string
	S3Region     string
	S3Client     *minio.Client
	S3Admin      *madmin.AdminClient
}

//S3MinioBucket defines minio config
type S3MinioBucket struct {
	MinioClient *minio.Client
	MinioAdmin  *madmin.AdminClient
	MinioRegion string
	MinioBucket string
	MinioACL    string
	MinioAccess string
}

//S3MinioIAMUserConfig defines IAM config
type S3MinioIAMUserConfig struct {
	MinioAdmin        *madmin.AdminClient
	MinioIAMName      string
	MinioDisableUser  bool
	MinioForceDestroy bool
	MinioUpdateKey    bool
	MinioIAMTags      map[string]string
}

//S3MinioIAMGroupConfig defines IAM Group config
type S3MinioIAMGroupConfig struct {
	MinioAdmin        *madmin.AdminClient
	MinioIAMName      string
	MinioDisableGroup bool
	MinioForceDestroy bool
}

//S3MinioIAMGroupMembershipConfig defines IAM Group membership config
type S3MinioIAMGroupMembershipConfig struct {
	MinioAdmin    *madmin.AdminClient
	MinioIAMName  string
	MinioIAMUsers []*string
	MinioIAMGroup string
}

//S3MinioIAMPolicyConfig defines IAM Policy config
type S3MinioIAMPolicyConfig struct {
	MinioAdmin         *madmin.AdminClient
	MinioIAMName       string
	MinioIAMNamePrefix string
	MinioIAMPolicy     string
}

// Error represents a basic error that implies the error interface.
type Error struct {
	Message string
}

//Stmt defines policy statement
type Stmt struct {
	Sid        string
	Actions    set.StringSet `json:"Action"`
	Conditions ConditionMap  `json:"Condition,omitempty"`
	Effect     string
	Principal  string        `json:"Principal,omitempty"`
	Resources  set.StringSet `json:"Resource"`
}

//Princ defines policy princ
type Princ struct {
	AWS           set.StringSet `json:"AWS,omitempty"`
	CanonicalUser set.StringSet `json:"CanonicalUser,omitempty"`
}

//BucketPolicy defines bucket policy
type BucketPolicy struct {
	Version    string `json:"Version"`
	Statements []Stmt `json:"Statement"`
}

// UserStatus User status
type UserStatus struct {
	AccessKey string               `json:"accessKey,omitempty"`
	SecretKey string               `json:"secretKey,omitempty"`
	Status    madmin.AccountStatus `json:"status,omitempty"`
}

// Resource prefix for all aws resources.
const awsResourcePrefix = "arn:aws:s3:::"

// All bucket actions.
var allBucketActions = set.CreateStringSet("s3:GetBucketLocation", "s3:ListBucket", "s3:ListBucketMultipartUploads", "s3:GetObject", "s3:AbortMultipartUpload", "s3:DeleteObject", "s3:ListMultipartUploadParts", "s3:PutObject", "s3:CreateBucket", "s3:DeleteBucket", "s3:DeleteBucketPolicy", "s3:DeleteObject", "s3:GetBucketLocation", "s3:GetBucketNotification", "s3:GetBucketPolicy", "s3:GetObject", "s3:HeadBucket", "s3:ListAllMyBuckets", "s3:ListBucket", "s3:ListBucketMultipartUploads", "s3:ListenBucketNotification", "s3:ListMultipartUploadParts", "s3:PutObject", "s3:PutBucketPolicy", "s3:PutBucketNotification") //"s3:PutBucketLifecycle", "s3:GetBucketLifecycle"

// Common bucket actions for both read and write policies.
var commonBucketActions = set.CreateStringSet("s3:GetBucketLocation")

// Read only bucket actions.
var readOnlyBucketActions = set.CreateStringSet("s3:ListBucket")

// Write only bucket actions.
var writeOnlyBucketActions = set.CreateStringSet("s3:ListBucketMultipartUploads")

// Read only all bucket actions.
var readOnlyAllBucketsActions = set.CreateStringSet("s3:ListBucket", "s3:ListAllMyBuckets")

// Read only object actions.
var readOnlyObjectActions = set.CreateStringSet("s3:GetObject")

// Write object actions.
var uploadObjectActions = set.CreateStringSet("s3:PutObject")

// Write object acl.
var uploadObjectACL = set.CreateStringSet("s3:PutObjectAcl")

// Write only object actions.
var writeOnlyObjectActions = set.CreateStringSet("s3:AbortMultipartUpload", "s3:DeleteObject", "s3:ListMultipartUploadParts", "s3:PutObject")

// All object actions.
var allObjectActions = set.CreateStringSet("s3:*Object")

// Read and write object actions.
var readWriteObjectActions = readOnlyObjectActions.Union(writeOnlyObjectActions)

var readListObjectActions = readOnlyBucketActions.Union(commonBucketActions)

var readListMultObjectActions = readListObjectActions.Union(writeOnlyBucketActions)

var readListMyObjectActions = readOnlyBucketActions.Union(readOnlyObjectActions)

var uploadMyObjectActions = uploadObjectActions.Union(uploadObjectACL)
