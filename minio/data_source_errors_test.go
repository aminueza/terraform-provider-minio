package minio

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func newFailingDataSourceClients(t *testing.T, code, msg string) *S3MinioClient {
	t.Helper()

	type s3ErrorXML struct {
		XMLName   xml.Name `xml:"Error"`
		Code      string   `xml:"Code"`
		Message   string   `xml:"Message"`
		RequestID string   `xml:"RequestId"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/minio/admin/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"Code":    code,
				"Message": msg,
			})
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_ = xml.NewEncoder(w).Encode(s3ErrorXML{
			Code:      code,
			Message:   msg,
			RequestID: "req-1",
		})
	}))
	t.Cleanup(srv.Close)

	endpoint := strings.TrimPrefix(srv.URL, "http://")
	adminClient, err := madmin.NewWithOptions(endpoint, &madmin.Options{
		Creds:  credentials.NewStaticV4("access", "secret", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating madmin client: %v", err)
	}
	s3Client, err := miniogo.New(endpoint, &miniogo.Options{
		Creds:        credentials.NewStaticV4("access", "secret", ""),
		Secure:       false,
		BucketLookup: miniogo.BucketLookupPath,
	})
	if err != nil {
		t.Fatalf("creating minio client: %v", err)
	}

	return &S3MinioClient{
		S3Admin:  adminClient,
		S3Client: s3Client,
	}
}

func assertNewResourceErrorWithDetails(t *testing.T, diags diag.Diagnostics, wantSummaryPrefix string) {
	t.Helper()
	if !diags.HasError() {
		t.Fatalf("expected error diagnostics, got none")
	}
	if !strings.Contains(diags[0].Summary, wantSummaryPrefix) {
		t.Errorf("summary = %q, want substring %q", diags[0].Summary, wantSummaryPrefix)
	}
	foundDetails := false
	for _, d := range diags {
		if d.Summary == "Server Response Details" {
			foundDetails = true
			break
		}
	}
	if !foundDetails {
		t.Errorf("expected 'Server Response Details' diagnostic, got %+v", diags)
	}
}

func TestDataSourcesReturnNewResourceErrorOnServerErrors(t *testing.T) {
	meta := newFailingDataSourceClients(t, "AccessDenied", "Access Denied")
	ctx := context.Background()

	cases := []struct {
		name              string
		ds                *schema.Resource
		raw               map[string]interface{}
		wantSummaryPrefix string
	}{
		{
			name:              "minio_iam_policy",
			ds:                dataSourceIAMPolicy(),
			raw:               map[string]interface{}{"name": "missing-policy"},
			wantSummaryPrefix: "[FATAL] reading IAM policy (missing-policy):",
		},
		{
			name:              "minio_iam_group",
			ds:                dataSourceIAMGroup(),
			raw:               map[string]interface{}{"name": "missing-group"},
			wantSummaryPrefix: "[FATAL] reading IAM group (missing-group):",
		},
		{
			name:              "minio_iam_service_accounts",
			ds:                dataSourceIAMServiceAccounts(),
			raw:               map[string]interface{}{"user": "alice"},
			wantSummaryPrefix: "[FATAL] listing IAM service accounts (alice):",
		},
		{
			name:              "minio_account_info",
			ds:                dataSourceMinioAccountInfo(),
			raw:               map[string]interface{}{},
			wantSummaryPrefix: "[FATAL] reading account info (account_info):",
		},
		{
			name:              "minio_data_usage",
			ds:                dataSourceMinioDataUsage(),
			raw:               map[string]interface{}{},
			wantSummaryPrefix: "[FATAL] reading data usage (data_usage):",
		},
		{
			name:              "minio_ilm_tier_stats",
			ds:                dataSourceMinioILMTierStats(),
			raw:               map[string]interface{}{},
			wantSummaryPrefix: "[FATAL] reading ILM tier stats (ilm_tier_stats):",
		},
		{
			name:              "minio_ilm_tiers",
			ds:                dataSourceMinioILMTiers(),
			raw:               map[string]interface{}{},
			wantSummaryPrefix: "[FATAL] listing ILM tiers (ilm_tiers):",
		},
		{
			name:              "minio_server_info",
			ds:                dataSourceMinioServerInfo(),
			raw:               map[string]interface{}{},
			wantSummaryPrefix: "[FATAL] reading server info (server_info):",
		},
		{
			name:              "minio_storage_info",
			ds:                dataSourceMinioStorageInfo(),
			raw:               map[string]interface{}{},
			wantSummaryPrefix: "[FATAL] reading storage info (storage_info):",
		},
		{
			name:              "minio_s3_bucket_object_lock_configuration",
			ds:                dataSourceMinioS3BucketObjectLockConfiguration(),
			raw:               map[string]interface{}{"bucket": "my-bucket"},
			wantSummaryPrefix: "[FATAL] reading bucket object lock configuration (my-bucket):",
		},
		{
			name:              "minio_s3_bucket_versioning",
			ds:                dataSourceMinioS3BucketVersioning(),
			raw:               map[string]interface{}{"bucket": "my-bucket"},
			wantSummaryPrefix: "[FATAL] reading bucket versioning (my-bucket):",
		},
		{
			name:              "minio_s3_buckets",
			ds:                dataSourceMinioS3Buckets(),
			raw:               map[string]interface{}{"name_prefix": "logs-"},
			wantSummaryPrefix: "[FATAL] listing S3 buckets (logs-):",
		},
		{
			name:              "minio_s3_objects",
			ds:                dataSourceMinioS3Objects(),
			raw:               map[string]interface{}{"bucket": "my-bucket"},
			wantSummaryPrefix: "[FATAL] listing S3 objects (my-bucket):",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := schema.TestResourceDataRaw(t, tc.ds.Schema, tc.raw)
			diags := tc.ds.ReadContext(ctx, d, meta)
			assertNewResourceErrorWithDetails(t, diags, tc.wantSummaryPrefix)
		})
	}
}

func TestDataSourceMinioIAMPolicyDocumentErrorsUseNewResourceError(t *testing.T) {
	ds := dataSourceMinioIAMPolicyDocument()
	ctx := context.Background()

	cases := []struct {
		name        string
		raw         map[string]interface{}
		wantSummary string
	}{
		{
			name:        "bad_source_json",
			raw:         map[string]interface{}{"policy_id": "p1", "source_json": "{not json"},
			wantSummary: "[FATAL] unmarshaling source_json (p1):",
		},
		{
			name:        "bad_override_json",
			raw:         map[string]interface{}{"override_json": "{not json"},
			wantSummary: "[FATAL] unmarshaling override_json (iam_policy_document):",
		},
		{
			name: "bad_resources",
			raw: map[string]interface{}{
				"version": "2008-10-17",
				"statement": []interface{}{
					map[string]interface{}{
						"resources": []interface{}{"arn:aws:s3:::&{aws:username}/*"},
					},
				},
			},
			wantSummary: "[FATAL] reading statement resources (iam_policy_document):",
		},
		{
			name: "bad_not_resources",
			raw: map[string]interface{}{
				"version": "2008-10-17",
				"statement": []interface{}{
					map[string]interface{}{
						"not_resources": []interface{}{"arn:aws:s3:::&{aws:username}/*"},
					},
				},
			},
			wantSummary: "[FATAL] reading statement not_resources (iam_policy_document):",
		},
		{
			name: "bad_condition",
			raw: map[string]interface{}{
				"version": "2008-10-17",
				"statement": []interface{}{
					map[string]interface{}{
						"condition": []interface{}{
							map[string]interface{}{
								"test":     "StringEquals",
								"variable": "s3:prefix",
								"values":   []interface{}{"&{aws:username}"},
							},
						},
					},
				},
			},
			wantSummary: "[FATAL] reading statement condition (iam_policy_document):",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := schema.TestResourceDataRaw(t, ds.Schema, tc.raw)
			diags := ds.ReadContext(ctx, d, nil)
			if !diags.HasError() {
				t.Fatal("expected error diagnostics, got none")
			}
			if !strings.Contains(diags[0].Summary, tc.wantSummary) {
				t.Errorf("summary = %q, want substring %q", diags[0].Summary, tc.wantSummary)
			}
		})
	}
}
