package agentcore

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/aminueza/terraform-provider-minio/v3/experiments/agentcore/internal/tfplugin6"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"google.golang.org/grpc"
)

const (
	pluginMagicCookieKey   = "TF_PLUGIN_MAGIC_COOKIE"
	pluginMagicCookieValue = "d602bf8f470bc67ca7faa0386276bbdd4330efaf76d1a219cb4d6991ca9872b2"
	pluginName             = "provider"
)

func OpenBinary(ctx context.Context, path string) (*Core, error) {
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  6,
			MagicCookieKey:   pluginMagicCookieKey,
			MagicCookieValue: pluginMagicCookieValue,
		},
		VersionedPlugins: map[int]plugin.PluginSet{
			6: {pluginName: &grpcPlugin{}},
		},
		Cmd:              exec.CommandContext(ctx, path),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		AutoMTLS:         true,
		Logger:           hclog.NewNullLogger(),
	})

	rpc, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("starting %s: %w", path, err)
	}

	raw, err := rpc.Dispense(pluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("dispensing the provider from %s: %w", path, err)
	}

	server, ok := raw.(providerAPI)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("%s did not serve a protocol 6 provider", path)
	}

	core, err := openServer(ctx, server)
	if err != nil {
		client.Kill()
		return nil, err
	}
	core.closer = client.Kill
	return core, nil
}

func (p *grpcPlugin) GRPCServer(*plugin.GRPCBroker, *grpc.Server) error {
	return fmt.Errorf("agentcore only runs providers, it does not serve them")
}

func (p *grpcPlugin) GRPCClient(_ context.Context, _ *plugin.GRPCBroker, conn *grpc.ClientConn) (interface{}, error) {
	return &grpcProvider{client: tfplugin6.NewProviderClient(conn)}, nil
}

func (p *grpcProvider) GetProviderSchema(ctx context.Context, _ *tfprotov6.GetProviderSchemaRequest) (*tfprotov6.GetProviderSchemaResponse, error) {
	resp, err := p.client.GetProviderSchema(ctx, &tfplugin6.GetProviderSchema_Request{})
	if err != nil {
		return nil, err
	}

	out := &tfprotov6.GetProviderSchemaResponse{
		Provider:        schemaFromProto(resp.Provider),
		ResourceSchemas: make(map[string]*tfprotov6.Schema, len(resp.ResourceSchemas)),
		Diagnostics:     diagnosticsFromProto(resp.Diagnostics),
	}
	for name, schema := range resp.ResourceSchemas {
		out.ResourceSchemas[name] = schemaFromProto(schema)
	}
	return out, nil
}

func (p *grpcProvider) ConfigureProvider(ctx context.Context, req *tfprotov6.ConfigureProviderRequest) (*tfprotov6.ConfigureProviderResponse, error) {
	resp, err := p.client.ConfigureProvider(ctx, &tfplugin6.ConfigureProvider_Request{
		TerraformVersion: req.TerraformVersion,
		Config:           dynamicToProto(req.Config),
	})
	if err != nil {
		return nil, err
	}
	return &tfprotov6.ConfigureProviderResponse{Diagnostics: diagnosticsFromProto(resp.Diagnostics)}, nil
}

func (p *grpcProvider) ReadResource(ctx context.Context, req *tfprotov6.ReadResourceRequest) (*tfprotov6.ReadResourceResponse, error) {
	resp, err := p.client.ReadResource(ctx, &tfplugin6.ReadResource_Request{
		TypeName:     req.TypeName,
		CurrentState: dynamicToProto(req.CurrentState),
		Private:      req.Private,
	})
	if err != nil {
		return nil, err
	}
	return &tfprotov6.ReadResourceResponse{
		NewState:    dynamicFromProto(resp.NewState),
		Private:     resp.Private,
		Diagnostics: diagnosticsFromProto(resp.Diagnostics),
	}, nil
}

func (p *grpcProvider) ImportResourceState(ctx context.Context, req *tfprotov6.ImportResourceStateRequest) (*tfprotov6.ImportResourceStateResponse, error) {
	resp, err := p.client.ImportResourceState(ctx, &tfplugin6.ImportResourceState_Request{
		TypeName: req.TypeName,
		Id:       req.ID,
	})
	if err != nil {
		return nil, err
	}

	out := &tfprotov6.ImportResourceStateResponse{Diagnostics: diagnosticsFromProto(resp.Diagnostics)}
	for _, imported := range resp.ImportedResources {
		out.ImportedResources = append(out.ImportedResources, &tfprotov6.ImportedResource{
			TypeName: imported.TypeName,
			State:    dynamicFromProto(imported.State),
			Private:  imported.Private,
		})
	}
	return out, nil
}

func (p *grpcProvider) PlanResourceChange(ctx context.Context, req *tfprotov6.PlanResourceChangeRequest) (*tfprotov6.PlanResourceChangeResponse, error) {
	resp, err := p.client.PlanResourceChange(ctx, &tfplugin6.PlanResourceChange_Request{
		TypeName:         req.TypeName,
		PriorState:       dynamicToProto(req.PriorState),
		ProposedNewState: dynamicToProto(req.ProposedNewState),
		Config:           dynamicToProto(req.Config),
		PriorPrivate:     req.PriorPrivate,
	})
	if err != nil {
		return nil, err
	}

	out := &tfprotov6.PlanResourceChangeResponse{
		PlannedState:                dynamicFromProto(resp.PlannedState),
		PlannedPrivate:              resp.PlannedPrivate,
		Diagnostics:                 diagnosticsFromProto(resp.Diagnostics),
		UnsafeToUseLegacyTypeSystem: resp.LegacyTypeSystem,
	}
	for _, path := range resp.RequiresReplace {
		out.RequiresReplace = append(out.RequiresReplace, pathFromProto(path))
	}
	return out, nil
}

func (p *grpcProvider) ApplyResourceChange(ctx context.Context, req *tfprotov6.ApplyResourceChangeRequest) (*tfprotov6.ApplyResourceChangeResponse, error) {
	resp, err := p.client.ApplyResourceChange(ctx, &tfplugin6.ApplyResourceChange_Request{
		TypeName:       req.TypeName,
		PriorState:     dynamicToProto(req.PriorState),
		PlannedState:   dynamicToProto(req.PlannedState),
		Config:         dynamicToProto(req.Config),
		PlannedPrivate: req.PlannedPrivate,
	})
	if err != nil {
		return nil, err
	}
	return &tfprotov6.ApplyResourceChangeResponse{
		NewState:                    dynamicFromProto(resp.NewState),
		Private:                     resp.Private,
		Diagnostics:                 diagnosticsFromProto(resp.Diagnostics),
		UnsafeToUseLegacyTypeSystem: resp.LegacyTypeSystem,
	}, nil
}

func dynamicToProto(value *tfprotov6.DynamicValue) *tfplugin6.DynamicValue {
	if value == nil {
		return nil
	}
	return &tfplugin6.DynamicValue{Msgpack: value.MsgPack, Json: value.JSON}
}

func dynamicFromProto(value *tfplugin6.DynamicValue) *tfprotov6.DynamicValue {
	if value == nil {
		return nil
	}
	return &tfprotov6.DynamicValue{MsgPack: value.Msgpack, JSON: value.Json}
}

func diagnosticsFromProto(diagnostics []*tfplugin6.Diagnostic) []*tfprotov6.Diagnostic {
	out := make([]*tfprotov6.Diagnostic, 0, len(diagnostics))
	for _, d := range diagnostics {
		out = append(out, &tfprotov6.Diagnostic{
			Severity:  tfprotov6.DiagnosticSeverity(d.Severity),
			Summary:   d.Summary,
			Detail:    d.Detail,
			Attribute: pathFromProto(d.Attribute),
		})
	}
	return out
}

func pathFromProto(path *tfplugin6.AttributePath) *tftypes.AttributePath {
	if path == nil {
		return nil
	}
	steps := make([]tftypes.AttributePathStep, 0, len(path.Steps))
	for _, step := range path.Steps {
		switch selector := step.Selector.(type) {
		case *tfplugin6.AttributePath_Step_AttributeName:
			steps = append(steps, tftypes.AttributeName(selector.AttributeName))
		case *tfplugin6.AttributePath_Step_ElementKeyString:
			steps = append(steps, tftypes.ElementKeyString(selector.ElementKeyString))
		case *tfplugin6.AttributePath_Step_ElementKeyInt:
			steps = append(steps, tftypes.ElementKeyInt(selector.ElementKeyInt))
		}
	}
	return tftypes.NewAttributePathWithSteps(steps)
}

func schemaFromProto(schema *tfplugin6.Schema) *tfprotov6.Schema {
	if schema == nil {
		return nil
	}
	return &tfprotov6.Schema{Version: schema.Version, Block: blockFromProto(schema.Block)}
}

func blockFromProto(block *tfplugin6.Schema_Block) *tfprotov6.SchemaBlock {
	if block == nil {
		return nil
	}
	out := &tfprotov6.SchemaBlock{
		Version:         block.Version,
		Description:     block.Description,
		DescriptionKind: tfprotov6.StringKind(block.DescriptionKind),
		Deprecated:      block.Deprecated,
	}
	for _, attribute := range block.Attributes {
		out.Attributes = append(out.Attributes, attributeFromProto(attribute))
	}
	for _, nested := range block.BlockTypes {
		out.BlockTypes = append(out.BlockTypes, &tfprotov6.SchemaNestedBlock{
			TypeName: nested.TypeName,
			Block:    blockFromProto(nested.Block),
			Nesting:  tfprotov6.SchemaNestedBlockNestingMode(nested.Nesting),
			MinItems: nested.MinItems,
			MaxItems: nested.MaxItems,
		})
	}
	return out
}

func attributeFromProto(attribute *tfplugin6.Schema_Attribute) *tfprotov6.SchemaAttribute {
	out := &tfprotov6.SchemaAttribute{
		Name:            attribute.Name,
		Description:     attribute.Description,
		DescriptionKind: tfprotov6.StringKind(attribute.DescriptionKind),
		Required:        attribute.Required,
		Optional:        attribute.Optional,
		Computed:        attribute.Computed,
		Sensitive:       attribute.Sensitive,
		Deprecated:      attribute.Deprecated,
		WriteOnly:       attribute.WriteOnly,
	}
	if len(attribute.Type) > 0 {
		parsed, err := tftypes.ParseJSONType(attribute.Type)
		if err == nil {
			out.Type = parsed
		}
	}
	if attribute.NestedType != nil {
		nested := &tfprotov6.SchemaObject{
			Nesting: tfprotov6.SchemaObjectNestingMode(attribute.NestedType.Nesting),
		}
		for _, inner := range attribute.NestedType.Attributes {
			nested.Attributes = append(nested.Attributes, attributeFromProto(inner))
		}
		out.NestedType = nested
	}
	return out
}
