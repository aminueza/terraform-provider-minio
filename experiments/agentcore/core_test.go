package agentcore

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func openCore(t *testing.T) *Core {
	t.Helper()
	core, err := Open(context.Background())
	if err != nil {
		t.Fatalf("opening the core: %s", err)
	}
	return core
}

func TestOpenServesTheBucketResource(t *testing.T) {
	core := openCore(t)

	found := false
	for _, name := range core.ResourceTypes() {
		if name == "minio_s3_bucket" {
			found = true
		}
	}
	if !found {
		t.Fatalf("minio_s3_bucket is not served; the core advertises %v", core.ResourceTypes())
	}
}

func TestPlanWithoutAServerMarksComputedAttributesUnknown(t *testing.T) {
	core := openCore(t)

	preview, err := core.Plan(context.Background(), Request{
		Type:       "minio_s3_bucket",
		Attributes: map[string]interface{}{"bucket": "agent-core"},
	})
	if err != nil {
		t.Fatalf("planning: %s", err)
	}

	if preview.Action != ActionCreate {
		t.Errorf("action = %q, want %q for a resource with no prior state", preview.Action, ActionCreate)
	}
	if preview.Planned["bucket"] != "agent-core" {
		t.Errorf("planned bucket = %v, want the requested name", preview.Planned["bucket"])
	}
	if preview.Planned["acl"] != "private" {
		t.Errorf("planned acl = %v, want the schema default applied by the provider", preview.Planned["acl"])
	}
	if preview.Planned["force_destroy"] != false {
		t.Errorf("planned force_destroy = %v, want the schema default applied by the provider", preview.Planned["force_destroy"])
	}
	for _, computed := range []string{"arn", "bucket_domain_name", "id"} {
		if preview.Planned[computed] != "(known after apply)" {
			t.Errorf("planned %s = %v, want it unknown before apply", computed, preview.Planned[computed])
		}
	}
}

func TestPlanRejectsAttributesOutsideTheSchema(t *testing.T) {
	core := openCore(t)

	_, err := core.Plan(context.Background(), Request{
		Type:       "minio_s3_bucket",
		Attributes: map[string]interface{}{"bucket": "agent-core", "colour": "blue"},
	})
	if err == nil {
		t.Fatal("an attribute the schema does not declare must be refused before any provider call")
	}
}

func TestPlanRejectsAnUnknownResourceType(t *testing.T) {
	core := openCore(t)

	_, err := core.Plan(context.Background(), Request{Type: "minio_teapot"})
	if err == nil {
		t.Fatal("a resource type the provider does not serve must be refused")
	}
}

func liveCore(t *testing.T) *Core {
	t.Helper()
	if os.Getenv("MINIO_ENDPOINT") == "" {
		t.Skip("MINIO_ENDPOINT is not set")
	}
	core := openCore(t)
	if err := core.Configure(context.Background(), nil); err != nil {
		t.Fatalf("configuring the provider from the environment: %s", err)
	}
	return core
}

func TestConvergeCreatesUpdatesAndDeletesABucket(t *testing.T) {
	core := liveCore(t)
	ctx := context.Background()
	name := fmt.Sprintf("agent-core-%d", time.Now().UnixNano())

	created, err := core.Converge(ctx, Request{
		Type:       "minio_s3_bucket",
		ID:         name,
		Attributes: map[string]interface{}{"bucket": name},
	})
	if err != nil {
		t.Fatalf("first converge: %s", err)
	}
	if created.Action != ActionCreate {
		t.Errorf("first converge action = %q, want %q", created.Action, ActionCreate)
	}
	if !created.Converged {
		t.Errorf("first converge did not converge: %v", created.Drift)
	}
	if created.State["arn"] != "arn:aws:s3:::"+name {
		t.Errorf("arn = %v", created.State["arn"])
	}

	again, err := core.Converge(ctx, Request{
		Type:       "minio_s3_bucket",
		ID:         name,
		Attributes: map[string]interface{}{"bucket": name},
	})
	if err != nil {
		t.Fatalf("second converge: %s", err)
	}
	if again.Action != ActionNoop {
		t.Errorf("second converge action = %q, want %q: the same request must be idempotent", again.Action, ActionNoop)
	}

	tagged, err := core.Converge(ctx, Request{
		Type:       "minio_s3_bucket",
		ID:         name,
		Attributes: map[string]interface{}{"bucket": name, "tags": map[string]string{"owner": "agent"}},
	})
	if err != nil {
		t.Fatalf("tagging converge: %s", err)
	}
	if tagged.Action != ActionUpdate {
		t.Errorf("tagging converge action = %q, want %q", tagged.Action, ActionUpdate)
	}
	if !tagged.Converged {
		t.Errorf("tagging converge did not converge: %v", tagged.Drift)
	}
	tags, _ := tagged.State["tags"].(map[string]interface{})
	if tags["owner"] != "agent" {
		t.Errorf("tags after converge = %v", tagged.State["tags"])
	}

	deleted, err := core.Delete(ctx, Request{Type: "minio_s3_bucket", ID: name})
	if err != nil {
		t.Fatalf("delete: %s", err)
	}
	if deleted.Action != ActionDelete || !deleted.Converged {
		t.Errorf("delete = %+v", deleted)
	}

	gone, err := core.Delete(ctx, Request{Type: "minio_s3_bucket", ID: name})
	if err != nil {
		t.Fatalf("second delete: %s", err)
	}
	if gone.Action != ActionNoop {
		t.Errorf("second delete action = %q, want %q", gone.Action, ActionNoop)
	}
}

func TestConvergeReplacesWhenTheNameChanges(t *testing.T) {
	core := liveCore(t)
	ctx := context.Background()
	name := fmt.Sprintf("agent-core-%d", time.Now().UnixNano())
	renamed := name + "-renamed"

	if _, err := core.Converge(ctx, Request{
		Type:       "minio_s3_bucket",
		ID:         name,
		Attributes: map[string]interface{}{"bucket": name},
	}); err != nil {
		t.Fatalf("create: %s", err)
	}
	t.Cleanup(func() {
		_, _ = core.Delete(ctx, Request{Type: "minio_s3_bucket", ID: name})
		_, _ = core.Delete(ctx, Request{Type: "minio_s3_bucket", ID: renamed})
	})

	preview, err := core.Plan(ctx, Request{
		Type:       "minio_s3_bucket",
		ID:         name,
		Attributes: map[string]interface{}{"bucket": renamed},
	})
	if err != nil {
		t.Fatalf("plan: %s", err)
	}
	if preview.Action != ActionReplace {
		t.Fatalf("plan action = %q, want %q; requires_replace = %v", preview.Action, ActionReplace, preview.RequiresReplace)
	}

	result, err := core.Converge(ctx, Request{
		Type:       "minio_s3_bucket",
		ID:         name,
		Attributes: map[string]interface{}{"bucket": renamed},
	})
	if err != nil {
		t.Fatalf("converge: %s", err)
	}
	if result.Action != ActionReplace || !result.Converged || result.ID != renamed {
		t.Errorf("converge = %+v", result)
	}

	old, err := core.Delete(ctx, Request{Type: "minio_s3_bucket", ID: name})
	if err != nil {
		t.Fatalf("checking the old bucket: %s", err)
	}
	if old.Action != ActionNoop {
		t.Errorf("the old bucket still existed after the replacement")
	}
}
