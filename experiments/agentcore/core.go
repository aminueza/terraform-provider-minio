package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/aminueza/terraform-provider-minio/v3/minio"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const terraformVersion = "1.13.0"

const (
	ActionCreate  = "create"
	ActionUpdate  = "update"
	ActionReplace = "replace"
	ActionNoop    = "noop"
	ActionDelete  = "delete"
)

func (e *ProtocolError) Error() string {
	parts := make([]string, 0, len(e.Diagnostics))
	for _, d := range e.Diagnostics {
		if d.Severity != tfprotov6.DiagnosticSeverityError {
			continue
		}
		part := d.Summary
		if d.Detail != "" {
			part += ": " + d.Detail
		}
		if d.Attribute != nil {
			part = d.Attribute.String() + ": " + part
		}
		parts = append(parts, part)
	}
	return e.Step + ": " + strings.Join(parts, "; ")
}

func failed(step string, diagnostics []*tfprotov6.Diagnostic) error {
	for _, d := range diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			return &ProtocolError{Step: step, Diagnostics: diagnostics}
		}
	}
	return nil
}

func Open(ctx context.Context) (*Core, error) {
	server, err := minio.MuxProviderServer(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting the provider: %w", err)
	}

	schema, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		return nil, fmt.Errorf("reading the provider schema: %w", err)
	}
	if err := failed("GetProviderSchema", schema.Diagnostics); err != nil {
		return nil, err
	}

	return &Core{server: server, schema: schema}, nil
}

func (c *Core) ResourceTypes() []string {
	names := make([]string, 0, len(c.schema.ResourceSchemas))
	for name := range c.schema.ResourceSchemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Core) Configure(ctx context.Context, attributes map[string]interface{}) error {
	config, err := valueFromAttributes(c.schema.Provider.ValueType(), attributes)
	if err != nil {
		return fmt.Errorf("provider configuration: %w", err)
	}

	dynamic, err := tfprotov6.NewDynamicValue(config.Type(), config)
	if err != nil {
		return err
	}

	resp, err := c.server.ConfigureProvider(ctx, &tfprotov6.ConfigureProviderRequest{
		TerraformVersion: terraformVersion,
		Config:           &dynamic,
	})
	if err != nil {
		return err
	}
	return failed("ConfigureProvider", resp.Diagnostics)
}

func (c *Core) resourceSchema(typeName string) (*tfprotov6.Schema, error) {
	schema, ok := c.schema.ResourceSchemas[typeName]
	if !ok {
		return nil, fmt.Errorf("the provider serves no resource named %q", typeName)
	}
	return schema, nil
}

func (c *Core) Plan(ctx context.Context, req Request) (*Preview, error) {
	schema, err := c.resourceSchema(req.Type)
	if err != nil {
		return nil, err
	}

	prior, err := c.discover(ctx, req.Type, schema, req.ID)
	if err != nil {
		return nil, err
	}

	config, err := valueFromAttributes(schema.ValueType(), req.Attributes)
	if err != nil {
		return nil, err
	}

	planned, replace, _, err := c.plan(ctx, req.Type, schema, prior, config)
	if err != nil {
		return nil, err
	}

	return &Preview{
		Action:          classify(prior.value, planned, replace),
		RequiresReplace: pathStrings(replace),
		Planned:         attributesFromValue(planned),
	}, nil
}

func (c *Core) Converge(ctx context.Context, req Request) (*Result, error) {
	schema, err := c.resourceSchema(req.Type)
	if err != nil {
		return nil, err
	}

	result := &Result{Type: req.Type, ID: req.ID}

	prior, err := c.discover(ctx, req.Type, schema, req.ID)
	if err != nil {
		return nil, err
	}
	result.Steps = append(result.Steps, "discovered prior state through ImportResourceState and ReadResource")

	config, err := valueFromAttributes(schema.ValueType(), req.Attributes)
	if err != nil {
		return nil, err
	}

	planned, replace, private, err := c.plan(ctx, req.Type, schema, prior, config)
	if err != nil {
		return nil, err
	}
	result.Action = classify(prior.value, planned, replace)
	result.Steps = append(result.Steps, "planned "+result.Action)

	current := prior
	switch result.Action {
	case ActionNoop:
	case ActionReplace:
		if _, err := c.apply(ctx, req.Type, schema, prior, nullOf(schema), nullOf(schema), nil); err != nil {
			return nil, err
		}
		result.Steps = append(result.Steps, "applied the destroy half of the replacement")

		absent := resourceState{value: nullOf(schema)}
		planned, _, private, err = c.plan(ctx, req.Type, schema, absent, config)
		if err != nil {
			return nil, err
		}
		current, err = c.apply(ctx, req.Type, schema, absent, planned, config, private)
		if err != nil {
			return nil, err
		}
		result.Steps = append(result.Steps, "applied the create half of the replacement")
	default:
		current, err = c.apply(ctx, req.Type, schema, prior, planned, config, private)
		if err != nil {
			return nil, err
		}
		result.Steps = append(result.Steps, "applied")
	}

	read, err := c.read(ctx, req.Type, current)
	if err != nil {
		return nil, err
	}
	result.Steps = append(result.Steps, "read the resource back")

	again, _, _, err := c.plan(ctx, req.Type, schema, read, config)
	if err != nil {
		return nil, err
	}
	result.Converged = again.Equal(read.value)
	result.Drift = diffPaths(read.value, again)
	result.State = attributesFromValue(read.value)
	result.ID = idOf(read.value, req.ID)
	if result.Converged {
		result.Steps = append(result.Steps, "planned again: no change")
	} else {
		result.Steps = append(result.Steps, "planned again: the provider still wants to change "+strings.Join(result.Drift, ", "))
	}

	return result, nil
}

func (c *Core) Delete(ctx context.Context, req Request) (*Result, error) {
	schema, err := c.resourceSchema(req.Type)
	if err != nil {
		return nil, err
	}

	result := &Result{Type: req.Type, ID: req.ID, Converged: true}

	prior, err := c.discover(ctx, req.Type, schema, req.ID)
	if err != nil {
		return nil, err
	}
	if prior.value.IsNull() {
		result.Action = ActionNoop
		result.Steps = append(result.Steps, "the resource is already absent")
		return result, nil
	}

	if _, err := c.apply(ctx, req.Type, schema, prior, nullOf(schema), nullOf(schema), nil); err != nil {
		return nil, err
	}
	result.Action = ActionDelete
	result.Steps = append(result.Steps, "applied the destroy")

	read, err := c.read(ctx, req.Type, prior)
	if err != nil {
		return nil, err
	}
	result.Converged = read.value.IsNull()
	if !result.Converged {
		result.Steps = append(result.Steps, "read the resource back and it still exists")
	}
	return result, nil
}

func (c *Core) discover(ctx context.Context, typeName string, schema *tfprotov6.Schema, id string) (resourceState, error) {
	absent := resourceState{value: nullOf(schema)}
	if id == "" {
		return absent, nil
	}

	found, err := c.read(ctx, typeName, stubState(schema, id))
	if err != nil {
		return absent, err
	}
	if found.value.IsNull() {
		return absent, nil
	}

	imported, err := c.server.ImportResourceState(ctx, &tfprotov6.ImportResourceStateRequest{
		TypeName: typeName,
		ID:       id,
	})
	if err != nil {
		return absent, err
	}
	if err := failed("ImportResourceState", imported.Diagnostics); err != nil {
		return absent, err
	}
	if len(imported.ImportedResources) == 0 {
		return found, nil
	}

	stub := imported.ImportedResources[0]
	value, err := stub.State.Unmarshal(schema.ValueType())
	if err != nil {
		return absent, err
	}
	return resourceState{value: value, private: stub.Private}, nil
}

func stubState(schema *tfprotov6.Schema, id string) resourceState {
	objectType, ok := schema.ValueType().(tftypes.Object)
	if !ok {
		return resourceState{value: nullOf(schema)}
	}

	attributes := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attributeType, nil)
	}
	if _, ok := objectType.AttributeTypes["id"]; ok {
		attributes["id"] = tftypes.NewValue(tftypes.String, id)
	}
	return resourceState{value: tftypes.NewValue(objectType, attributes)}
}

func (c *Core) read(ctx context.Context, typeName string, state resourceState) (resourceState, error) {
	if state.value.IsNull() {
		return state, nil
	}

	dynamic, err := tfprotov6.NewDynamicValue(state.value.Type(), state.value)
	if err != nil {
		return state, err
	}

	resp, err := c.server.ReadResource(ctx, &tfprotov6.ReadResourceRequest{
		TypeName:     typeName,
		CurrentState: &dynamic,
		Private:      state.private,
	})
	if err != nil {
		return state, err
	}
	if err := failed("ReadResource", resp.Diagnostics); err != nil {
		return state, err
	}

	value, err := resp.NewState.Unmarshal(state.value.Type())
	if err != nil {
		return state, err
	}
	return resourceState{value: value, private: resp.Private}, nil
}

func (c *Core) plan(ctx context.Context, typeName string, schema *tfprotov6.Schema, prior resourceState, config tftypes.Value) (tftypes.Value, []*tftypes.AttributePath, []byte, error) {
	proposed, err := proposedNew(schema.Block, prior.value, config)
	if err != nil {
		return tftypes.Value{}, nil, nil, err
	}

	priorDynamic, err := tfprotov6.NewDynamicValue(prior.value.Type(), prior.value)
	if err != nil {
		return tftypes.Value{}, nil, nil, err
	}
	proposedDynamic, err := tfprotov6.NewDynamicValue(proposed.Type(), proposed)
	if err != nil {
		return tftypes.Value{}, nil, nil, err
	}
	configDynamic, err := tfprotov6.NewDynamicValue(config.Type(), config)
	if err != nil {
		return tftypes.Value{}, nil, nil, err
	}

	resp, err := c.server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         typeName,
		PriorState:       &priorDynamic,
		ProposedNewState: &proposedDynamic,
		Config:           &configDynamic,
		PriorPrivate:     prior.private,
	})
	if err != nil {
		return tftypes.Value{}, nil, nil, err
	}
	if err := failed("PlanResourceChange", resp.Diagnostics); err != nil {
		return tftypes.Value{}, nil, nil, err
	}

	planned, err := resp.PlannedState.Unmarshal(schema.ValueType())
	if err != nil {
		return tftypes.Value{}, nil, nil, err
	}
	return planned, resp.RequiresReplace, resp.PlannedPrivate, nil
}

func (c *Core) apply(ctx context.Context, typeName string, schema *tfprotov6.Schema, prior resourceState, planned, config tftypes.Value, private []byte) (resourceState, error) {
	priorDynamic, err := tfprotov6.NewDynamicValue(prior.value.Type(), prior.value)
	if err != nil {
		return resourceState{}, err
	}
	plannedDynamic, err := tfprotov6.NewDynamicValue(planned.Type(), planned)
	if err != nil {
		return resourceState{}, err
	}
	configDynamic, err := tfprotov6.NewDynamicValue(config.Type(), config)
	if err != nil {
		return resourceState{}, err
	}

	resp, err := c.server.ApplyResourceChange(ctx, &tfprotov6.ApplyResourceChangeRequest{
		TypeName:       typeName,
		PriorState:     &priorDynamic,
		PlannedState:   &plannedDynamic,
		Config:         &configDynamic,
		PlannedPrivate: private,
	})
	if err != nil {
		return resourceState{}, err
	}
	if err := failed("ApplyResourceChange", resp.Diagnostics); err != nil {
		return resourceState{}, err
	}

	value, err := resp.NewState.Unmarshal(schema.ValueType())
	if err != nil {
		return resourceState{}, err
	}
	return resourceState{value: value, private: resp.Private}, nil
}

func proposedNew(block *tfprotov6.SchemaBlock, prior, config tftypes.Value) (tftypes.Value, error) {
	if config.IsNull() {
		return config, nil
	}

	priorAttributes := map[string]tftypes.Value{}
	if !prior.IsNull() {
		if err := prior.As(&priorAttributes); err != nil {
			return tftypes.Value{}, err
		}
	}

	configAttributes := map[string]tftypes.Value{}
	if err := config.As(&configAttributes); err != nil {
		return tftypes.Value{}, err
	}

	proposed := make(map[string]tftypes.Value, len(configAttributes))
	for name, value := range configAttributes {
		proposed[name] = value
	}
	for _, attribute := range block.Attributes {
		if !attribute.Computed {
			continue
		}
		if !proposed[attribute.Name].IsNull() {
			continue
		}
		if priorValue, ok := priorAttributes[attribute.Name]; ok {
			proposed[attribute.Name] = priorValue
		}
	}

	return tftypes.NewValue(config.Type(), proposed), nil
}

func classify(prior, planned tftypes.Value, replace []*tftypes.AttributePath) string {
	switch {
	case prior.IsNull():
		return ActionCreate
	case len(replace) > 0:
		return ActionReplace
	case planned.Equal(prior):
		return ActionNoop
	default:
		return ActionUpdate
	}
}

func nullOf(schema *tfprotov6.Schema) tftypes.Value {
	return tftypes.NewValue(schema.ValueType(), nil)
}

func idOf(state tftypes.Value, fallback string) string {
	if state.IsNull() {
		return fallback
	}
	attributes := map[string]tftypes.Value{}
	if err := state.As(&attributes); err != nil {
		return fallback
	}
	id, ok := attributes["id"]
	if !ok || id.IsNull() || !id.IsKnown() {
		return fallback
	}
	var s string
	if err := id.As(&s); err != nil {
		return fallback
	}
	return s
}

func pathStrings(paths []*tftypes.AttributePath) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, p.String())
	}
	return out
}

func diffPaths(before, after tftypes.Value) []string {
	diffs, err := before.Diff(after)
	if err != nil {
		return []string{err.Error()}
	}
	out := make([]string, 0, len(diffs))
	for _, d := range diffs {
		out = append(out, fmt.Sprintf("%s: %s -> %s", d.Path.String(), d.Value1.String(), d.Value2.String()))
	}
	sort.Strings(out)
	return out
}

func valueFromAttributes(t tftypes.Type, attributes map[string]interface{}) (tftypes.Value, error) {
	if attributes == nil {
		attributes = map[string]interface{}{}
	}
	raw, err := json.Marshal(attributes)
	if err != nil {
		return tftypes.Value{}, err
	}
	value, err := tftypes.ValueFromJSON(raw, t)
	if err != nil {
		return tftypes.Value{}, fmt.Errorf("attributes do not fit the schema: %w", err)
	}
	return value, nil
}

func attributesFromValue(value tftypes.Value) map[string]interface{} {
	if value.IsNull() || !value.IsKnown() {
		return nil
	}
	out, _ := plain(value).(map[string]interface{})
	return out
}

func plain(value tftypes.Value) interface{} {
	if !value.IsKnown() {
		return "(known after apply)"
	}
	if value.IsNull() {
		return nil
	}

	t := value.Type()
	switch {
	case t.Is(tftypes.String):
		var s string
		_ = value.As(&s)
		return s
	case t.Is(tftypes.Bool):
		var b bool
		_ = value.As(&b)
		return b
	case t.Is(tftypes.Number):
		n := new(big.Float)
		_ = value.As(&n)
		if n.IsInt() {
			i, _ := n.Int64()
			return i
		}
		f, _ := n.Float64()
		return f
	case t.Is(tftypes.List{}), t.Is(tftypes.Set{}), t.Is(tftypes.Tuple{}):
		var items []tftypes.Value
		_ = value.As(&items)
		out := make([]interface{}, 0, len(items))
		for _, item := range items {
			out = append(out, plain(item))
		}
		return out
	case t.Is(tftypes.Map{}), t.Is(tftypes.Object{}):
		var items map[string]tftypes.Value
		_ = value.As(&items)
		out := make(map[string]interface{}, len(items))
		for name, item := range items {
			out[name] = plain(item)
		}
		return out
	default:
		return value.String()
	}
}
