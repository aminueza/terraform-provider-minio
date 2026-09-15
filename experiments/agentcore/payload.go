package agentcore

import (
	"context"

	"github.com/aminueza/terraform-provider-minio/v3/experiments/agentcore/internal/tfplugin6"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type providerAPI interface {
	GetProviderSchema(context.Context, *tfprotov6.GetProviderSchemaRequest) (*tfprotov6.GetProviderSchemaResponse, error)
	ConfigureProvider(context.Context, *tfprotov6.ConfigureProviderRequest) (*tfprotov6.ConfigureProviderResponse, error)
	ReadResource(context.Context, *tfprotov6.ReadResourceRequest) (*tfprotov6.ReadResourceResponse, error)
	ImportResourceState(context.Context, *tfprotov6.ImportResourceStateRequest) (*tfprotov6.ImportResourceStateResponse, error)
	PlanResourceChange(context.Context, *tfprotov6.PlanResourceChangeRequest) (*tfprotov6.PlanResourceChangeResponse, error)
	ApplyResourceChange(context.Context, *tfprotov6.ApplyResourceChangeRequest) (*tfprotov6.ApplyResourceChangeResponse, error)
}

type Core struct {
	server providerAPI
	schema *tfprotov6.GetProviderSchemaResponse
	closer func()
}

type Request struct {
	Type       string                 `json:"type"`
	ID         string                 `json:"id,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

type Result struct {
	Type      string                 `json:"type"`
	ID        string                 `json:"id"`
	Action    string                 `json:"action"`
	Converged bool                   `json:"converged"`
	Drift     []string               `json:"drift,omitempty"`
	State     map[string]interface{} `json:"state,omitempty"`
	Steps     []string               `json:"steps"`
}

type Preview struct {
	Action          string                 `json:"action"`
	RequiresReplace []string               `json:"requires_replace,omitempty"`
	Planned         map[string]interface{} `json:"planned,omitempty"`
	Steps           []string               `json:"steps"`
}

type ProtocolError struct {
	Step        string
	Diagnostics []*tfprotov6.Diagnostic
}

type resourceState struct {
	value   tftypes.Value
	private []byte
	note    string
}

type grpcPlugin struct {
	plugin.NetRPCUnsupportedPlugin
}

type grpcProvider struct {
	client tfplugin6.ProviderClient
}
