package minio

import (
	"time"

	"github.com/minio/madmin-go/v4"
	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/notification"
	"github.com/minio/minio-go/v7/pkg/policy"
	"github.com/minio/minio-go/v7/pkg/set"
	"github.com/minio/minio-go/v7/pkg/sse"
)

type S3MinioConfig struct {
	S3HostPort        string
	S3UserAccess      string
	S3UserSecret      string
	S3Region          string
	S3SessionToken    string
	S3APISignature    string
	S3SSL             bool
	S3SSLCACertFile   string
	S3SSLCertFile     string
	S3SSLKeyFile      string
	S3SSLSkipVerify   bool
	SkipBucketTagging bool
	S3CompatMode      bool
	Edition           string

	AssumeRoleARN         string
	AssumeRoleSessionName string
	AssumeRoleDuration    int
	AssumeRolePolicy      string
	AssumeRoleExternalID  string

	WebIdentityToken     string
	WebIdentityTokenFile string
	WebIdentityDuration  int

	RequestTimeoutSeconds int
	MaxRetries            int
	RetryDelayMs          int
}

type S3MinioClient struct {
	S3UserAccess      string
	S3Region          string
	S3Client          *minio.Client
	S3Admin           *madmin.AdminClient
	S3Endpoint        string
	S3UserSecret      string
	S3SSL             bool
	SkipBucketTagging bool
	S3CompatMode      bool
	Edition           string

	RequestTimeoutSeconds int
	MaxRetries            int
	RetryDelayMs          int
}

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
	SkipBucketTagging    bool
	S3CompatMode         bool
}

type S3MinioBucketPolicy struct {
	MinioClient       *minio.Client
	MinioBucket       string
	MinioBucketPolicy string
}

type S3MinioBucketVersioningConfiguration struct {
	Status           string
	ExcludedPrefixes []string
	ExcludeFolders   bool
}

type S3PathStyle int8

const (
	S3PathStyleAuto S3PathStyle = iota
	S3PathStyleOn
	S3PathStyleOff
)

func (p S3PathStyle) String() string {
	switch p {
	case S3PathStyleOn:
		return "on"
	case S3PathStyleOff:
		return "off"
	default:
		return "auto"
	}
}

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

type S3MinioBucketReplicationRuleTarget struct {
	Bucket            string
	StorageClass      string
	Host              string
	Secure            bool
	Path              string
	PathStyle         S3PathStyle
	Synchronous       bool
	DisableProxy      bool
	HealthCheckPeriod time.Duration
	BandwidthLimit    int64
	Region            string
	AccessKey         string
	SecretKey         string
}

type S3MinioBucketVersioning struct {
	MinioClient             *minio.Client
	MinioBucket             string
	VersioningConfiguration *S3MinioBucketVersioningConfiguration
}

type S3MinioBucketReplication struct {
	MinioAdmin       *madmin.AdminClient
	MinioClient      *minio.Client
	MinioBucket      string
	ReplicationRules []S3MinioBucketReplicationRule
}

type S3MinioBucketNotification struct {
	MinioClient   *minio.Client
	MinioBucket   string
	Configuration *notification.Configuration
}

type S3MinioBucketServerSideEncryption struct {
	MinioClient   *minio.Client
	MinioBucket   string
	Configuration *sse.Configuration
}

type S3MinioBucketCors struct {
	MinioClient *minio.Client
	MinioBucket string
}

type S3MinioBucketObjectLockConfiguration struct {
	MinioClient       *minio.Client
	MinioBucket       string
	ObjectLockEnabled string
	Mode              *minio.RetentionMode
	Validity          *uint
	Unit              *minio.ValidityUnit
}

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

type S3MinioIAMUserConfig struct {
	MinioAdmin        *madmin.AdminClient
	MinioIAMName      string
	MinioSecret       string
	MinioDisableUser  bool
	MinioForceDestroy bool
	MinioUpdateKey    bool
	MinioIAMTags      map[string]string
}

type S3MinioIAMGroupConfig struct {
	MinioAdmin        *madmin.AdminClient
	MinioIAMName      string
	MinioDisableGroup bool
	MinioForceDestroy bool
}

type S3MinioIAMGroupAttachmentConfig struct {
	MinioAdmin    *madmin.AdminClient
	MinioIAMUser  string
	MinioIAMGroup string
}

type S3MinioIAMGroupMembershipConfig struct {
	MinioAdmin    *madmin.AdminClient
	MinioIAMName  string
	MinioIAMUsers []string
	MinioIAMGroup string
}

type S3MinioIAMPolicyConfig struct {
	MinioAdmin         *madmin.AdminClient
	MinioIAMName       string
	MinioIAMNamePrefix string
	MinioIAMPolicy     string
}

type S3MinioIAMGroupPolicyConfig struct {
	MinioAdmin         *madmin.AdminClient
	MinioIAMName       string
	MinioIAMNamePrefix string
	MinioIAMPolicy     string
	MinioIAMGroup      string
}

type S3MinioKMSKeyConfig struct {
	MinioAdmin    *madmin.AdminClient
	MinioKMSKeyID string
}

type S3MinioObjectTags struct {
	MinioClient    *minio.Client
	MinioBucket    string
	MinioObjectKey string
}

type S3MinioObjectLegalHold struct {
	MinioClient    *minio.Client
	MinioBucket    string
	MinioObjectKey string
	MinioVersionID string
	MinioStatus    string
}

type Princ struct {
	AWS           set.StringSet `json:"AWS,omitempty"`
	CanonicalUser set.StringSet `json:"CanonicalUser,omitempty"`
}

type BucketPolicy struct {
	Version    string             `json:",omitempty"`
	ID         string             `json:",omitempty"`
	Statements []policy.Statement `json:"Statement"`
}

type IAMPolicyDoc struct {
	Version    string                `json:"Version,omitempty"`
	ID         string                `json:"Id,omitempty"`
	Statements []*IAMPolicyStatement `json:"Statement"`
}

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

type IAMPolicyStatementCondition struct {
	Test     string `json:"-"`
	Variable string `json:"-"`
	Values   interface{}
}

type IAMPolicyStatementConditionSet []IAMPolicyStatementCondition

type ServiceAccountStatus struct {
	AccessKey     string `json:"accessKey,omitempty"`
	SecretKey     string `json:"secretKey,omitempty"`
	AccountStatus string `json:"status,omitempty"`
}

type UserStatus struct {
	AccessKey string               `json:"accessKey,omitempty"`
	SecretKey string               `json:"secretKey,omitempty"`
	Status    madmin.AccountStatus `json:"status,omitempty"`
}

type ResponseError struct {
	Code       string `json:"Code,omitempty"`
	Message    string `json:"Message,omitempty"`
	BucketName string `json:"BucketName,omitempty"`
	Region     string `json:"Region,omitempty"`
}

const awsResourcePrefix = "arn:aws:s3:::"

// Bucket actions granted by the `public` access type. Deliberately limited to discovery:
// anonymous callers must not be able to administer the bucket (create/delete it, or read and
// rewrite its policy and notification configuration).
var publicBucketActions = set.CreateStringSet("s3:GetBucketLocation", "s3:ListBucket", "s3:ListBucketMultipartUploads")

// Bucket actions granted by the `public-read` access type, matching what `mc anonymous set
// download` writes. s3:GetBucketLocation is what makes minio-go's policy.GetPolicy recognise
// the shape at all; without it every client reports the bucket as `custom`.
var downloadBucketActions = set.CreateStringSet("s3:GetBucketLocation", "s3:ListBucket")

// Bucket actions granted by the `public-write` access type, matching what `mc anonymous set
// upload` writes.
var uploadBucketActions = set.CreateStringSet("s3:GetBucketLocation", "s3:ListBucketMultipartUploads")

var readOnlyObjectActions = set.CreateStringSet("s3:GetObject")

var writeOnlyObjectActions = set.CreateStringSet("s3:AbortMultipartUpload", "s3:DeleteObject", "s3:ListMultipartUploadParts", "s3:PutObject")

var publicObjectActions = readOnlyObjectActions.Union(writeOnlyObjectActions)

type S3MinioPrometheusBearerToken struct {
	MinioAdmin     *madmin.AdminClient
	MinioAccessKey string
	MinioSecretKey string
	MetricType     string
	ExpiresIn      string
	Limit          int
}

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

type S3MinioIdpLdap struct {
	MinioAdmin         *madmin.AdminClient
	ServerAddr         string
	LookupBindDN       string
	LookupBindPassword string
	UserDNSearchBaseDN string
	UserDNSearchFilter string
	GroupSearchBaseDN  string
	GroupSearchFilter  string
	TLSSkipVerify      bool
	ServerInsecure     bool
	StartTLS           bool
	Enable             bool
}

type S3MinioIdpOpenId struct {
	MinioAdmin   *madmin.AdminClient
	Name         string
	ConfigURL    string
	ClientID     string
	ClientSecret string
	ClaimName    string
	ClaimPrefix  string
	Scopes       string
	RedirectURI  string
	DisplayName  string
	Comment      string
	RolePolicy   string
	Enable       bool
}

type S3MinioIncompleteUploadCleanup struct {
	MinioClient *minio.Client
	MinioBucket string
	MinioPrefix string
}

type S3MinioAuditWebhook struct {
	MinioAdmin *madmin.AdminClient
	Name       string
	Endpoint   string
	AuthToken  string
	Enable     bool
	QueueSize  int
	BatchSize  int
	ClientCert string
	ClientKey  string
}

type S3MinioIAMImport struct {
	MinioAdmin *madmin.AdminClient
	IAMData    string
}

type S3MinioBatchJob struct {
	MinioAdmin *madmin.AdminClient
	JobType    string
	JobYAML    string
}
