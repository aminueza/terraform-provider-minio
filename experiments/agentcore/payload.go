package agentcore

import (
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type Core struct {
	server tfprotov6.ProviderServer
	schema *tfprotov6.GetProviderSchemaResponse
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
}

type ProtocolError struct {
	Step        string
	Diagnostics []*tfprotov6.Diagnostic
}

type resourceState struct {
	value   tftypes.Value
	private []byte
}
