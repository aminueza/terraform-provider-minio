package minio

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// s3CompatReadCase describes one resource read that must survive a backend
// which does not implement the feature it reads.
type s3CompatReadCase struct {
	name     string
	resource func() *schema.Resource
	read     func(context.Context, *schema.ResourceData, interface{}) diag.Diagnostics
	// create and feature drive the write half of the table. A case without a
	// create is read-only and the write test skips it.
	create  func(context.Context, *schema.ResourceData, interface{}) diag.Diagnostics
	feature string
	// wantWriteRequest is the request the create must reach when it differs
	// from the one the read reaches, as it does whenever the two use different
	// HTTP methods on the same path.
	wantWriteRequest string
	id               string
	raw              map[string]interface{}
	attributes       []string
	wantRequest      string
	// methodNotAllowed is what a 405 means to this read. The object resources
	// pass s3Compat405IsAnAnswer, because 405 on an object GET or HEAD is an
	// answer about the object rather than a missing feature.
	methodNotAllowed string
	// compatOffDrops records that the read already dropped the resource on a
	// failed read before s3_compat_mode existed. The regression test pins that
	// behaviour instead of asserting a uniform one.
	compatOffDrops bool
}

// s3CompatReadCases covers every resource that consults s3_compat_mode. The
// raw maps hold a plausible non-empty state, so a read that skipped the
// compatibility branch would leave a value behind and fail the zero assertion.
func s3CompatReadCases(bucket, key string) []s3CompatReadCase {
	tierName := strings.ToUpper(acctest.RandString(8))
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::` + bucket + `/*"]}]}`

	return []s3CompatReadCase{
		{
			name:     "minio_s3_bucket_cors",
			resource: resourceMinioS3BucketCors,
			read:     minioReadBucketCors,
			create:   minioCreateBucketCors,
			feature:  "CORS configuration",
			id:       bucket,
			raw: map[string]interface{}{
				"bucket": bucket,
				"cors_rule": []interface{}{map[string]interface{}{
					"id":              "compat-stub-rule",
					"allowed_methods": []interface{}{"GET"},
					"allowed_origins": []interface{}{"*"},
				}},
			},
			attributes:  []string{"cors_rule"},
			wantRequest: "?cors=",
		},
		{
			name:     "minio_s3_bucket_lifecycle",
			resource: resourceMinioS3BucketLifecycle,
			read:     minioReadS3BucketLifecycle,
			create:   minioCreateS3BucketLifecycle,
			feature:  "bucket lifecycle configuration",
			id:       bucket,
			raw: map[string]interface{}{
				"bucket": bucket,
				"rule": []interface{}{map[string]interface{}{
					"id":     "compat-stub-rule",
					"status": "Enabled",
					"expiration": []interface{}{map[string]interface{}{
						"days": 30,
					}},
				}},
			},
			attributes:  []string{"rule"},
			wantRequest: "?lifecycle=",
		},
		{
			name:     "minio_s3_bucket_notification",
			resource: resourceMinioBucketNotification,
			read:     minioReadBucketNotification,
			create:   minioPutBucketNotification,
			feature:  "bucket notification configuration",
			id:       bucket,
			raw: map[string]interface{}{
				"bucket": bucket,
				"queue": []interface{}{map[string]interface{}{
					"id":        "compat-stub-queue",
					"queue_arn": "arn:minio:sqs::primary:webhook",
					"events":    []interface{}{"s3:ObjectCreated:*"},
				}},
			},
			attributes:  []string{"queue"},
			wantRequest: "?notification=",
		},
		{
			name:             "minio_s3_bucket_object_lock_configuration",
			resource:         resourceMinioS3BucketObjectLockConfiguration,
			read:             minioReadObjectLockConfiguration,
			create:           minioCreateObjectLockConfiguration,
			feature:          "object lock configuration",
			wantWriteRequest: "?versioning=",
			id:               bucket,
			raw: map[string]interface{}{
				"bucket":              bucket,
				"object_lock_enabled": true,
				"rule": []interface{}{map[string]interface{}{
					"default_retention": []interface{}{map[string]interface{}{
						"mode": "GOVERNANCE",
						"days": 7,
					}},
				}},
			},
			attributes:  []string{"object_lock_enabled", "rule"},
			wantRequest: "?object-lock=",
		},
		{
			name:     "minio_ilm_policy",
			resource: resourceMinioILMPolicy,
			read:     minioReadILMPolicy,
			create:   minioCreateILMPolicy,
			feature:  "ILM policy",
			id:       bucket,
			raw: map[string]interface{}{
				"bucket": bucket,
				"rule": []interface{}{map[string]interface{}{
					"id":         "compat-stub-rule",
					"status":     "Enabled",
					"expiration": "30d",
				}},
			},
			attributes:     []string{"rule"},
			wantRequest:    "?lifecycle=",
			compatOffDrops: true,
		},
		{
			name:     "minio_s3_bucket_policy",
			resource: resourceMinioBucketPolicy,
			read:     minioReadBucketPolicy,
			create:   minioPutBucketPolicy,
			feature:  "bucket policy",
			id:       bucket,
			raw: map[string]interface{}{
				"bucket": bucket,
				"policy": policy,
			},
			attributes:  []string{"policy"},
			wantRequest: "?policy=",
		},
		{
			name:     "minio_s3_bucket_versioning",
			resource: resourceMinioBucketVersioning,
			read:     minioReadBucketVersioning,
			create:   minioPutBucketVersioning,
			feature:  "bucket versioning configuration",
			id:       bucket,
			raw: map[string]interface{}{
				"bucket": bucket,
				"versioning_configuration": []interface{}{map[string]interface{}{
					"status": "Enabled",
				}},
			},
			attributes:  []string{"versioning_configuration"},
			wantRequest: "?versioning=",
		},
		{
			name:     "minio_s3_bucket_anonymous_access",
			resource: resourceMinioS3BucketAnonymousAccess,
			read:     minioReadAnonymousPolicy,
			create:   minioSetAnonymousPolicy,
			feature:  "anonymous access policy",
			id:       encodeAnonymousAccessID(bucket),
			raw: map[string]interface{}{
				"bucket":      bucket,
				"access_type": "public-read",
				"policy":      policy,
			},
			attributes:  []string{"policy", "access_type"},
			wantRequest: "?policy=",
		},
		{
			name:     "minio_s3_bucket_server_side_encryption_configuration",
			resource: resourceMinioBucketServerSideEncryption,
			read:     minioReadBucketServerSideEncryption,
			create:   minioPutBucketServerSideEncryption,
			feature:  "bucket encryption configuration",
			id:       bucket,
			raw: map[string]interface{}{
				"bucket":          bucket,
				"encryption_type": "aws:kms",
				"kms_key_id":      "compat-stub-key",
			},
			attributes:     []string{"encryption_type", "kms_key_id"},
			wantRequest:    "?encryption=",
			compatOffDrops: true,
		},
		{
			name:             "minio_s3_bucket_quota",
			resource:         resourceMinioBucketQuota,
			read:             minioReadBucketQuota,
			create:           minioCreateBucketQuota,
			feature:          "bucket quota",
			wantWriteRequest: "set-bucket-quota",
			id:               bucket,
			raw: map[string]interface{}{
				"bucket": bucket,
				"quota":  4096,
				"type":   "hard",
			},
			attributes:  []string{"quota", "type"},
			wantRequest: "get-bucket-quota",
		},
		{
			name:             "minio_s3_bucket_retention",
			resource:         resourceMinioBucketRetention,
			read:             minioReadRetention,
			create:           minioCreateRetention,
			feature:          "bucket object lock configuration",
			wantWriteRequest: "?versioning=",
			id:               bucket,
			raw: map[string]interface{}{
				"bucket":          bucket,
				"mode":            "GOVERNANCE",
				"unit":            "DAYS",
				"validity_period": 30,
			},
			attributes:  []string{"mode", "unit", "validity_period"},
			wantRequest: "?object-lock=",
		},
		{
			name:     "minio_s3_bucket_replication",
			resource: resourceMinioBucketReplication,
			read:     minioReadBucketReplication,
			create:   minioPutBucketReplication,
			feature:  "bucket replication configuration",
			id:       bucket,
			raw: map[string]interface{}{
				"bucket": bucket,
				"rule": []interface{}{map[string]interface{}{
					"id":                          "compat-stub-rule",
					"enabled":                     true,
					"priority":                    1,
					"tags":                        map[string]interface{}{"env": "stub"},
					"delete_replication":          true,
					"delete_marker_replication":   true,
					"existing_object_replication": true,
					"metadata_sync":               true,
					"target": []interface{}{map[string]interface{}{
						"bucket":     bucket,
						"host":       "s3.example.com",
						"region":     "us-east-1",
						"access_key": "accesskey",
						"secret_key": "secretkey",
					}},
				}},
			},
			attributes:  []string{"rule"},
			wantRequest: "?replication=",
		},
		{
			name:             "minio_s3_object_tags",
			resource:         resourceMinioObjectTags,
			read:             minioReadObjectTags,
			create:           minioCreateObjectTags,
			feature:          "object tags",
			wantWriteRequest: "PUT /",
			id:               bucket + "/" + key,
			raw: map[string]interface{}{
				"bucket": bucket,
				"key":    key,
				"tags":   map[string]interface{}{"env": "stub"},
			},
			attributes:       []string{"tags"},
			wantRequest:      "?tagging=",
			methodNotAllowed: s3Compat405IsAnAnswer,
		},
		{
			name:     "minio_s3_object_legal_hold",
			resource: resourceMinioObjectLegalHold,
			read:     minioReadObjectLegalHold,
			create:   minioCreateObjectLegalHold,
			feature:  "object legal hold",
			id:       bucket + "/" + key,
			raw: map[string]interface{}{
				"bucket": bucket,
				"key":    key,
				"status": "ON",
			},
			attributes:       []string{"status"},
			wantRequest:      "?legal-hold=",
			methodNotAllowed: s3Compat405IsAnAnswer,
		},
		{
			name:     "minio_s3_object_retention",
			resource: resourceMinioObjectRetention,
			read:     minioReadObjectRetention,
			create:   minioCreateObjectRetention,
			feature:  "object retention",
			id:       bucket + "/" + key,
			raw: map[string]interface{}{
				"bucket":            bucket,
				"key":               key,
				"mode":              "GOVERNANCE",
				"retain_until_date": "2099-01-02T15:04:05Z",
			},
			attributes:       []string{"mode", "retain_until_date"},
			wantRequest:      "?retention=",
			methodNotAllowed: s3Compat405IsAnAnswer,
		},
		{
			name:     "minio_s3_object",
			resource: resourceMinioObject,
			read:     minioReadObject,
			create:   minioCreateObject,
			feature:  "object",
			// minio-go uploads a stream of unknown length as a multipart
			// upload, so the create reaches the initiate call, not a plain PUT.
			wantWriteRequest: "?uploads=",
			id:               bucket + "/" + key,
			raw: map[string]interface{}{
				"bucket_name":   bucket,
				"object_name":   key,
				"content":       "compat-stub-body",
				"content_type":  "text/plain",
				"etag":          "compat-stub-etag",
				"storage_class": "STANDARD",
				"metadata":      map[string]interface{}{"x-amz-meta-stub": "1"},
			},
			attributes: []string{
				"etag", "content_type", "content_encoding", "storage_class",
				"cache_control", "content_disposition", "expires", "metadata",
			},
			wantRequest:      "HEAD /" + bucket + "/" + key,
			methodNotAllowed: s3Compat405IsAnAnswer,
		},
		{
			name:             "minio_ilm_tier",
			resource:         resourceMinioILMTier,
			read:             minioReadILMTier,
			create:           minioCreateILMTier,
			feature:          "remote tier",
			wantWriteRequest: "/tier",
			id:               tierName,
			raw: map[string]interface{}{
				"name":     tierName,
				"bucket":   bucket,
				"type":     "s3",
				"endpoint": "s3.example.com",
				"region":   "us-east-1",
				"s3_config": []interface{}{map[string]interface{}{
					"access_key":    "stub-access-key",
					"secret_key":    "stub-secret-key",
					"storage_class": "STANDARD",
				}},
			},
			attributes: []string{
				"type", "prefix", "name", "bucket", "endpoint", "region",
				"minio_config", "s3_config", "azure_config", "gcs_config",
			},
			wantRequest: "/tier",
		},
	}
}

// TestS3CompatReadAbsorbsMissingFeature is the read half of the decided
// semantics: a backend that answers NotImplemented leaves the resource in state
// with the attributes the last write stored, and only that response is absorbed.
// A 405 on an object read is an answer about the object, so it stays an error
// there while the bucket sub-resources still absorb it. Every resource that
// consults s3_compat_mode is covered, against both codes a real backend uses.
func TestS3CompatReadAbsorbsMissingFeature(t *testing.T) {
	bucket := "compat-stub-" + acctest.RandString(8)
	key := "object-" + acctest.RandString(8)

	for _, status := range []int{http.StatusNotImplemented, http.StatusMethodNotAllowed} {
		for _, tc := range s3CompatReadCases(bucket, key) {
			t.Run(tc.name+"/"+s3CompatStubCode(status), func(t *testing.T) {
				stub := newS3CompatStub(t, bucket, status)
				d := schema.TestResourceDataRaw(t, tc.resource().Schema, tc.raw)
				d.SetId(tc.id)

				before := make(map[string]interface{}, len(tc.attributes))
				for _, attribute := range tc.attributes {
					before[attribute] = d.Get(attribute)
				}

				diags := tc.read(context.Background(), d, stub.provider(true))

				if !stub.sawRequest(tc.wantRequest) {
					t.Fatalf("the stub never received the feature request %q, so the compatibility branch was never reached; requests: %v", tc.wantRequest, stub.requestLog())
				}
				if d.Id() == "" {
					t.Fatal("resource was dropped from state, which produces a perpetual diff")
				}

				// A 405 is only a missing feature where the backend would serve
				// it on another method, so an object read must still fail.
				if status == http.StatusMethodNotAllowed && tc.methodNotAllowed == s3Compat405IsAnAnswer {
					if !diags.HasError() {
						t.Fatalf("read absorbed a 405 on an object read, which is an answer about the object; requests: %v", stub.requestLog())
					}
					for _, message := range s3CompatDiagMessages(diags) {
						if strings.Contains(message, "s3_compat_mode") {
							t.Errorf("diagnostic %q mentions s3_compat_mode for a response that is not a missing feature", message)
						}
					}
					return
				}

				if diags.HasError() {
					t.Fatalf("read returned an error with s3_compat_mode on: %v; requests: %v", s3CompatDiagMessages(diags), stub.requestLog())
				}
				for _, attribute := range tc.attributes {
					if got := d.Get(attribute); !reflect.DeepEqual(got, before[attribute]) {
						t.Errorf("attribute %q = %#v, want the value the last write left (%#v): an absorbed read says nothing about what was stored", attribute, got, before[attribute])
					}
				}
			})
		}
	}
}

func s3CompatDiagMessages(diags diag.Diagnostics) []string {
	messages := make([]string, 0, len(diags))
	for _, d := range diags {
		messages = append(messages, d.Summary)
	}
	return messages
}

// TestS3CompatWriteNamesFeatureAndFlag is the create half of the decided
// semantics: the write still fails, but the diagnostic names the feature and
// points at s3_compat_mode instead of leaving the backend's response as the
// only thing the user sees. One S3 resource and one admin-API resource, since
// the two clients build their errors differently.
func TestS3CompatWriteNamesFeatureAndFlag(t *testing.T) {
	bucket := "compat-stub-" + acctest.RandString(8)
	key := "object-" + acctest.RandString(8)

	for _, status := range []int{http.StatusNotImplemented, http.StatusMethodNotAllowed} {
		for _, tc := range s3CompatReadCases(bucket, key) {
			if tc.create == nil {
				continue
			}
			t.Run(tc.name+"/"+s3CompatStubCode(status), func(t *testing.T) {
				stub := newS3CompatStub(t, bucket, status)
				d := schema.TestResourceDataRaw(t, tc.resource().Schema, tc.raw)

				wantRequest := tc.wantRequest
				if tc.wantWriteRequest != "" {
					wantRequest = tc.wantWriteRequest
				}

				diags := tc.create(context.Background(), d, stub.provider(true))
				if !diags.HasError() {
					t.Fatalf("create succeeded against a backend that does not support the feature; requests: %v", stub.requestLog())
				}
				summary := strings.Join(s3CompatDiagMessages(diags), " | ")
				if !stub.sawRequest(wantRequest) {
					t.Fatalf("the stub never received the write request %q; error: %s; requests: %v", wantRequest, summary, stub.requestLog())
				}
				if !strings.Contains(summary, tc.feature) {
					t.Errorf("error %q does not name the feature %q", summary, tc.feature)
				}
				if !strings.Contains(summary, "s3_compat_mode") {
					t.Errorf("error %q does not point at s3_compat_mode", summary)
				}
				if d.Id() != "" {
					t.Errorf("a failed create left the resource %q in state", d.Id())
				}
			})
		}
	}
}

func TestS3CompatOffLeavesReadsUnchanged(t *testing.T) {
	bucket := "compat-stub-" + acctest.RandString(8)
	key := "object-" + acctest.RandString(8)

	for _, status := range []int{http.StatusNotImplemented, http.StatusMethodNotAllowed} {
		for _, tc := range s3CompatReadCases(bucket, key) {
			t.Run(tc.name+"/"+s3CompatStubCode(status), func(t *testing.T) {
				stub := newS3CompatStub(t, bucket, status)
				d := schema.TestResourceDataRaw(t, tc.resource().Schema, tc.raw)
				d.SetId(tc.id)

				before := make(map[string]interface{}, len(tc.attributes))
				for _, attribute := range tc.attributes {
					before[attribute] = d.Get(attribute)
				}

				diags := tc.read(context.Background(), d, stub.provider(false))

				if !stub.sawRequest(tc.wantRequest) {
					t.Fatalf("the stub never received the feature request %q; requests: %v", tc.wantRequest, stub.requestLog())
				}

				for _, message := range s3CompatDiagMessages(diags) {
					if strings.Contains(message, "s3_compat_mode") {
						t.Errorf("diagnostic %q mentions s3_compat_mode with the flag off", message)
					}
				}

				if tc.compatOffDrops {
					// Pinned pre-existing behaviour: this read swallowed every
					// read error and removed the resource from state.
					if diags.HasError() {
						t.Errorf("read returned %v, want the pre-existing silent drop", s3CompatDiagMessages(diags))
					}
					if d.Id() != "" {
						t.Errorf("resource %q is still in state, want it removed as it was before", d.Id())
					}
					return
				}

				if !diags.HasError() {
					t.Fatalf("read succeeded against a backend that does not support the feature and s3_compat_mode is off; requests: %v", stub.requestLog())
				}
				if d.Id() != tc.id {
					t.Errorf("resource id = %q, want %q: a failed read with the flag off must keep the resource", d.Id(), tc.id)
				}
				for _, attribute := range tc.attributes {
					if got := d.Get(attribute); !reflect.DeepEqual(got, before[attribute]) {
						t.Errorf("attribute %q = %#v, want the value it held before the read (%#v)", attribute, got, before[attribute])
					}
				}
			})
		}
	}
}

// TestS3CompatOffLeavesWritesUnchanged is the write half of the regression: with
// the flag off the diagnostic is the one NewResourceError produced before, with
// the backend's error appended and no hint about s3_compat_mode.
func TestS3CompatOffLeavesWritesUnchanged(t *testing.T) {
	bucket := "compat-stub-" + acctest.RandString(8)

	cases := []struct {
		name        string
		resource    func() *schema.Resource
		create      func(context.Context, *schema.ResourceData, interface{}) diag.Diagnostics
		raw         map[string]interface{}
		wantSummary string
		wantRequest string
	}{
		{
			name:     "minio_s3_bucket_cors",
			resource: resourceMinioS3BucketCors,
			create:   minioCreateBucketCors,
			raw: map[string]interface{}{
				"bucket": bucket,
				"cors_rule": []interface{}{map[string]interface{}{
					"id":              "compat-stub-rule",
					"allowed_methods": []interface{}{"GET"},
					"allowed_origins": []interface{}{"*"},
				}},
			},
			wantSummary: "[FATAL] creating CORS configuration (" + bucket + "): ",
			wantRequest: "?cors=",
		},
		{
			name:     "minio_s3_bucket_quota",
			resource: resourceMinioBucketQuota,
			create:   minioCreateBucketQuota,
			raw: map[string]interface{}{
				"bucket": bucket,
				"quota":  4096,
				"type":   "hard",
			},
			wantSummary: "[FATAL] setting bucket quota (" + bucket + "): ",
			wantRequest: "set-bucket-quota",
		},
	}

	for _, status := range []int{http.StatusNotImplemented, http.StatusMethodNotAllowed} {
		for _, tc := range cases {
			t.Run(tc.name+"/"+s3CompatStubCode(status), func(t *testing.T) {
				stub := newS3CompatStub(t, bucket, status)
				d := schema.TestResourceDataRaw(t, tc.resource().Schema, tc.raw)

				diags := tc.create(context.Background(), d, stub.provider(false))
				if !diags.HasError() {
					t.Fatalf("create succeeded against a backend that does not support the feature; requests: %v", stub.requestLog())
				}
				if !stub.sawRequest(tc.wantRequest) {
					t.Fatalf("the stub never received the write request %q; requests: %v", tc.wantRequest, stub.requestLog())
				}
				summary := diags[0].Summary
				if !strings.HasPrefix(summary, tc.wantSummary) {
					t.Errorf("error summary %q, want the pre-existing summary starting with %q", summary, tc.wantSummary)
				}
				if strings.Contains(summary, "s3_compat_mode") {
					t.Errorf("error summary %q mentions s3_compat_mode with the flag off", summary)
				}
				if strings.Contains(summary, "is not supported by this S3 backend, and s3_compat_mode") {
					t.Errorf("error summary %q carries the compatibility hint added for s3_compat_mode", summary)
				}
				if d.Id() != "" {
					t.Errorf("a failed create left the resource %q in state", d.Id())
				}
			})
		}
	}
}
