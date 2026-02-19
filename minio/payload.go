package minio

import (
	"time"

	"github.com/minio/madmin-go/v3"
	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/notification"
	"github.com/minio/minio-go/v7/pkg/policy"
	"github.com/minio/minio-go/v7/pkg/set"
	"github.com/minio/minio-go/v7/pkg/sse"
)

// S3MinioConfig defines variable for minio
type S3MinioConfig struct {
	S3HostPort      string
	S3UserAccess    string
	S3UserSecret    string
	S3Region        string
	S3SessionToken  string
	S3APISignature  string
	S3SSL           bool
	S3SSLCACertFile string
	S3SSLCertFile   string
	S3SSLKeyFile    string
	S3SSLSkipVerify bool
}

// S3MinioClient defines default minio
type S3MinioClient struct {
	S3UserAccess string
	S3Region     string
	S3Client     *minio.Client
	S3Admin      *madmin.AdminClient
	S3Endpoint   string
	S3UserSecret string
	S3SSL        bool
}

// S3MinioBucket defines minio config
type S3MinioBucket struct {
	MinioClient          *minio.Client
	MinioAdmin           *madmin.AdminClient
	MinioRegion          string
	MinioBucket          string
	MinioBucketPrefix    string
	MinioACL             string
	MinioAccess          string
	MinioForceDestroy    bool
	ObjectLockingEnabled bool
}

// S3MinioBucketPolicy defines bucket policy config
type S3MinioBucketPolicy struct {
	MinioClient       *minio.Client
	MinioBucket       string
	MinioBucketPolicy string
}

// S3MinioBucketVersioningConfiguration defines bucket versioning config
type S3MinioBucketVersioningConfiguration struct {
	Status           string
	ExcludedPrefixes []string
	ExcludeFolders   bool
}

// S3PathSyle
type S3PathSyle int8

const (
	S3PathSyleAuto S3PathSyle = iota
	S3PathSyleOn
	S3PathSyleOff
)

func (p S3PathSyle) String() string {
	switch p {
	case S3PathSyleOn:
		return "on"
	case S3PathSyleOff:
		return "off"
	default:
		return "auto"
	}
}

// S3MinioBucketReplicationConfiguration defines bucket replication rule
type S3MinioBucketReplicationRule struct {
	Id       string
	Arn      string
	Enabled  bool
	Priority int

	Prefix string
	Tags   map[string]string

	DeleteReplication         bool
	DeleteMarkerReplication   bool
	ExistingObjectReplication bool
	MetadataSync              bool

	Target S3MinioBucketReplicationRuleTarget
}

// S3MinioBucketReplicationRuleTarget defines bucket replication rule target
type S3MinioBucketReplicationRuleTarget struct {
	Bucket            string
	StorageClass      string
	Host              string
	Secure            bool
	Path              string
	PathStyle         S3PathSyle
	Synchronous       bool
	DisableProxy      bool
	HealthCheckPeriod time.Duration
	BandwidthLimit    int64
	Region            string
	AccessKey         string
	SecretKey         string
}

// S3MinioBucketVersioning defines bucket versioning
type S3MinioBucketVersioning struct {
	MinioClient             *minio.Client
	MinioBucket             string
	VersioningConfiguration *S3MinioBucketVersioningConfiguration
}

// S3MinioBucketReplication defines bucket replication
type S3MinioBucketReplication struct {
	MinioAdmin       *madmin.AdminClient
	MinioClient      *minio.Client
	MinioBucket      string
	ReplicationRules []S3MinioBucketReplicationRule
}

// S3MinioBucketNotification
type S3MinioBucketNotification struct {
	MinioClient   *minio.Client
	MinioBucket   string
	Configuration *notification.Configuration
}

// S3MinioBucketServerSideEncryption defines bucket encryption
type S3MinioBucketServerSideEncryption struct {
	MinioClient   *minio.Client
	MinioBucket   string
	Configuration *sse.Configuration
}

// S3MinioBucketCors defines bucket CORS configuration
type S3MinioBucketCors struct {
	MinioClient *minio.Client
	MinioBucket string
}

// S3MinioBucketObjectLockConfiguration defines bucket object lock configuration
type S3MinioBucketObjectLockConfiguration struct {
	MinioClient       *minio.Client
	MinioBucket       string
	ObjectLockEnabled string
	Mode              *minio.RetentionMode
	Validity          *uint
	Unit              *minio.ValidityUnit
}

// S3MinioServiceAccountConfig defines service account config
type S3MinioServiceAccountConfig struct {
	MinioAdmin        *madmin.AdminClient
	MinioTargetUser   string
	MinioAccessKey    string
	MinioSecretKey    string
	MinioSAPolicy     string
	MinioDisableUser  bool
	MinioForceDestroy bool
	MinioUpdateKey    bool
	MinioIAMTags      map[string]string
	MinioDescription  string
	MinioName         string
	MinioExpiration   string
}

// S3MinioIAMUserConfig defines IAM config
type S3MinioIAMUserConfig struct {
	MinioAdmin        *madmin.AdminClient
	MinioIAMName      string
	MinioSecret       string
	MinioDisableUser  bool
	MinioForceDestroy bool
	MinioUpdateKey    bool
	MinioIAMTags      map[string]string
}

// S3MinioIAMGroupConfig defines IAM Group config
type S3MinioIAMGroupConfig struct {
	MinioAdmin        *madmin.AdminClient
	MinioIAMName      string
	MinioDisableGroup bool
	MinioForceDestroy bool
}

// S3MinioIAMGroupAttachmentConfig defines IAM Group membership config
type S3MinioIAMGroupAttachmentConfig struct {
	MinioAdmin    *madmin.AdminClient
	MinioIAMUser  string
	MinioIAMGroup string
}

// S3MinioIAMGroupMembershipConfig defines IAM Group membership config
type S3MinioIAMGroupMembershipConfig struct {
	MinioAdmin    *madmin.AdminClient
	MinioIAMName  string
	MinioIAMUsers []*string
	MinioIAMGroup string
}

// S3MinioIAMPolicyConfig defines IAM Policy config
type S3MinioIAMPolicyConfig struct {
	MinioAdmin         *madmin.AdminClient
	MinioIAMName       string
	MinioIAMNamePrefix string
	MinioIAMPolicy     string
}

// S3MinioIAMGroupPolicyConfig defines IAM Policy config
type S3MinioIAMGroupPolicyConfig struct {
	MinioAdmin         *madmin.AdminClient
	MinioIAMName       string
	MinioIAMNamePrefix string
	MinioIAMPolicy     string
	MinioIAMGroup      string
}

// S3MinioKMSKeyConfig defines service account config
type S3MinioKMSKeyConfig struct {
	MinioAdmin    *madmin.AdminClient
	MinioKMSKeyID string
}

// S3MinioObjectTags defines object tags configuration
type S3MinioObjectTags struct {
	MinioClient    *minio.Client
	MinioBucket    string
	MinioObjectKey string
}

// S3MinioObjectLegalHold defines object legal hold configuration
type S3MinioObjectLegalHold struct {
	MinioClient    *minio.Client
	MinioBucket    string
	MinioObjectKey string
	MinioVersionID string
	MinioStatus    string
}

// Princ defines policy princ
type Princ struct {
	AWS           set.StringSet `json:"AWS,omitempty"`
	CanonicalUser set.StringSet `json:"CanonicalUser,omitempty"`
}

// BucketPolicy defines bucket policy
type BucketPolicy struct {
	Version    string             `json:",omitempty"`
	ID         string             `json:",omitempty"`
	Statements []policy.Statement `json:"Statement"`
}

// IAMPolicyDoc returns IAM policy
type IAMPolicyDoc struct {
	Version    string                `json:"Version,omitempty"`
	ID         string                `json:"Id,omitempty"`
	Statements []*IAMPolicyStatement `json:"Statement"`
}

// IAMPolicyStatement returns IAM policy statement
type IAMPolicyStatement struct {
	Sid          string
	Effect       string      `json:",omitempty"`
	Actions      interface{} `json:"Action,omitempty"`
	Resources    interface{} `json:"Resource,omitempty"`
	NotResources interface{} `json:"NotResource,omitempty"`
	Principal    string      `json:"Principal,omitempty"`
	NotPrincipal string      `json:"NotPrincipal,omitempty"`
	Conditions   interface{} `json:"Condition,omitempty"`
}

// IAMPolicyStatementCondition returns IAM policy condition
type IAMPolicyStatementCondition struct {
	Test     string `json:"-"`
	Variable string `json:"-"`
	Values   interface{}
}

// IAMPolicyStatementConditionSet returns IAM policy condition set
type IAMPolicyStatementConditionSet []IAMPolicyStatementCondition

// ServiceAccountStatus User status
type ServiceAccountStatus struct {
	AccessKey     string `json:"accessKey,omitempty"`
	SecretKey     string `json:"secretKey,omitempty"`
	AccountStatus string `json:"status,omitempty"`
}

// UserStatus User status
type UserStatus struct {
	AccessKey string               `json:"accessKey,omitempty"`
	SecretKey string               `json:"secretKey,omitempty"`
	Status    madmin.AccountStatus `json:"status,omitempty"`
}

// ResponseError handles error message
type ResponseError struct {
	Code       string `json:"Code,omitempty"`
	Message    string `json:"Message,omitempty"`
	BucketName string `json:"BucketName,omitempty"`
	Region     string `json:"Region,omitempty"`
}

// Resource prefix for all aws resources.
const awsResourcePrefix = "arn:aws:s3:::"

// All bucket actions.
var allBucketActions = set.CreateStringSet("s3:GetBucketLocation", "s3:ListBucket", "s3:ListBucketMultipartUploads", "s3:GetObject", "s3:AbortMultipartUpload", "s3:DeleteObject", "s3:ListMultipartUploadParts", "s3:PutObject", "s3:CreateBucket", "s3:DeleteBucket", "s3:DeleteBucketPolicy", "s3:DeleteObject", "s3:GetBucketLocation", "s3:GetBucketNotification", "s3:GetBucketPolicy", "s3:GetObject", "s3:HeadBucket", "s3:ListAllMyBuckets", "s3:ListBucket", "s3:ListBucketMultipartUploads", "s3:ListenBucketNotification", "s3:ListMultipartUploadParts", "s3:PutObject", "s3:PutBucketPolicy", "s3:PutBucketNotification") //"s3:PutBucketLifecycle", "s3:GetBucketLifecycle"

// Read only bucket actions.
var readOnlyBucketActions = set.CreateStringSet("s3:ListBucket")

// Read only all bucket actions.
var readOnlyAllBucketsActions = set.CreateStringSet("s3:ListBucket", "s3:ListAllMyBuckets")

// Read only object actions.
var readOnlyObjectActions = set.CreateStringSet("s3:GetObject")

// Write object actions.
var uploadObjectActions = set.CreateStringSet("s3:PutObject")

// Write only object actions.
var writeOnlyObjectActions = set.CreateStringSet("s3:AbortMultipartUpload", "s3:DeleteObject", "s3:ListMultipartUploadParts", "s3:PutObject")

var readListMyObjectActions = readOnlyBucketActions.Union(readOnlyObjectActions)

// S3MinioPrometheusBearerToken defines Prometheus bearer token configuration
type S3MinioPrometheusBearerToken struct {
	MinioAdmin     *madmin.AdminClient
	MinioAccessKey string
	MinioSecretKey string
	MetricType     string
	ExpiresIn      string
	Limit          int
}

// S3MinioPrometheusScrapeConfig defines Prometheus scrape configuration
type S3MinioPrometheusScrapeConfig struct {
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	UseSSL         bool
	MetricType     string
	Alias          string
	MetricsVersion string
	BearerToken    string
}
