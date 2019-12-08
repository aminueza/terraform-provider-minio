package pactions

import (
	"github.com/minio/minio-go"
	"github.com/minio/minio-go/pkg/set"
)

//MinioBucket defines minio config
type MinioBucket struct {
	MinioClient *minio.Client
	MinioRegion string
	MinioBucket string
	MinioDebug  string
	MinioACL    string
	MinioAccess string
}

// Error represents a basic error that implies the error interface.
type Error struct {
	Message string
}

//Stmt defines policy statement
type Stmt struct {
	Actions    set.StringSet `json:"Action"`
	Conditions ConditionMap  `json:"Condition,omitempty"`
	Effect     string
	Principal  string        `json:"Principal,omitempty"`
	Resources  set.StringSet `json:"Resource"`
	Sid        string
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

// Resource prefix for all aws resources.
const awsResourcePrefix = "arn:aws:s3:::"

// All bucket actions.
var allBucketActions = set.CreateStringSet("s3:*")

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

// Write only object actions.
var writeOnlyObjectActions = set.CreateStringSet("s3:AbortMultipartUpload", "s3:DeleteObject", "s3:ListMultipartUploadParts", "s3:PutObject")

// All object actions.
var allObjectActions = set.CreateStringSet("s3:*Object")

// Read and write object actions.
var readWriteObjectActions = readOnlyObjectActions.Union(writeOnlyObjectActions)

var readListObjectActions = readOnlyBucketActions.Union(commonBucketActions)

var readListMultObjectActions = readListObjectActions.Union(writeOnlyBucketActions)

var readListMyObjectActions = readOnlyBucketActions.Union(readOnlyObjectActions)
