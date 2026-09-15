package agentcore

import (
	"context"
	"os"
	"strings"
	"testing"
)

func binaryCore(t *testing.T, envKey string) *Core {
	t.Helper()
	path := os.Getenv(envKey)
	if path == "" {
		t.Skipf("%s is not set", envKey)
	}
	core, err := OpenBinary(context.Background(), path)
	if err != nil {
		t.Fatalf("opening %s: %s", path, err)
	}
	t.Cleanup(core.Close)
	if err := core.Configure(context.Background(), nil); err != nil {
		t.Fatalf("configuring %s: %s", path, err)
	}
	return core
}

func TestRandomProviderServesItsResourcesOverGRPC(t *testing.T) {
	core := binaryCore(t, "AGENTCORE_RANDOM_PROVIDER")

	types := strings.Join(core.ResourceTypes(), ",")
	for _, want := range []string{"random_pet", "random_integer", "random_string"} {
		if !strings.Contains(types, want) {
			t.Errorf("%s is not served; the provider advertises %s", want, types)
		}
	}
}

func TestRandomPetConvergesWithoutAServer(t *testing.T) {
	core := binaryCore(t, "AGENTCORE_RANDOM_PROVIDER")
	ctx := context.Background()

	created, err := core.Converge(ctx, Request{
		Type:       "random_pet",
		Attributes: map[string]interface{}{"length": 3, "prefix": "agent"},
	})
	if err != nil {
		t.Fatalf("converge: %s", err)
	}
	if created.Action != ActionCreate || !created.Converged {
		t.Fatalf("converge = %+v", created)
	}

	id, _ := created.State["id"].(string)
	if !strings.HasPrefix(id, "agent-") || len(strings.Split(id, "-")) != 4 {
		t.Errorf("id = %q, want a prefix and three words", id)
	}
	if created.ID != id {
		t.Errorf("result id = %q, want the computed id %q", created.ID, id)
	}

	preview, err := core.Plan(ctx, Request{
		Type:       "random_pet",
		ID:         id,
		Attributes: map[string]interface{}{"length": 3, "prefix": "agent"},
	})
	if err != nil {
		t.Fatalf("plan by id: %s", err)
	}
	if preview.Action == ActionNoop {
		t.Errorf("plan by id = %+v: random keeps nothing to read back, so discovery by id cannot recover the attributes and the core must remember them", preview)
	}

	deleted, err := core.Delete(ctx, Request{Type: "random_pet", ID: id})
	if err != nil {
		t.Fatalf("delete: %s", err)
	}
	if deleted.Action != ActionDelete || !deleted.Converged {
		t.Errorf("delete = %+v", deleted)
	}
}

func TestRandomIntegerHonoursItsBounds(t *testing.T) {
	core := binaryCore(t, "AGENTCORE_RANDOM_PROVIDER")

	result, err := core.Converge(context.Background(), Request{
		Type:       "random_integer",
		Attributes: map[string]interface{}{"min": 10, "max": 10},
	})
	if err != nil {
		t.Fatalf("converge: %s", err)
	}
	if result.State["result"] != int64(10) {
		t.Errorf("result = %v (%T), want 10", result.State["result"], result.State["result"])
	}
	if result.State["id"] != "10" {
		t.Errorf("id = %v, want the integer as a string", result.State["id"])
	}
}

func TestNullProviderServesItsResourcesOverGRPC(t *testing.T) {
	core := binaryCore(t, "AGENTCORE_NULL_PROVIDER")

	types := strings.Join(core.ResourceTypes(), ",")
	if !strings.Contains(types, "null_resource") {
		t.Errorf("null_resource is not served; the provider advertises %s", types)
	}
}

func TestNullResourceConvergesAndDeletes(t *testing.T) {
	core := binaryCore(t, "AGENTCORE_NULL_PROVIDER")
	ctx := context.Background()

	created, err := core.Converge(ctx, Request{
		Type:       "null_resource",
		Attributes: map[string]interface{}{"triggers": map[string]string{"version": "1"}},
	})
	if err != nil {
		t.Fatalf("converge: %s", err)
	}
	if created.Action != ActionCreate || !created.Converged {
		t.Fatalf("converge = %+v", created)
	}
	if created.ID == "" {
		t.Fatal("null_resource returned no id")
	}

	deleted, err := core.Delete(ctx, Request{Type: "null_resource", ID: created.ID})
	if err != nil {
		t.Fatalf("delete: %s", err)
	}
	if deleted.Action != ActionDelete || !deleted.Converged {
		t.Errorf("delete = %+v", deleted)
	}
}

func TestBinaryCoreRefusesAttributesOutsideTheSchema(t *testing.T) {
	core := binaryCore(t, "AGENTCORE_NULL_PROVIDER")

	_, err := core.Plan(context.Background(), Request{
		Type:       "null_resource",
		Attributes: map[string]interface{}{"colour": "blue"},
	})
	if err == nil {
		t.Fatal("an attribute the schema does not declare must be refused before any provider call")
	}
}
