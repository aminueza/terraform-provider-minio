package agentcore

import (
	"context"

	"github.com/aminueza/terraform-provider-minio/v3/experiments/agentcore/internal/tfplugin5"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func (p *grpcProvider5) GetProviderSchema(ctx context.Context, _ *tfprotov6.GetProviderSchemaRequest) (*tfprotov6.GetProviderSchemaResponse, error) {
	resp, err := p.client.GetSchema(ctx, &tfplugin5.GetProviderSchema_Request{})
	if err != nil {
		return nil, err
	}

	out := &tfprotov6.GetProviderSchemaResponse{
		Provider:        schemaFromProto5(resp.Provider),
		ResourceSchemas: make(map[string]*tfprotov6.Schema, len(resp.ResourceSchemas)),
		Diagnostics:     diagnosticsFromProto5(resp.Diagnostics),
	}
	for name, schema := range resp.ResourceSchemas {
		out.ResourceSchemas[name] = schemaFromProto5(schema)
	}
	return out, nil
}

func (p *grpcProvider5) ConfigureProvider(ctx context.Context, req *tfprotov6.ConfigureProviderRequest) (*tfprotov6.ConfigureProviderResponse, error) {
	resp, err := p.client.Configure(ctx, &tfplugin5.Configure_Request{
		TerraformVersion: req.TerraformVersion,
		Config:           dynamicToProto5(req.Config),
	})
	if err != nil {
		return nil, err
	}
	return &tfprotov6.ConfigureProviderResponse{Diagnostics: diagnosticsFromProto5(resp.Diagnostics)}, nil
}

func (p *grpcProvider5) ReadResource(ctx context.Context, req *tfprotov6.ReadResourceRequest) (*tfprotov6.ReadResourceResponse, error) {
	resp, err := p.client.ReadResource(ctx, &tfplugin5.ReadResource_Request{
		TypeName:     req.TypeName,
		CurrentState: dynamicToProto5(req.CurrentState),
		Private:      req.Private,
	})
	if err != nil {
		return nil, err
	}
	return &tfprotov6.ReadResourceResponse{
		NewState:    dynamicFromProto5(resp.NewState),
		Private:     resp.Private,
		Diagnostics: diagnosticsFromProto5(resp.Diagnostics),
	}, nil
}

func (p *grpcProvider5) ImportResourceState(ctx context.Context, req *tfprotov6.ImportResourceStateRequest) (*tfprotov6.ImportResourceStateResponse, error) {
	resp, err := p.client.ImportResourceState(ctx, &tfplugin5.ImportResourceState_Request{
		TypeName: req.TypeName,
		Id:       req.ID,
	})
	if err != nil {
		return nil, err
	}

	out := &tfprotov6.ImportResourceStateResponse{Diagnostics: diagnosticsFromProto5(resp.Diagnostics)}
	for _, imported := range resp.ImportedResources {
		out.ImportedResources = append(out.ImportedResources, &tfprotov6.ImportedResource{
			TypeName: imported.TypeName,
			State:    dynamicFromProto5(imported.State),
			Private:  imported.Private,
		})
	}
	return out, nil
}

func (p *grpcProvider5) PlanResourceChange(ctx context.Context, req *tfprotov6.PlanResourceChangeRequest) (*tfprotov6.PlanResourceChangeResponse, error) {
	resp, err := p.client.PlanResourceChange(ctx, &tfplugin5.PlanResourceChange_Request{
		TypeName:         req.TypeName,
		PriorState:       dynamicToProto5(req.PriorState),
		ProposedNewState: dynamicToProto5(req.ProposedNewState),
		Config:           dynamicToProto5(req.Config),
		PriorPrivate:     req.PriorPrivate,
	})
	if err != nil {
		return nil, err
	}

	out := &tfprotov6.PlanResourceChangeResponse{
		PlannedState:                dynamicFromProto5(resp.PlannedState),
		PlannedPrivate:              resp.PlannedPrivate,
		Diagnostics:                 diagnosticsFromProto5(resp.Diagnostics),
		UnsafeToUseLegacyTypeSystem: resp.LegacyTypeSystem,
	}
	for _, path := range resp.RequiresReplace {
		out.RequiresReplace = append(out.RequiresReplace, pathFromProto5(path))
	}
	return out, nil
}

func (p *grpcProvider5) ApplyResourceChange(ctx context.Context, req *tfprotov6.ApplyResourceChangeRequest) (*tfprotov6.ApplyResourceChangeResponse, error) {
	resp, err := p.client.ApplyResourceChange(ctx, &tfplugin5.ApplyResourceChange_Request{
		TypeName:       req.TypeName,
		PriorState:     dynamicToProto5(req.PriorState),
		PlannedState:   dynamicToProto5(req.PlannedState),
		Config:         dynamicToProto5(req.Config),
		PlannedPrivate: req.PlannedPrivate,
	})
	if err != nil {
		return nil, err
	}
	return &tfprotov6.ApplyResourceChangeResponse{
		NewState:                    dynamicFromProto5(resp.NewState),
		Private:                     resp.Private,
		Diagnostics:                 diagnosticsFromProto5(resp.Diagnostics),
		UnsafeToUseLegacyTypeSystem: resp.LegacyTypeSystem,
	}, nil
}

func dynamicToProto5(value *tfprotov6.DynamicValue) *tfplugin5.DynamicValue {
	if value == nil {
		return nil
	}
	return &tfplugin5.DynamicValue{Msgpack: value.MsgPack, Json: value.JSON}
}

func dynamicFromProto5(value *tfplugin5.DynamicValue) *tfprotov6.DynamicValue {
	if value == nil {
		return nil
	}
	return &tfprotov6.DynamicValue{MsgPack: value.Msgpack, JSON: value.Json}
}

func diagnosticsFromProto5(diagnostics []*tfplugin5.Diagnostic) []*tfprotov6.Diagnostic {
	out := make([]*tfprotov6.Diagnostic, 0, len(diagnostics))
	for _, d := range diagnostics {
		out = append(out, &tfprotov6.Diagnostic{
			Severity:  tfprotov6.DiagnosticSeverity(d.Severity),
			Summary:   d.Summary,
			Detail:    d.Detail,
			Attribute: pathFromProto5(d.Attribute),
		})
	}
	return out
}

func pathFromProto5(path *tfplugin5.AttributePath) *tftypes.AttributePath {
	if path == nil {
		return nil
	}
	steps := make([]tftypes.AttributePathStep, 0, len(path.Steps))
	for _, step := range path.Steps {
		switch selector := step.Selector.(type) {
		case *tfplugin5.AttributePath_Step_AttributeName:
			steps = append(steps, tftypes.AttributeName(selector.AttributeName))
		case *tfplugin5.AttributePath_Step_ElementKeyString:
			steps = append(steps, tftypes.ElementKeyString(selector.ElementKeyString))
		case *tfplugin5.AttributePath_Step_ElementKeyInt:
			steps = append(steps, tftypes.ElementKeyInt(selector.ElementKeyInt))
		}
	}
	return tftypes.NewAttributePathWithSteps(steps)
}

func schemaFromProto5(schema *tfplugin5.Schema) *tfprotov6.Schema {
	if schema == nil {
		return nil
	}
	return &tfprotov6.Schema{Version: schema.Version, Block: blockFromProto5(schema.Block)}
}

func blockFromProto5(block *tfplugin5.Schema_Block) *tfprotov6.SchemaBlock {
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
		converted := &tfprotov6.SchemaAttribute{
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
		if parsed, err := tftypes.ParseJSONType(attribute.Type); err == nil {
			converted.Type = parsed
		}
		out.Attributes = append(out.Attributes, converted)
	}
	for _, nested := range block.BlockTypes {
		out.BlockTypes = append(out.BlockTypes, &tfprotov6.SchemaNestedBlock{
			TypeName: nested.TypeName,
			Block:    blockFromProto5(nested.Block),
			Nesting:  tfprotov6.SchemaNestedBlockNestingMode(nested.Nesting),
			MinItems: nested.MinItems,
			MaxItems: nested.MaxItems,
		})
	}
	return out
}
