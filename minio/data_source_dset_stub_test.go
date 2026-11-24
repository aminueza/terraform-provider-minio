package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/replication"
)

func TestDataSourceDSetHelpers(t *testing.T) {
	d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketVersioning().Schema, map[string]interface{}{"bucket": "test-bucket"})
	if diags := setResourceField(d, "enabled", true, "test-bucket"); diags.HasError() {
		t.Fatalf("expected nil diags for valid data source field, got %v", diags)
	}
	if !d.Get("enabled").(bool) {
		t.Fatal("expected enabled = true")
	}

	diags := setResourceField(d, "nonexistent_field", "val", "test-bucket")
	if !diags.HasError() {
		t.Fatal("expected error diagnostic when setting nonexistent schema key")
	}
	if msg := fmt.Sprint(diags); !strings.Contains(msg, ErrorSeverityFatal) || !strings.Contains(msg, "setting nonexistent_field (test-bucket)") {
		t.Fatalf("unexpected diagnostic message: %q", msg)
	}

	metricsData := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketReplicationMetrics().Schema, map[string]interface{}{"bucket": "metrics-bucket"})
	if zeroDiags := setBucketReplicationMetricsZero(metricsData, "metrics-bucket"); zeroDiags.HasError() {
		t.Fatalf("setBucketReplicationMetricsZero failed: %v", zeroDiags)
	}
	for _, key := range []string{
		"pending_size", "pending_count",
		"failed_size", "failed_count",
		"replicated_size", "replicated_count",
		"replica_size", "replica_count",
		"queued_size", "queued_count",
	} {
		if got := metricsData.Get(key).(string); got != "0" {
			t.Errorf("%s = %q, want %q", key, got, "0")
		}
	}
}

func TestDataSourceS3DSetPopulationAgainstStubBackend(t *testing.T) {
	t.Run("s3_buckets filters by name_prefix", func(t *testing.T) {
		client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Owner><ID>owner</ID><DisplayName>owner</DisplayName></Owner>
  <Buckets>
    <Bucket><Name>app-logs</Name><CreationDate>2026-01-01T00:00:00Z</CreationDate></Bucket>
    <Bucket><Name>other-data</Name><CreationDate>2026-01-02T00:00:00Z</CreationDate></Bucket>
  </Buckets>
</ListAllMyBucketsResult>`)
		})

		d := schema.TestResourceDataRaw(t, dataSourceMinioS3Buckets().Schema, map[string]interface{}{"name_prefix": "app-"})
		if diags := dataSourceMinioS3BucketsRead(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
			t.Fatalf("dataSourceMinioS3BucketsRead failed: %v", diags)
		}
		buckets := d.Get("buckets").([]interface{})
		if len(buckets) != 1 {
			t.Fatalf("got %d buckets, want 1", len(buckets))
		}
		entry := buckets[0].(map[string]interface{})
		if entry["name"] != "app-logs" || entry["creation_date"] != "2026-01-01T00:00:00Z" {
			t.Errorf("unexpected bucket entry: %+v", entry)
		}
	})

	t.Run("s3_objects populates keys and common_prefixes", func(t *testing.T) {
		client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>my-bucket</Name>
  <Prefix>logs/</Prefix>
  <Delimiter>/</Delimiter>
  <MaxKeys>100</MaxKeys>
  <IsTruncated>false</IsTruncated>
  <Contents><Key>logs/app.log</Key><Size>12</Size><ETag>"abc"</ETag></Contents>
  <CommonPrefixes><Prefix>logs/2026/</Prefix></CommonPrefixes>
</ListBucketResult>`)
		})

		d := schema.TestResourceDataRaw(t, dataSourceMinioS3Objects().Schema, map[string]interface{}{
			"bucket":    "my-bucket",
			"prefix":    "logs/",
			"delimiter": "/",
		})
		if diags := dataSourceMinioS3ObjectsRead(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
			t.Fatalf("dataSourceMinioS3ObjectsRead failed: %v", diags)
		}
		if got := toStringSlice(d.Get("keys")); !slices.Equal(got, []string{"logs/app.log"}) {
			t.Errorf("keys = %v, want [logs/app.log]", got)
		}
		if got := toStringSlice(d.Get("common_prefixes")); !slices.Equal(got, []string{"logs/2026/"}) {
			t.Errorf("common_prefixes = %v, want [logs/2026/]", got)
		}
	})

	t.Run("s3_bucket_versioning populates enabled and suspended", func(t *testing.T) {
		for _, tc := range []struct {
			status        string
			wantEnabled   bool
			wantSuspended bool
		}{
			{status: "Enabled", wantEnabled: true, wantSuspended: false},
			{status: "Suspended", wantEnabled: false, wantSuspended: true},
		} {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				_, _ = fmt.Fprintf(w, `<VersioningConfiguration><Status>%s</Status></VersioningConfiguration>`, tc.status)
			})
			d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketVersioning().Schema, map[string]interface{}{"bucket": "ver-bucket"})
			if diags := dataSourceMinioS3BucketVersioningRead(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
				t.Fatalf("dataSourceMinioS3BucketVersioningRead(%s) failed: %v", tc.status, diags)
			}
			if d.Get("enabled").(bool) != tc.wantEnabled || d.Get("suspended").(bool) != tc.wantSuspended {
				t.Errorf("%s: got enabled=%v suspended=%v, want %v/%v",
					tc.status, d.Get("enabled"), d.Get("suspended"), tc.wantEnabled, tc.wantSuspended)
			}
		}
	})

	t.Run("s3_bucket_retention populates config and empty fallback", func(t *testing.T) {
		configured := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ObjectLockEnabled>Enabled</ObjectLockEnabled><Rule><DefaultRetention><Mode>COMPLIANCE</Mode><Days>30</Days></DefaultRetention></Rule></ObjectLockConfiguration>`)
		})
		d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketRetention().Schema, map[string]interface{}{"bucket": "worm-bucket"})
		if diags := dataSourceMinioS3BucketRetentionRead(context.Background(), d, &S3MinioClient{S3Client: configured}); diags.HasError() {
			t.Fatalf("dataSourceMinioS3BucketRetentionRead failed: %v", diags)
		}
		if d.Get("mode") != "COMPLIANCE" || d.Get("unit") != "DAYS" || d.Get("validity_period") != 30 {
			t.Errorf("got mode=%v unit=%v validity=%v", d.Get("mode"), d.Get("unit"), d.Get("validity_period"))
		}

		unconfigured := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, dataSourceBucketStubXML("ObjectLockConfigurationNotFoundError", "Object Lock configuration does not exist for this bucket"))
		})
		dEmpty := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketRetention().Schema, map[string]interface{}{"bucket": "plain-bucket"})
		if diags := dataSourceMinioS3BucketRetentionRead(context.Background(), dEmpty, &S3MinioClient{S3Client: unconfigured}); diags.HasError() {
			t.Fatalf("dataSourceMinioS3BucketRetentionRead unconfigured failed: %v", diags)
		}
		if dEmpty.Get("mode") != "" || dEmpty.Get("unit") != "" || dEmpty.Get("validity_period") != 0 {
			t.Errorf("unconfigured got mode=%v unit=%v validity=%v", dEmpty.Get("mode"), dEmpty.Get("unit"), dEmpty.Get("validity_period"))
		}
	})

	t.Run("s3_bucket_object_lock_configuration populates days, years, and empty fallback", func(t *testing.T) {
		yearsClient := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ObjectLockEnabled>Enabled</ObjectLockEnabled><Rule><DefaultRetention><Mode>GOVERNANCE</Mode><Years>5</Years></DefaultRetention></Rule></ObjectLockConfiguration>`)
		})
		dYears := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketObjectLockConfiguration().Schema, map[string]interface{}{"bucket": "lock-years"})
		if diags := dataSourceMinioS3BucketObjectLockConfigurationRead(context.Background(), dYears, &S3MinioClient{S3Client: yearsClient}); diags.HasError() {
			t.Fatalf("object lock years read failed: %v", diags)
		}
		if dYears.Get("object_lock_enabled") != "Enabled" {
			t.Errorf("object_lock_enabled = %v, want Enabled", dYears.Get("object_lock_enabled"))
		}
		rules := dYears.Get("rule").([]interface{})
		if len(rules) != 1 {
			t.Fatalf("rule len = %d, want 1", len(rules))
		}
		dr := rules[0].(map[string]interface{})["default_retention"].([]interface{})[0].(map[string]interface{})
		if dr["mode"] != "GOVERNANCE" || dr["years"] != 5 {
			t.Errorf("default_retention = %+v", dr)
		}

		notFoundClient := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, dataSourceBucketStubXML("ObjectLockConfigurationNotFoundError", "Object Lock configuration does not exist"))
		})
		dNone := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketObjectLockConfiguration().Schema, map[string]interface{}{"bucket": "lock-none"})
		if diags := dataSourceMinioS3BucketObjectLockConfigurationRead(context.Background(), dNone, &S3MinioClient{S3Client: notFoundClient}); diags.HasError() {
			t.Fatalf("object lock notfound read failed: %v", diags)
		}
		if dNone.Get("object_lock_enabled") != "" || len(dNone.Get("rule").([]interface{})) != 0 {
			t.Errorf("notfound state = enabled=%v rules=%v", dNone.Get("object_lock_enabled"), dNone.Get("rule"))
		}
	})

	t.Run("s3_bucket_replication_metrics populates all fields", func(t *testing.T) {
		client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(replication.MetricsV2{
				CurrentStats: replication.Metrics{
					PendingSize:     10,
					PendingCount:    2,
					FailedSize:      20,
					FailedCount:     3,
					ReplicatedSize:  30,
					ReplicatedCount: 4,
					ReplicaSize:     40,
					ReplicaCount:    5,
					QStats: replication.InQueueMetric{
						Curr: replication.QStat{Bytes: 512, Count: 6},
					},
				},
			})
		})
		d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketReplicationMetrics().Schema, map[string]interface{}{"bucket": "repl-bucket"})
		if diags := dataSourceMinioS3BucketReplicationMetricsRead(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
			t.Fatalf("replication metrics read failed: %v", diags)
		}
		want := map[string]string{
			"pending_size":     "10",
			"pending_count":    "2",
			"failed_size":      "20",
			"failed_count":     "3",
			"replicated_size":  "30",
			"replicated_count": "4",
			"replica_size":     "40",
			"replica_count":    "5",
			"queued_size":      "512",
			"queued_count":     "6",
		}
		for k, expected := range want {
			if got := d.Get(k).(string); got != expected {
				t.Errorf("%s = %q, want %q", k, got, expected)
			}
		}
	})

	t.Run("s3_bucket_replication_status populates rules, metrics, and compat warning", func(t *testing.T) {
		client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Has("replication-metrics") {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"completedReplicationSize":42,"replicaSize":24,"replicationCount":2,"replicaCount":1}`)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><ReplicationConfiguration><Role>arn:minio:replication::target-1</Role><Rule><ID>r1</ID><Priority>7</Priority><Status>Enabled</Status><Destination><Bucket>arn:minio:replication::target-1</Bucket></Destination></Rule></ReplicationConfiguration>`)
		})
		d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketReplicationStatus().Schema, map[string]interface{}{"bucket": "repl-status"})
		if diags := dataSourceMinioS3BucketReplicationStatusRead(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
			t.Fatalf("replication status read failed: %v", diags)
		}
		if d.Get("rule_count") != 1 || d.Get("replicated_size") != "42" || d.Get("replica_size") != "24" || d.Get("replicated_count") != 2 || d.Get("replica_count") != 1 {
			t.Errorf("unexpected metrics/rule_count: rule_count=%v replicated=%v/%v replica=%v/%v",
				d.Get("rule_count"), d.Get("replicated_size"), d.Get("replicated_count"), d.Get("replica_size"), d.Get("replica_count"))
		}
		rules := d.Get("rules").([]interface{})
		if len(rules) != 1 || rules[0].(map[string]interface{})["id"] != "r1" || rules[0].(map[string]interface{})["priority"] != 7 {
			t.Errorf("unexpected rules: %+v", rules)
		}

		compatClient := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = fmt.Fprint(w, dataSourceBucketStubXML("MethodNotAllowed", "Method Not Allowed"))
		})
		dCompat := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketReplicationStatus().Schema, map[string]interface{}{"bucket": "repl-compat"})
		compatDiags := dataSourceMinioS3BucketReplicationStatusRead(context.Background(), dCompat, &S3MinioClient{S3Client: compatClient, S3CompatMode: true})
		assertDataSourceS3CompatWarning(t, compatDiags, "rule_count")
		if dCompat.Get("rule_count") != 0 {
			t.Errorf("compat rule_count = %v, want 0", dCompat.Get("rule_count"))
		}
	})
}

func TestDataSourceAdminAndIAMDSetPopulationAgainstStubBackend(t *testing.T) {
	t.Run("iam_policy_document sets json and id", func(t *testing.T) {
		d := schema.TestResourceDataRaw(t, dataSourceMinioIAMPolicyDocument().Schema, map[string]interface{}{
			"statement": []interface{}{
				map[string]interface{}{
					"sid":       "AllowRead",
					"effect":    "Allow",
					"actions":   []interface{}{"s3:GetObject"},
					"resources": []interface{}{"arn:aws:s3:::demo/*"},
				},
			},
		})
		if diags := dataSourceMinioIAMPolicyDocumentRead(context.Background(), d, nil); diags.HasError() {
			t.Fatalf("dataSourceMinioIAMPolicyDocumentRead failed: %v", diags)
		}
		if d.Id() == "" || !strings.Contains(d.Get("json").(string), `"s3:GetObject"`) {
			t.Errorf("unexpected policy document id=%q json=%q", d.Id(), d.Get("json"))
		}
	})

	t.Run("iam_user, iam_group, iam_groups, iam_policy, iam_service_accounts, iam_user_policies", func(t *testing.T) {
		admin, _ := newDataSourceAdminStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/user-info"):
				_, _ = fmt.Fprint(w, `{"status":"enabled","policyName":"direct-ro, common-read","memberOf":["devs"]}`)
			case strings.HasSuffix(r.URL.Path, "/groups"):
				_, _ = fmt.Fprint(w, `["devs","ops"]`)
			case strings.HasSuffix(r.URL.Path, "/group"):
				_ = json.NewEncoder(w).Encode(madmin.GroupDesc{
					Name:    "devs",
					Status:  "enabled",
					Members: []string{"alice"},
					Policy:  "group-rw, common-read",
				})
			case strings.HasSuffix(r.URL.Path, "/info-canned-policy"):
				_ = json.NewEncoder(w).Encode(madmin.PolicyInfo{
					PolicyName: "read-only",
					Policy:     json.RawMessage(`{"Version":"2012-10-17"}`),
				})
			case strings.HasSuffix(r.URL.Path, "/list-service-accounts"):
				plain, err := json.Marshal(madmin.ListServiceAccountsResp{
					Accounts: []madmin.ServiceAccountInfo{{AccessKey: "svc-key-1"}},
				})
				if err != nil {
					t.Fatal(err)
				}
				enc, err := madmin.EncryptData("secretkey", plain)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = w.Write(enc)
			default:
				http.NotFound(w, r)
			}
		})
		meta := &S3MinioClient{S3Admin: admin}

		userData := schema.TestResourceDataRaw(t, dataSourceIAMUser().Schema, map[string]interface{}{"name": "alice"})
		if diags := dataSourceIAMUserRead(context.Background(), userData, meta); diags.HasError() {
			t.Fatalf("dataSourceIAMUserRead failed: %v", diags)
		}
		if userData.Get("status") != "enabled" ||
			!slices.Equal(toStringSlice(userData.Get("policy_names")), []string{"direct-ro", " common-read"}) ||
			!slices.Equal(toStringSlice(userData.Get("member_of_groups")), []string{"devs"}) {
			t.Errorf("unexpected iam_user state: status=%v policies=%v groups=%v",
				userData.Get("status"), userData.Get("policy_names"), userData.Get("member_of_groups"))
		}

		groupData := schema.TestResourceDataRaw(t, dataSourceIAMGroup().Schema, map[string]interface{}{"name": "devs"})
		if diags := dataSourceIAMGroupRead(context.Background(), groupData, meta); diags.HasError() {
			t.Fatalf("dataSourceIAMGroupRead failed: %v", diags)
		}
		if groupData.Get("status") != "enabled" || groupData.Get("policy") != "group-rw, common-read" ||
			!slices.Equal(toStringSlice(groupData.Get("members")), []string{"alice"}) {
			t.Errorf("unexpected iam_group state: status=%v policy=%v members=%v",
				groupData.Get("status"), groupData.Get("policy"), groupData.Get("members"))
		}

		groupsData := schema.TestResourceDataRaw(t, dataSourceIAMGroups().Schema, map[string]interface{}{"name_prefix": "dev"})
		if diags := dataSourceIAMGroupsRead(context.Background(), groupsData, meta); diags.HasError() {
			t.Fatalf("dataSourceIAMGroupsRead failed: %v", diags)
		}
		groupsList := groupsData.Get("groups").([]interface{})
		if len(groupsList) != 1 || groupsList[0].(map[string]interface{})["name"] != "devs" {
			t.Errorf("unexpected iam_groups list: %+v", groupsList)
		}

		policyData := schema.TestResourceDataRaw(t, dataSourceIAMPolicy().Schema, map[string]interface{}{"name": "read-only"})
		if diags := dataSourceIAMPolicyRead(context.Background(), policyData, meta); diags.HasError() {
			t.Fatalf("dataSourceIAMPolicyRead failed: %v", diags)
		}
		if !strings.Contains(policyData.Get("policy").(string), "2012-10-17") {
			t.Errorf("unexpected iam_policy: %v", policyData.Get("policy"))
		}

		svcData := schema.TestResourceDataRaw(t, dataSourceIAMServiceAccounts().Schema, map[string]interface{}{"user": "alice"})
		if diags := dataSourceIAMServiceAccountsRead(context.Background(), svcData, meta); diags.HasError() {
			t.Fatalf("dataSourceIAMServiceAccountsRead failed: %v", diags)
		}
		svcs := svcData.Get("service_accounts").([]interface{})
		if len(svcs) != 1 || svcs[0].(map[string]interface{})["access_key"] != "svc-key-1" {
			t.Errorf("unexpected iam_service_accounts: %+v", svcs)
		}

		upData := schema.TestResourceDataRaw(t, dataSourceIAMUserPolicies().Schema, map[string]interface{}{"name": "alice"})
		if diags := dataSourceIAMUserPoliciesRead(context.Background(), upData, meta); diags.HasError() {
			t.Fatalf("dataSourceIAMUserPoliciesRead failed: %v", diags)
		}
		if !slices.Equal(toStringSlice(upData.Get("direct_policies")), []string{"direct-ro", "common-read"}) {
			t.Errorf("direct_policies = %v", upData.Get("direct_policies"))
		}
		allPols := toStringSlice(upData.Get("all_policies"))
		slices.Sort(allPols)
		if !slices.Equal(allPols, []string{"common-read", "direct-ro", "group-rw"}) {
			t.Errorf("all_policies = %v, want [common-read direct-ro group-rw]", allPols)
		}
	})

	t.Run("account_info, data_usage, ilm_tiers, ilm_tier_stats, server_info, storage_info", func(t *testing.T) {
		fixedTime := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
		admin, _ := newDataSourceAdminStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/accountinfo"):
				_ = json.NewEncoder(w).Encode(madmin.AccountInfo{
					AccountName: "svc-user",
					Buckets: []madmin.BucketAccessInfo{
						{Name: "b1", Size: 100, Objects: 2},
						{Name: "b2", Size: 250, Objects: 5},
					},
				})
			case strings.HasSuffix(r.URL.Path, "/datausageinfo"):
				_ = json.NewEncoder(w).Encode(madmin.DataUsageInfo{
					LastUpdate:        fixedTime,
					ObjectsTotalCount: 9,
					ObjectsTotalSize:  900,
					BucketsUsage: map[string]madmin.BucketUsageInfo{
						"b1": {Size: 300},
					},
				})
			case strings.HasSuffix(r.URL.Path, "/tier"):
				_, _ = fmt.Fprint(w, `[{"version":"v1","type":"minio","name":"cold","minio":{"endpoint":"http://remote:9000","bucket":"cold-bucket","prefix":"p/","region":"us-east-1"}}]`)
			case strings.HasSuffix(r.URL.Path, "/tier-stats"):
				_ = json.NewEncoder(w).Encode([]madmin.TierInfo{
					{
						Name: "cold",
						Type: "minio",
						Stats: madmin.TierStats{
							TotalSize:   1024,
							NumObjects:  5,
							NumVersions: 8,
						},
					},
				})
			case strings.HasSuffix(r.URL.Path, "/info"):
				_ = json.NewEncoder(w).Encode(madmin.InfoMessage{
					DeploymentID: "dep-123",
					SQSARN:       []string{"arn:minio:sqs::1:webhook"},
					Servers: []madmin.ServerProperties{
						{
							State:    "online",
							Endpoint: "127.0.0.1:9000",
							Uptime:   60,
							Version:  "2026-01-01T00:00:00Z",
							CommitID: "deadbeef",
							Edition:  "aistor",
							Network:  map[string]string{"127.0.0.1:9000": "online"},
							Disks: []madmin.Disk{
								{Endpoint: "/data1", State: "ok", UUID: "uuid-1", TotalSpace: 1000, UsedSpace: 400, AvailableSpace: 600},
							},
						},
					},
				})
			case strings.HasSuffix(r.URL.Path, "/storageinfo"):
				_ = json.NewEncoder(w).Encode(madmin.StorageInfo{
					Disks: []madmin.Disk{
						{Endpoint: "node1", DrivePath: "/data1", State: "ok", TotalSpace: 1000, UsedSpace: 400, AvailableSpace: 600},
						{Endpoint: "node1", DrivePath: "/data2", State: "offline", TotalSpace: 500, UsedSpace: 100, AvailableSpace: 400, Healing: true},
					},
				})
			default:
				http.NotFound(w, r)
			}
		})
		meta := &S3MinioClient{S3Admin: admin}

		accData := schema.TestResourceDataRaw(t, dataSourceMinioAccountInfo().Schema, map[string]interface{}{})
		if diags := dataSourceMinioAccountInfoRead(context.Background(), accData, meta); diags.HasError() {
			t.Fatalf("dataSourceMinioAccountInfoRead failed: %v", diags)
		}
		if accData.Get("account_name") != "svc-user" || accData.Get("bucket_count") != 2 ||
			accData.Get("total_size") != "350" || accData.Get("total_objects") != "7" {
			t.Errorf("unexpected account_info: name=%v count=%v size=%v objects=%v",
				accData.Get("account_name"), accData.Get("bucket_count"), accData.Get("total_size"), accData.Get("total_objects"))
		}

		usageData := schema.TestResourceDataRaw(t, dataSourceMinioDataUsage().Schema, map[string]interface{}{})
		if diags := dataSourceMinioDataUsageRead(context.Background(), usageData, meta); diags.HasError() {
			t.Fatalf("dataSourceMinioDataUsageRead failed: %v", diags)
		}
		if usageData.Get("last_update") != fixedTime.Format(time.RFC3339) ||
			usageData.Get("total_objects") != "9" || usageData.Get("total_size") != "900" ||
			usageData.Get("buckets_count") != 1 || usageData.Get("buckets_usage").(map[string]interface{})["b1"] != "300" {
			t.Errorf("unexpected data_usage: %v %v %v %v",
				usageData.Get("last_update"), usageData.Get("total_objects"), usageData.Get("total_size"), usageData.Get("buckets_usage"))
		}

		tiersData := schema.TestResourceDataRaw(t, dataSourceMinioILMTiers().Schema, map[string]interface{}{})
		if diags := dataSourceMinioILMTiersRead(context.Background(), tiersData, meta); diags.HasError() {
			t.Fatalf("dataSourceMinioILMTiersRead failed: %v", diags)
		}
		tiers := tiersData.Get("tiers").([]interface{})
		if len(tiers) != 1 || tiers[0].(map[string]interface{})["name"] != "cold" || tiers[0].(map[string]interface{})["bucket"] != "cold-bucket" {
			t.Errorf("unexpected ilm_tiers: %+v", tiers)
		}

		statsData := schema.TestResourceDataRaw(t, dataSourceMinioILMTierStats().Schema, map[string]interface{}{})
		if diags := dataSourceMinioILMTierStatsRead(context.Background(), statsData, meta); diags.HasError() {
			t.Fatalf("dataSourceMinioILMTierStatsRead failed: %v", diags)
		}
		stats := statsData.Get("tiers").([]interface{})
		if len(stats) != 1 || stats[0].(map[string]interface{})["total_size"] != "1024" || stats[0].(map[string]interface{})["num_objects"] != 5 {
			t.Errorf("unexpected ilm_tier_stats: %+v", stats)
		}

		srvData := schema.TestResourceDataRaw(t, dataSourceMinioServerInfo().Schema, map[string]interface{}{})
		if diags := dataSourceMinioServerInfoRead(context.Background(), srvData, meta); diags.HasError() {
			t.Fatalf("dataSourceMinioServerInfoRead failed: %v", diags)
		}
		if srvData.Get("version") != "2026-01-01T00:00:00Z" || srvData.Get("commit") != "deadbeef" ||
			srvData.Get("edition") != aistorEdition || srvData.Get("region") != "us-east-1" ||
			srvData.Get("deployment_id") != "dep-123" ||
			!slices.Equal(toStringSlice(srvData.Get("sqsarn")), []string{"arn:minio:sqs::1:webhook"}) {
			t.Errorf("unexpected server_info: version=%v commit=%v edition=%v region=%v deployment_id=%v sqsarn=%v",
				srvData.Get("version"), srvData.Get("commit"), srvData.Get("edition"), srvData.Get("region"), srvData.Get("deployment_id"), srvData.Get("sqsarn"))
		}

		stoData := schema.TestResourceDataRaw(t, dataSourceMinioStorageInfo().Schema, map[string]interface{}{})
		if diags := dataSourceMinioStorageInfoRead(context.Background(), stoData, meta); diags.HasError() {
			t.Fatalf("dataSourceMinioStorageInfoRead failed: %v", diags)
		}
		if stoData.Get("disk_count") != 2 || stoData.Get("online_disks") != 1 || stoData.Get("offline_disks") != 1 ||
			stoData.Get("total_space") != "1500" || stoData.Get("used_space") != "500" || stoData.Get("available_space") != "1000" {
			t.Errorf("unexpected storage_info: count=%v online=%v offline=%v total=%v used=%v avail=%v",
				stoData.Get("disk_count"), stoData.Get("online_disks"), stoData.Get("offline_disks"),
				stoData.Get("total_space"), stoData.Get("used_space"), stoData.Get("available_space"))
		}
	})
}
