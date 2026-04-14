package minio

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &minioAuditWebhookResource{}
	_ resource.ResourceWithConfigure   = &minioAuditWebhookResource{}
	_ resource.ResourceWithImportState = &minioAuditWebhookResource{}
)

type minioAuditWebhookResource struct {
	client *S3MinioClient
}

type minioAuditWebhookResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Enable          types.Bool   `tfsdk:"enable"`
	QueueSize       types.Int64  `tfsdk:"queue_size"`
	BatchSize       types.Int64  `tfsdk:"batch_size"`
	RestartRequired types.Bool   `tfsdk:"restart_required"`
	Endpoint        types.String `tfsdk:"endpoint"`
	AuthToken       types.String `tfsdk:"auth_token"`
	ClientCert      types.String `tfsdk:"client_cert"`
	ClientKey       types.String `tfsdk:"client_key"`
}

func resourceMinioAuditWebhookFramework() resource.Resource {
	return &minioAuditWebhookResource{}
}

func (r *minioAuditWebhookResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_audit_webhook"
}

func (r *minioAuditWebhookResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *S3MinioClient, got: %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *minioAuditWebhookResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an audit webhook target for MinIO audit logging. Audit webhooks send detailed API audit events to HTTP endpoints for compliance, SIEM integration, and security monitoring.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true, Description: "Identifier of the audit webhook (same as name).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":             schema.StringAttribute{Required: true, Description: "Target name for the audit webhook (e.g., 'splunk', 'elk'). Used as the identifier in the configuration key 'audit_webhook:<name>'.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable":           schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether this audit webhook target is enabled.", Default: booldefault.StaticBool(true)},
			"queue_size":       schema.Int64Attribute{Optional: true, Computed: true, Description: "Maximum number of audit events to queue before dropping."},
			"batch_size":       schema.Int64Attribute{Optional: true, Computed: true, Description: "Number of audit events to send in a single batch to the endpoint."},
			"restart_required": schema.BoolAttribute{Computed: true, Description: "Indicates whether a MinIO server restart is required for the configuration to take effect."},
			"endpoint":         schema.StringAttribute{Required: true, Description: "HTTP(S) endpoint URL to send audit events to."},
			"auth_token":       schema.StringAttribute{Optional: true, Sensitive: true, Description: "Authentication token for the webhook endpoint (e.g., Bearer token). MinIO does not return this value on read."},
			"client_cert":      schema.StringAttribute{Optional: true, Description: "Path to the X.509 client certificate for mTLS authentication with the webhook endpoint."},
			"client_key":       schema.StringAttribute{Optional: true, Sensitive: true, Description: "Path to the X.509 private key for mTLS authentication. MinIO does not return this value on read."},
		},
	}
}

func (r *minioAuditWebhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioAuditWebhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := plan.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating audit_webhook: %s", name))
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "endpoint", data.GetStringField("endpoint").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "auth_token", data.GetStringField("auth_token").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_cert", data.GetStringField("client_cert").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_key", data.GetStringField("client_key").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "queue_size", data.GetInt64Field("queue_size").ValueInt64())
		notifyFrameworkBuildCfgAddInt(&parts, "batch_size", data.GetInt64Field("batch_size").ValueInt64())
		if !data.Enable.IsNull() && !data.Enable.IsUnknown() {
			notifyFrameworkBuildCfgAddBool(&parts, "enable", data.Enable.ValueBool())
		}
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["endpoint"]; ok {
			data.SetStringField("endpoint", types.StringValue(v))
		}
		if v, ok := cfgMap["client_cert"]; ok {
			if v != "" {
				data.SetStringField("client_cert", types.StringValue(v))
			} else if !data.GetStringField("client_cert").IsUnknown() {
				data.SetStringField("client_cert", types.StringNull())
			}
		}
		if v, ok := cfgMap["queue_size"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				data.SetInt64Field("queue_size", types.Int64Value(int64(n)))
			}
		}
		if v, ok := cfgMap["batch_size"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				data.SetInt64Field("batch_size", types.Int64Value(int64(n)))
			}
		}
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "audit_webhook", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueLimit: plan.QueueSize, Comment: types.String{}, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("endpoint", plan.Endpoint)
	notifyData.SetStringField("auth_token", plan.AuthToken)
	notifyData.SetStringField("client_cert", plan.ClientCert)
	notifyData.SetStringField("client_key", plan.ClientKey)
	notifyData.SetInt64Field("queue_size", plan.QueueSize)
	notifyData.SetInt64Field("batch_size", plan.BatchSize)
	diags := notifyFrameworkCreate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Name = notifyData.Name
	plan.ID = plan.Name
	plan.Enable = notifyData.Enable
	plan.QueueSize = notifyData.GetInt64Field("queue_size")
	plan.BatchSize = notifyData.GetInt64Field("batch_size")
	plan.RestartRequired = notifyData.RestartRequired
	plan.Endpoint = notifyData.GetStringField("endpoint")
	plan.AuthToken = notifyData.GetStringField("auth_token")
	plan.ClientCert = notifyData.GetStringField("client_cert")
	plan.ClientKey = notifyData.GetStringField("client_key")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioAuditWebhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioAuditWebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "endpoint", data.GetStringField("endpoint").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "auth_token", data.GetStringField("auth_token").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_cert", data.GetStringField("client_cert").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_key", data.GetStringField("client_key").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "queue_size", data.GetInt64Field("queue_size").ValueInt64())
		notifyFrameworkBuildCfgAddInt(&parts, "batch_size", data.GetInt64Field("batch_size").ValueInt64())
		if !data.Enable.IsNull() && !data.Enable.IsUnknown() {
			notifyFrameworkBuildCfgAddBool(&parts, "enable", data.Enable.ValueBool())
		}
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["endpoint"]; ok {
			data.SetStringField("endpoint", types.StringValue(v))
		}
		if v, ok := cfgMap["client_cert"]; ok {
			if v != "" {
				data.SetStringField("client_cert", types.StringValue(v))
			} else if !data.GetStringField("client_cert").IsUnknown() {
				data.SetStringField("client_cert", types.StringNull())
			}
		}
		if v, ok := cfgMap["queue_size"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				data.SetInt64Field("queue_size", types.Int64Value(int64(n)))
			}
		}
		if v, ok := cfgMap["batch_size"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				data.SetInt64Field("batch_size", types.Int64Value(int64(n)))
			}
		}
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "audit_webhook", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: state.Name, Enable: state.Enable, QueueLimit: state.QueueSize, Comment: types.String{}, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("endpoint", state.Endpoint)
	notifyData.SetStringField("auth_token", state.AuthToken)
	notifyData.SetStringField("client_cert", state.ClientCert)
	notifyData.SetStringField("client_key", state.ClientKey)
	notifyData.SetInt64Field("queue_size", state.QueueSize)
	notifyData.SetInt64Field("batch_size", state.BatchSize)
	diags := notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if notifyData.Name.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Name = notifyData.Name
	state.ID = state.Name
	state.Enable = notifyData.Enable
	state.QueueSize = notifyData.GetInt64Field("queue_size")
	state.BatchSize = notifyData.GetInt64Field("batch_size")
	state.RestartRequired = notifyData.RestartRequired
	state.Endpoint = notifyData.GetStringField("endpoint")
	state.AuthToken = notifyData.GetStringField("auth_token")
	state.ClientCert = notifyData.GetStringField("client_cert")
	state.ClientKey = notifyData.GetStringField("client_key")
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *minioAuditWebhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioAuditWebhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "endpoint", data.GetStringField("endpoint").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "auth_token", data.GetStringField("auth_token").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_cert", data.GetStringField("client_cert").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_key", data.GetStringField("client_key").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "queue_size", data.GetInt64Field("queue_size").ValueInt64())
		notifyFrameworkBuildCfgAddInt(&parts, "batch_size", data.GetInt64Field("batch_size").ValueInt64())
		if !data.Enable.IsNull() && !data.Enable.IsUnknown() {
			notifyFrameworkBuildCfgAddBool(&parts, "enable", data.Enable.ValueBool())
		}
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["endpoint"]; ok {
			data.SetStringField("endpoint", types.StringValue(v))
		}
		if v, ok := cfgMap["client_cert"]; ok {
			if v != "" {
				data.SetStringField("client_cert", types.StringValue(v))
			} else if !data.GetStringField("client_cert").IsUnknown() {
				data.SetStringField("client_cert", types.StringNull())
			}
		}
		if v, ok := cfgMap["queue_size"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				data.SetInt64Field("queue_size", types.Int64Value(int64(n)))
			}
		}
		if v, ok := cfgMap["batch_size"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				data.SetInt64Field("batch_size", types.Int64Value(int64(n)))
			}
		}
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "audit_webhook", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueLimit: plan.QueueSize, Comment: types.String{}, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("endpoint", plan.Endpoint)
	notifyData.SetStringField("auth_token", plan.AuthToken)
	notifyData.SetStringField("client_cert", plan.ClientCert)
	notifyData.SetStringField("client_key", plan.ClientKey)
	notifyData.SetInt64Field("queue_size", plan.QueueSize)
	notifyData.SetInt64Field("batch_size", plan.BatchSize)
	diags := notifyFrameworkUpdate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Name = notifyData.Name
	plan.ID = plan.Name
	plan.Enable = notifyData.Enable
	plan.QueueSize = notifyData.GetInt64Field("queue_size")
	plan.BatchSize = notifyData.GetInt64Field("batch_size")
	plan.RestartRequired = notifyData.RestartRequired
	plan.Endpoint = notifyData.GetStringField("endpoint")
	plan.AuthToken = notifyData.GetStringField("auth_token")
	plan.ClientCert = notifyData.GetStringField("client_cert")
	plan.ClientKey = notifyData.GetStringField("client_key")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioAuditWebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioAuditWebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := state.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting audit_webhook: %s", name))
	diags := notifyFrameworkDelete(ctx, r.client, "audit_webhook", name, &notifyFrameworkResourceData{Name: state.Name})
	resp.Diagnostics.Append(diags...)
}

func (r *minioAuditWebhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
