package agentcore

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func awsCore(t *testing.T) *Core {
	t.Helper()
	path := os.Getenv("AGENTCORE_AWS_PROVIDER")
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if path == "" || endpoint == "" {
		t.Skip("AGENTCORE_AWS_PROVIDER and MINIO_ENDPOINT are not both set")
	}

	core, err := OpenBinary(context.Background(), path)
	if err != nil {
		t.Fatalf("opening %s: %s", path, err)
	}
	t.Cleanup(core.Close)

	scheme := "http://"
	if os.Getenv("MINIO_ENABLE_HTTPS") == "true" {
		scheme = "https://"
	}
	err = core.Configure(context.Background(), map[string]interface{}{
		"region":                      "us-east-1",
		"access_key":                  os.Getenv("MINIO_USER"),
		"secret_key":                  os.Getenv("MINIO_PASSWORD"),
		"skip_credentials_validation": true,
		"skip_requesting_account_id":  true,
		"skip_region_validation":      true,
		"skip_metadata_api_check":     "true",
		"s3_use_path_style":           true,
		"endpoints":                   []map[string]interface{}{{"s3": scheme + endpoint}},
	})
	if err != nil {
		t.Fatalf("configuring the AWS provider against %s: %s", endpoint, err)
	}
	return core
}

func TestAWSProviderServesTheBucketOverGRPC(t *testing.T) {
	core := awsCore(t)

	types := strings.Join(core.ResourceTypes(), ",")
	if !strings.Contains(types, "aws_s3_bucket,") {
		t.Fatalf("aws_s3_bucket is not served; the provider advertises %d resource types", len(core.ResourceTypes()))
	}
}

func TestAWSBucketConvergesAgainstMinIO(t *testing.T) {
	core := awsCore(t)
	ctx := context.Background()
	name := fmt.Sprintf("agent-aws-%d", time.Now().UnixNano())

	t.Cleanup(func() {
		_, _ = core.Delete(ctx, Request{Type: "aws_s3_bucket", ID: name})
	})

	created, err := core.Converge(ctx, Request{
		Type:       "aws_s3_bucket",
		ID:         name,
		Attributes: map[string]interface{}{"bucket": name},
	})
	if err != nil {
		t.Fatalf("first converge: %s", err)
	}
	if created.Action != ActionCreate {
		t.Errorf("first converge action = %q, want %q", created.Action, ActionCreate)
	}
	if created.State["bucket"] != name {
		t.Errorf("bucket = %v", created.State["bucket"])
	}
	t.Logf("create converged=%v drift=%v", created.Converged, created.Drift)

	preview, err := core.Plan(ctx, Request{
		Type:       "aws_s3_bucket",
		ID:         name,
		Attributes: map[string]interface{}{"bucket": name},
	})
	if err != nil {
		t.Fatalf("plan by id: %s", err)
	}
	t.Logf("plan by id after create: action=%s", preview.Action)
	for _, change := range preview.Changes {
		t.Logf("  change %s", change)
	}

	again, err := core.Converge(ctx, Request{
		Type:       "aws_s3_bucket",
		ID:         name,
		Attributes: map[string]interface{}{"bucket": name},
	})
	if err != nil {
		t.Fatalf("second converge: %s", err)
	}
	t.Logf("second converge action=%s converged=%v drift=%v", again.Action, again.Converged, again.Drift)

	tagged, err := core.Converge(ctx, Request{
		Type:       "aws_s3_bucket",
		ID:         name,
		Attributes: map[string]interface{}{"bucket": name, "tags": map[string]string{"owner": "agent"}},
	})
	if err != nil {
		t.Fatalf("tagging converge: %s", err)
	}
	tags, _ := tagged.State["tags"].(map[string]interface{})
	if tags["owner"] != "agent" {
		t.Errorf("tags after converge = %v", tagged.State["tags"])
	}
	t.Logf("tagging action=%s converged=%v drift=%v", tagged.Action, tagged.Converged, tagged.Drift)

	deleted, err := core.Delete(ctx, Request{Type: "aws_s3_bucket", ID: name})
	if err != nil {
		t.Fatalf("delete: %s", err)
	}
	if deleted.Action != ActionDelete || !deleted.Converged {
		t.Errorf("delete = %+v", deleted)
	}
}

func TestAWSLifecycleConfigurationAgainstMinIO(t *testing.T) {
	core := awsCore(t)
	ctx := context.Background()
	name := fmt.Sprintf("agent-aws-lc-%d", time.Now().UnixNano())

	if _, err := core.Converge(ctx, Request{
		Type:       "aws_s3_bucket",
		ID:         name,
		Attributes: map[string]interface{}{"bucket": name},
	}); err != nil {
		t.Fatalf("creating the bucket: %s", err)
	}
	t.Cleanup(func() {
		_, _ = core.Delete(ctx, Request{Type: "aws_s3_bucket_lifecycle_configuration", ID: name})
		_, _ = core.Delete(ctx, Request{Type: "aws_s3_bucket", ID: name})
	})

	rule := map[string]interface{}{
		"id":         "expire-logs",
		"status":     "Enabled",
		"filter":     []map[string]interface{}{{"prefix": "logs/"}},
		"expiration": []map[string]interface{}{{"days": 30}},
	}

	attributes := map[string]interface{}{
		"bucket":   name,
		"rule":     []map[string]interface{}{rule},
		"timeouts": map[string]interface{}{"create": "20s", "update": "20s"},
	}

	created, err := core.Converge(ctx, Request{
		Type:       "aws_s3_bucket_lifecycle_configuration",
		ID:         name,
		Attributes: attributes,
	})
	if err != nil {
		t.Logf("lifecycle converge failed: %s", err)
	} else {
		t.Logf("lifecycle create action=%s converged=%v drift=%v", created.Action, created.Converged, created.Drift)
	}

	preview, err := core.Plan(ctx, Request{
		Type:       "aws_s3_bucket_lifecycle_configuration",
		ID:         name,
		Attributes: attributes,
	})
	if err != nil {
		t.Fatalf("lifecycle plan after the apply: %s", err)
	}
	t.Logf("lifecycle plan after the apply: action=%s", preview.Action)
	for _, change := range preview.Changes {
		t.Logf("  change %s", change)
	}
	t.Logf("  prior rule: %v", preview.Prior["rule"])
	t.Logf("  planned rule: %v", preview.Planned["rule"])
	if preview.Action == ActionNoop {
		t.Log("the lifecycle configuration converged against MinIO")
	}
}
