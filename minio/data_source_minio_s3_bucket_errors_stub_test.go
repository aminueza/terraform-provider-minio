package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func newDataSourceAdminStubClient(t *testing.T, handler http.HandlerFunc) (*madmin.AdminClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}
	return admin, srv
}

func newDataSourceS3AndAdminStubClient(t *testing.T, handler http.HandlerFunc) (*S3MinioClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	creds := credentials.NewStaticV4("accesskey", "secretkey", "")

	s3Client, err := minio.New(host, &minio.Options{
		Creds:  creds,
		Secure: false,
		Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("creating S3 client: %v", err)
	}

	admin, err := madmin.NewWithOptions(host, &madmin.Options{
		Creds:  creds,
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}

	return &S3MinioClient{S3Client: s3Client, S3Admin: admin}, srv
}

func assertDataSourceErrorContains(t *testing.T, diags diag.Diagnostics, wantSummary string) {
	t.Helper()
	if !diags.HasError() {
		t.Fatalf("read succeeded (%v), want error containing %q", diags, wantSummary)
	}
	if !strings.Contains(diags[0].Summary, wantSummary) {
		t.Fatalf("error summary = %q, want it to contain %q", diags[0].Summary, wantSummary)
	}
}

func assertDataSourceS3CompatWarning(t *testing.T, diags diag.Diagnostics, wantAttr string) {
	t.Helper()
	if diags.HasError() {
		t.Fatalf("read failed: %v", diags)
	}
	if len(diags) != 1 || diags[0].Severity != diag.Warning {
		t.Fatalf("got diagnostics %v, want one warning", diags)
	}
	if !strings.Contains(diags[0].Detail, wantAttr) || !strings.Contains(diags[0].Detail, "s3_compat_mode") {
		t.Errorf("warning = %q, want it to name %q and s3_compat_mode", diags[0].Detail, wantAttr)
	}
}

func TestDataSourceMinioS3BucketQuotaReadAgainstStubBackend(t *testing.T) {
	cases := []struct {
		name           string
		compatMode     bool
		closeServer    bool
		status         int
		body           string
		wantErr        string
		wantQuota      int
		wantType       string
		wantGapWarning bool
	}{
		{
			name:      "200 with quota reports quota and type",
			status:    http.StatusOK,
			body:      `{"size":1048576,"quotatype":"hard"}`,
			wantQuota: 1048576,
			wantType:  "hard",
		},
		{
			name:   "200 with size 0 reports zero quota and empty type without error",
			status: http.StatusOK,
			body:   `{"size":0}`,
		},
		{
			name:    "404 NoSuchBucket is an error",
			status:  http.StatusNotFound,
			body:    `{"Code":"NoSuchBucket","Message":"The specified bucket does not exist"}`,
			wantErr: "reading bucket quota",
		},
		{
			name:       "403 AccessDenied is an error even with s3_compat_mode",
			compatMode: true,
			status:     http.StatusForbidden,
			body:       `{"Code":"AccessDenied","Message":"Access Denied."}`,
			wantErr:    "reading bucket quota",
		},
		{
			name:        "closed connection is an error",
			closeServer: true,
			wantErr:     "reading bucket quota",
		},
		{
			name:           "with s3_compat_mode a 501 is a warning",
			compatMode:     true,
			status:         http.StatusNotImplemented,
			body:           `{"Code":"NotImplemented","Message":"This operation is not implemented."}`,
			wantGapWarning: true,
		},
		{
			name:           "with s3_compat_mode a 405 is a warning",
			compatMode:     true,
			status:         http.StatusMethodNotAllowed,
			body:           `{"Code":"MethodNotAllowed","Message":"The specified method is not allowed against this resource."}`,
			wantGapWarning: true,
		},
		{
			name:    "without s3_compat_mode a 501 is an error",
			status:  http.StatusNotImplemented,
			body:    `{"Code":"NotImplemented","Message":"This operation is not implemented."}`,
			wantErr: "reading bucket quota",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			admin, srv := newDataSourceAdminStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/get-bucket-quota") {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})
			if tc.closeServer {
				srv.Close()
			}

			d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketQuota().Schema, map[string]interface{}{"bucket": "test-bucket"})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			diags := dataSourceMinioS3BucketQuotaRead(ctx, d, &S3MinioClient{S3Admin: admin, S3CompatMode: tc.compatMode})

			if tc.wantErr != "" {
				assertDataSourceErrorContains(t, diags, tc.wantErr)
				return
			}
			if tc.wantGapWarning {
				assertDataSourceS3CompatWarning(t, diags, "quota")
			} else if diags.HasError() || len(diags) != 0 {
				t.Fatalf("got diagnostics %v, want none", diags)
			}

			if got := d.Get("quota").(int); got != tc.wantQuota {
				t.Errorf("quota = %d, want %d", got, tc.wantQuota)
			}
			if got := d.Get("type").(string); got != tc.wantType {
				t.Errorf("type = %q, want %q", got, tc.wantType)
			}
		})
	}
}

func TestDataSourceMinioS3BucketPolicyReadAgainstStubBackend(t *testing.T) {
	const policyJSON = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::test-bucket/*"]}]}`

	cases := []struct {
		name           string
		compatMode     bool
		status         int
		body           string
		wantErr        string
		wantPolicy     string
		wantGapWarning bool
	}{
		{
			name:       "configured policy is returned",
			status:     http.StatusOK,
			body:       policyJSON,
			wantPolicy: policyJSON,
		},
		{
			name:   "bucket without policy reads as empty",
			status: http.StatusNotFound,
			body:   dataSourceBucketStubXML("NoSuchBucketPolicy", "The bucket policy does not exist"),
		},
		{
			name:    "missing bucket is an error",
			status:  http.StatusNotFound,
			body:    dataSourceBucketStubXML("NoSuchBucket", "The specified bucket does not exist"),
			wantErr: "reading bucket policy",
		},
		{
			name:       "denied read is an error even with s3_compat_mode",
			compatMode: true,
			status:     http.StatusForbidden,
			body:       dataSourceBucketStubXML("AccessDenied", "Access Denied."),
			wantErr:    "reading bucket policy",
		},
		{
			name:           "with s3_compat_mode a missing feature is a warning",
			compatMode:     true,
			status:         http.StatusNotImplemented,
			body:           dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
			wantGapWarning: true,
		},
		{
			name:    "without s3_compat_mode a missing feature is an error",
			status:  http.StatusNotImplemented,
			body:    dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
			wantErr: "reading bucket policy",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})

			d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketPolicy().Schema, map[string]interface{}{"bucket": "test-bucket"})
			diags := dataSourceMinioS3BucketPolicyRead(context.Background(), d, &S3MinioClient{S3Client: client, S3CompatMode: tc.compatMode})

			if tc.wantErr != "" {
				assertDataSourceErrorContains(t, diags, tc.wantErr)
				return
			}
			if tc.wantGapWarning {
				assertDataSourceS3CompatWarning(t, diags, "policy")
			} else if diags.HasError() || len(diags) != 0 {
				t.Fatalf("got diagnostics %v, want none", diags)
			}

			if got := d.Get("policy").(string); got != tc.wantPolicy {
				t.Errorf("policy = %q, want %q", got, tc.wantPolicy)
			}
		})
	}
}

func TestDataSourceMinioS3BucketTagsReadAgainstStubBackend(t *testing.T) {
	cases := []struct {
		name           string
		compatMode     bool
		status         int
		body           string
		wantErr        string
		wantTags       int
		wantGapWarning bool
	}{
		{
			name:     "configured tags are returned",
			status:   http.StatusOK,
			body:     `<Tagging><TagSet><Tag><Key>env</Key><Value>prod</Value></Tag></TagSet></Tagging>`,
			wantTags: 1,
		},
		{
			name:   "NoSuchTagSet reads as empty without error",
			status: http.StatusNotFound,
			body:   dataSourceBucketStubXML("NoSuchTagSet", "The TagSet does not exist"),
		},
		{
			name:    "NoSuchBucket is an error",
			status:  http.StatusNotFound,
			body:    dataSourceBucketStubXML("NoSuchBucket", "The specified bucket does not exist"),
			wantErr: "reading bucket tags",
		},
		{
			name:       "AccessDenied is an error even with s3_compat_mode",
			compatMode: true,
			status:     http.StatusForbidden,
			body:       dataSourceBucketStubXML("AccessDenied", "Access Denied."),
			wantErr:    "reading bucket tags",
		},
		{
			name:           "with s3_compat_mode a missing feature is a warning",
			compatMode:     true,
			status:         http.StatusNotImplemented,
			body:           dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
			wantGapWarning: true,
		},
		{
			name:    "without s3_compat_mode a missing feature is an error",
			status:  http.StatusNotImplemented,
			body:    dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
			wantErr: "reading bucket tags",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})

			d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketTags().Schema, map[string]interface{}{"bucket": "test-bucket"})
			diags := dataSourceMinioS3BucketTagsRead(context.Background(), d, &S3MinioClient{S3Client: client, S3CompatMode: tc.compatMode})

			if tc.wantErr != "" {
				assertDataSourceErrorContains(t, diags, tc.wantErr)
				return
			}
			if tc.wantGapWarning {
				assertDataSourceS3CompatWarning(t, diags, "tags")
			} else if diags.HasError() || len(diags) != 0 {
				t.Fatalf("got diagnostics %v, want none", diags)
			}

			tagsMap := d.Get("tags").(map[string]interface{})
			if len(tagsMap) != tc.wantTags {
				t.Errorf("got %d tags, want %d", len(tagsMap), tc.wantTags)
			}
			if tc.wantTags > 0 && tagsMap["env"] != "prod" {
				t.Errorf("tags[env] = %v, want prod", tagsMap["env"])
			}
		})
	}
}

func TestDataSourceMinioS3BucketCorsConfigReadAgainstStubBackend(t *testing.T) {
	cases := []struct {
		name           string
		compatMode     bool
		status         int
		body           string
		wantErr        string
		wantRules      int
		wantGapWarning bool
	}{
		{
			name:      "configured CORS rule is returned",
			status:    http.StatusOK,
			body:      `<CORSConfiguration><CORSRule><AllowedOrigin>https://example.com</AllowedOrigin><AllowedMethod>GET</AllowedMethod><MaxAgeSeconds>300</MaxAgeSeconds></CORSRule></CORSConfiguration>`,
			wantRules: 1,
		},
		{
			name:   "NoSuchCORSConfiguration reads as empty without error",
			status: http.StatusNotFound,
			body:   dataSourceBucketStubXML("NoSuchCORSConfiguration", "The CORS configuration does not exist"),
		},
		{
			name:    "NoSuchBucket is an error",
			status:  http.StatusNotFound,
			body:    dataSourceBucketStubXML("NoSuchBucket", "The specified bucket does not exist"),
			wantErr: "reading bucket CORS configuration",
		},
		{
			name:       "AccessDenied is an error even with s3_compat_mode",
			compatMode: true,
			status:     http.StatusForbidden,
			body:       dataSourceBucketStubXML("AccessDenied", "Access Denied."),
			wantErr:    "reading bucket CORS configuration",
		},
		{
			name:           "with s3_compat_mode a missing feature is a warning",
			compatMode:     true,
			status:         http.StatusNotImplemented,
			body:           dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
			wantGapWarning: true,
		},
		{
			name:    "without s3_compat_mode a missing feature is an error",
			status:  http.StatusNotImplemented,
			body:    dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
			wantErr: "reading bucket CORS configuration",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})

			d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketCorsConfig().Schema, map[string]interface{}{"bucket": "test-bucket"})
			diags := dataSourceMinioS3BucketCorsConfigRead(context.Background(), d, &S3MinioClient{S3Client: client, S3CompatMode: tc.compatMode})

			if tc.wantErr != "" {
				assertDataSourceErrorContains(t, diags, tc.wantErr)
				return
			}
			if tc.wantGapWarning {
				assertDataSourceS3CompatWarning(t, diags, "cors_rule")
			} else if diags.HasError() || len(diags) != 0 {
				t.Fatalf("got diagnostics %v, want none", diags)
			}

			if got := len(d.Get("cors_rule").([]interface{})); got != tc.wantRules {
				t.Errorf("got %d cors_rule entries, want %d", got, tc.wantRules)
			}
		})
	}
}

func TestDataSourceMinioS3BucketNotificationConfigReadAgainstStubBackend(t *testing.T) {
	cases := []struct {
		name           string
		compatMode     bool
		status         int
		body           string
		wantErr        string
		wantQueues     int
		wantGapWarning bool
	}{
		{
			name:       "configured notification queue without filter is returned",
			status:     http.StatusOK,
			body:       `<NotificationConfiguration><QueueConfiguration><Queue>arn:minio:sqs::primary:webhook</Queue><Event>s3:ObjectCreated:*</Event></QueueConfiguration></NotificationConfiguration>`,
			wantQueues: 1,
		},
		{
			name:   "bucket without notifications reads as empty",
			status: http.StatusOK,
			body:   `<NotificationConfiguration/>`,
		},
		{
			name:    "NoSuchBucket is an error",
			status:  http.StatusNotFound,
			body:    dataSourceBucketStubXML("NoSuchBucket", "The specified bucket does not exist"),
			wantErr: "reading bucket notification configuration",
		},
		{
			name:       "AccessDenied is an error even with s3_compat_mode",
			compatMode: true,
			status:     http.StatusForbidden,
			body:       dataSourceBucketStubXML("AccessDenied", "Access Denied."),
			wantErr:    "reading bucket notification configuration",
		},
		{
			name:           "with s3_compat_mode a missing feature is a warning",
			compatMode:     true,
			status:         http.StatusNotImplemented,
			body:           dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
			wantGapWarning: true,
		},
		{
			name:    "without s3_compat_mode a missing feature is an error",
			status:  http.StatusNotImplemented,
			body:    dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
			wantErr: "reading bucket notification configuration",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})

			d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketNotificationConfig().Schema, map[string]interface{}{"bucket": "test-bucket"})
			diags := dataSourceMinioS3BucketNotificationConfigRead(context.Background(), d, &S3MinioClient{S3Client: client, S3CompatMode: tc.compatMode})

			if tc.wantErr != "" {
				assertDataSourceErrorContains(t, diags, tc.wantErr)
				return
			}
			if tc.wantGapWarning {
				assertDataSourceS3CompatWarning(t, diags, "queue")
			} else if diags.HasError() || len(diags) != 0 {
				t.Fatalf("got diagnostics %v, want none", diags)
			}

			if got := len(d.Get("queue").([]interface{})); got != tc.wantQueues {
				t.Errorf("got %d queue entries, want %d", got, tc.wantQueues)
			}
		})
	}
}

func TestDataSourceMinioS3BucketReplicationReadAgainstStubBackend(t *testing.T) {
	const replXML = `<?xml version="1.0" encoding="UTF-8"?><ReplicationConfiguration><Role>arn:minio:replication::target-1</Role><Rule><ID>rule-1</ID><Priority>1</Priority><Status>Enabled</Status><Filter><Prefix>logs/</Prefix></Filter><Destination><Bucket>arn:minio:replication::target-1</Bucket><StorageClass>STANDARD</StorageClass></Destination></Rule></ReplicationConfiguration>`
	const targetsJSON = `[{"sourcebucket":"test-bucket","endpoint":"minio-b:9000","targetbucket":"dest-bucket","arn":"arn:minio:replication::target-1","type":"replication","secure":true,"region":"us-east-1"}]`

	cases := []struct {
		name           string
		compatMode     bool
		replStatus     int
		replBody       string
		targetsStatus  int
		targetsBody    string
		wantErr        string
		wantRules      int
		wantDestBucket string
		wantGapWarning bool
	}{
		{
			name:           "replication rules and remote targets are reported",
			replStatus:     http.StatusOK,
			replBody:       replXML,
			targetsStatus:  http.StatusOK,
			targetsBody:    targetsJSON,
			wantRules:      1,
			wantDestBucket: "dest-bucket",
		},
		{
			name:       "ReplicationConfigurationNotFoundError reads as empty without calling remote targets",
			replStatus: http.StatusNotFound,
			replBody:   dataSourceBucketStubXML("ReplicationConfigurationNotFoundError", "The replication configuration was not found"),
		},
		{
			name:       "NoSuchBucket on replication is an error",
			replStatus: http.StatusNotFound,
			replBody:   dataSourceBucketStubXML("NoSuchBucket", "The specified bucket does not exist"),
			wantErr:    "reading bucket replication configuration",
		},
		{
			name:          "ListRemoteTargets failure is an error",
			replStatus:    http.StatusOK,
			replBody:      replXML,
			targetsStatus: http.StatusForbidden,
			targetsBody:   `{"Code":"AccessDenied","Message":"Access Denied."}`,
			wantErr:       "reading bucket replication remote targets",
		},
		{
			name:           "with s3_compat_mode a missing feature is a warning",
			compatMode:     true,
			replStatus:     http.StatusNotImplemented,
			replBody:       dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
			wantGapWarning: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := newDataSourceS3AndAdminStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/list-remote-targets") {
					if tc.targetsStatus == 0 {
						t.Errorf("unexpected list-remote-targets call")
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.targetsStatus)
					_, _ = fmt.Fprint(w, tc.targetsBody)
					return
				}
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.replStatus)
				_, _ = fmt.Fprint(w, tc.replBody)
			})
			client.S3CompatMode = tc.compatMode

			d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketReplication().Schema, map[string]interface{}{"bucket": "test-bucket"})
			diags := dataSourceMinioS3BucketReplicationRead(context.Background(), d, client)

			if tc.wantErr != "" {
				assertDataSourceErrorContains(t, diags, tc.wantErr)
				return
			}
			if tc.wantGapWarning {
				assertDataSourceS3CompatWarning(t, diags, "rule")
			} else if diags.HasError() || len(diags) != 0 {
				t.Fatalf("got diagnostics %v, want none", diags)
			}

			rules := d.Get("rule").([]interface{})
			if len(rules) != tc.wantRules {
				t.Fatalf("got %d rules, want %d", len(rules), tc.wantRules)
			}
			if tc.wantDestBucket != "" {
				gotBucket := d.Get("rule.0.destination.0.bucket").(string)
				if gotBucket != tc.wantDestBucket {
					t.Errorf("destination.bucket = %q, want %q", gotBucket, tc.wantDestBucket)
				}
			}
		})
	}
}

func TestDataSourceMinioS3BucketReplicationStatusAndMetricsAgainstStubBackend(t *testing.T) {
	t.Run("replication_status", func(t *testing.T) {
		const replXML = `<?xml version="1.0" encoding="UTF-8"?><ReplicationConfiguration><Role>arn:minio:replication::target-1</Role><Rule><ID>r1</ID><Priority>1</Priority><Status>Enabled</Status><Destination><Bucket>arn:minio:replication::target-1</Bucket></Destination></Rule></ReplicationConfiguration>`
		const metricsJSON = `{"completedReplicationSize":42,"replicaSize":24,"replicationCount":2,"replicaCount":1}`

		cases := []struct {
			name           string
			compatMode     bool
			replStatus     int
			replBody       string
			metricsStatus  int
			metricsBody    string
			wantErr        string
			wantRuleCount  int
			wantGapWarning string
		}{
			{
				name:          "configured replication and metrics are reported",
				replStatus:    http.StatusOK,
				replBody:      replXML,
				metricsStatus: http.StatusOK,
				metricsBody:   metricsJSON,
				wantRuleCount: 1,
			},
			{
				name:          "unconfigured replication with zero metrics succeeds",
				replStatus:    http.StatusNotFound,
				replBody:      dataSourceBucketStubXML("ReplicationConfigurationNotFoundError", "The replication configuration was not found"),
				metricsStatus: http.StatusOK,
				metricsBody:   `{}`,
			},
			{
				name:       "NoSuchBucket on replication returns error",
				replStatus: http.StatusNotFound,
				replBody:   dataSourceBucketStubXML("NoSuchBucket", "The specified bucket does not exist"),
				wantErr:    "reading bucket replication status",
			},
			{
				name:          "AccessDenied on replication metrics returns error",
				replStatus:    http.StatusOK,
				replBody:      replXML,
				metricsStatus: http.StatusForbidden,
				metricsBody:   dataSourceBucketStubXML("AccessDenied", "Access Denied."),
				wantErr:       "reading bucket replication metrics",
			},
			{
				name:           "with s3_compat_mode a missing metrics endpoint warns",
				compatMode:     true,
				replStatus:     http.StatusNotFound,
				replBody:       dataSourceBucketStubXML("ReplicationConfigurationNotFoundError", "The replication configuration was not found"),
				metricsStatus:  http.StatusNotImplemented,
				metricsBody:    dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
				wantGapWarning: "replicated_size",
			},
			{
				name:           "with s3_compat_mode a 200 XML ListObjects fallback on metrics warns",
				compatMode:     true,
				replStatus:     http.StatusOK,
				replBody:       replXML,
				metricsStatus:  http.StatusOK,
				metricsBody:    `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>test-bucket</Name></ListBucketResult>`,
				wantRuleCount:  1,
				wantGapWarning: "replicated_size",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Query().Has("replication-metrics") {
						if strings.HasPrefix(tc.metricsBody, "<") {
							w.Header().Set("Content-Type", "application/xml")
						} else {
							w.Header().Set("Content-Type", "application/json")
						}
						w.WriteHeader(tc.metricsStatus)
						_, _ = fmt.Fprint(w, tc.metricsBody)
						return
					}
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(tc.replStatus)
					_, _ = fmt.Fprint(w, tc.replBody)
				})

				d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketReplicationStatus().Schema, map[string]interface{}{"bucket": "test-bucket"})
				diags := dataSourceMinioS3BucketReplicationStatusRead(context.Background(), d, &S3MinioClient{S3Client: client, S3CompatMode: tc.compatMode})

				if tc.wantErr != "" {
					assertDataSourceErrorContains(t, diags, tc.wantErr)
					return
				}
				if tc.wantGapWarning != "" {
					assertDataSourceS3CompatWarning(t, diags, tc.wantGapWarning)
				} else if diags.HasError() || len(diags) != 0 {
					t.Fatalf("got diagnostics %v, want none", diags)
				}

				if got := d.Get("rule_count").(int); got != tc.wantRuleCount {
					t.Errorf("rule_count = %d, want %d", got, tc.wantRuleCount)
				}
			})
		}
	})

	t.Run("replication_metrics", func(t *testing.T) {
		cases := []struct {
			name           string
			compatMode     bool
			status         int
			body           string
			wantErr        string
			wantGapWarning bool
			wantPending    string
		}{
			{
				name:        "200 returns reported metrics",
				status:      http.StatusOK,
				body:        `{"currStats":{"pendingReplicationSize":128,"completedReplicationSize":256}}`,
				wantPending: "128",
			},
			{
				name:    "NoSuchBucket returns error instead of zeros",
				status:  http.StatusNotFound,
				body:    dataSourceBucketStubXML("NoSuchBucket", "The specified bucket does not exist"),
				wantErr: "reading bucket replication metrics",
			},
			{
				name:       "AccessDenied returns error even with s3_compat_mode",
				compatMode: true,
				status:     http.StatusForbidden,
				body:       dataSourceBucketStubXML("AccessDenied", "Access Denied."),
				wantErr:    "reading bucket replication metrics",
			},
			{
				name:           "with s3_compat_mode a missing feature warns and returns zeros",
				compatMode:     true,
				status:         http.StatusNotImplemented,
				body:           dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
				wantGapWarning: true,
				wantPending:    "0",
			},
			{
				name:           "with s3_compat_mode a 200 XML ListObjects fallback warns and returns zeros",
				compatMode:     true,
				status:         http.StatusOK,
				body:           `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>test-bucket</Name></ListBucketResult>`,
				wantGapWarning: true,
				wantPending:    "0",
			},
			{
				name:    "without s3_compat_mode a 200 XML response returns an error",
				status:  http.StatusOK,
				body:    `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>test-bucket</Name></ListBucketResult>`,
				wantErr: "reading bucket replication metrics",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
					if strings.HasPrefix(tc.body, "<") {
						w.Header().Set("Content-Type", "application/xml")
					} else {
						w.Header().Set("Content-Type", "application/json")
					}
					w.WriteHeader(tc.status)
					_, _ = fmt.Fprint(w, tc.body)
				})

				d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketReplicationMetrics().Schema, map[string]interface{}{"bucket": "test-bucket"})
				diags := dataSourceMinioS3BucketReplicationMetricsRead(context.Background(), d, &S3MinioClient{S3Client: client, S3CompatMode: tc.compatMode})

				if tc.wantErr != "" {
					assertDataSourceErrorContains(t, diags, tc.wantErr)
					return
				}
				if tc.wantGapWarning {
					assertDataSourceS3CompatWarning(t, diags, "pending_size")
				} else if diags.HasError() || len(diags) != 0 {
					t.Fatalf("got diagnostics %v, want none", diags)
				}

				if got := d.Get("pending_size").(string); got != tc.wantPending {
					t.Errorf("pending_size = %q, want %q", got, tc.wantPending)
				}
			})
		}
	})
}

func TestDataSourceMinioIAMGroupErrorsAgainstStubBackend(t *testing.T) {
	t.Run("iam_groups returns error when GetGroupDescription is denied", func(t *testing.T) {
		admin, _ := newDataSourceAdminStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/groups"):
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `["devs"]`)
			case strings.HasSuffix(r.URL.Path, "/group"):
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"Code":"AccessDenied","Message":"Access Denied."}`)
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotImplemented)
			}
		})

		d := schema.TestResourceDataRaw(t, dataSourceIAMGroups().Schema, map[string]interface{}{})
		diags := dataSourceIAMGroupsRead(context.Background(), d, &S3MinioClient{S3Admin: admin})
		assertDataSourceErrorContains(t, diags, "reading IAM group")
	})

	t.Run("iam_user_policies returns error when GetGroupDescription is denied", func(t *testing.T) {
		admin, _ := newDataSourceAdminStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/user-info"):
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `{"status":"enabled","policyName":"direct-ro","memberOf":["devs"]}`)
			case strings.HasSuffix(r.URL.Path, "/group"):
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"Code":"AccessDenied","Message":"Access Denied."}`)
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotImplemented)
			}
		})

		d := schema.TestResourceDataRaw(t, dataSourceIAMUserPolicies().Schema, map[string]interface{}{"name": "alice"})
		diags := dataSourceIAMUserPoliciesRead(context.Background(), d, &S3MinioClient{S3Admin: admin})
		assertDataSourceErrorContains(t, diags, "reading IAM group")
	})
}
