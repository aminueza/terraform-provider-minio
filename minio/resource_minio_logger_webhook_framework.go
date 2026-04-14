package minio

import (
	"context"
	"fmt"
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
	_ resource.Resource                = &minioLoggerWebhookResource{}
	_ resource.ResourceWithConfigure   = &minioLoggerWebhookResource{}
	_ resource.ResourceWithImportState = &minioLoggerWebhookResource{}
)

type minioLoggerWebhookResource struct {
	client *S3MinioClient
}

type minioLoggerWebhookResourceModel struct {
	Name            types.String `tfsdk:"name"`
	Enable          types.Bool   `tfsdk:"enable"`
	QueueDir        types.String `tfsdk:"queue_dir"`
	QueueLimit      types.Int64  `tfsdk:"queue_limit"`
	Comment         types.String `tfsdk:"comment"`
	RestartRequired types.Bool   `tfsdk:"restart_required"`
	Endpoint        types.String `tfsdk:"endpoint"`
	AuthToken       types.String `tfsdk:"auth_token"`
	BatchSize       types.Int64  `tfsdk:"batch_size"`
	ClientCert      types.String `tfsdk:"client_cert"`
	ClientKey       types.String `tfsdk:"client_key"`
	Proxy           types.String `tfsdk:"proxy"`
}

func resourceMinioLoggerWebhookFramework() resource.Resource {
	return &minioLoggerWebhookResource{}
}

func (r *minioLoggerWebhookResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_logger_webhook"
}

func (r *minioLoggerWebhookResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *minioLoggerWebhookResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a logger webhook target for MinIO system log forwarding. Logger webhooks send server log events to HTTP endpoints for centralized logging.",
		Attributes: map[string]schema.Attribute{
			"name":             schema.StringAttribute{Required: true, Description: "Unique name for the logger webhook target.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable":           schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether this logger webhook target is enabled.", Default: booldefault.StaticBool(true)},
			"queue_dir":        schema.StringAttribute{Optional: true, Description: "Directory path for persistent event store when the target is offline."},
			"queue_limit":      schema.Int64Attribute{Optional: true, Computed: true, Description: "Maximum number of undelivered messages to queue."},
			"comment":          schema.StringAttribute{Optional: true, Description: "Comment or description for this logger webhook target."},
			"restart_required": schema.BoolAttribute{Computed: true, Description: "Indicates whether a MinIO server restart is required."},
			"endpoint":         schema.StringAttribute{Required: true, Description: "HTTP(S) endpoint URL to send log events to."},
			"auth_token":       schema.StringAttribute{Optional: true, Sensitive: true, Description: "Authentication token for the endpoint."},
			"batch_size":       schema.Int64Attribute{Optional: true, Computed: true, Description: "Number of log events per batch."},
			"client_cert":      schema.StringAttribute{Optional: true, Description: "Path to X.509 client certificate for mTLS."},
			"client_key":       schema.StringAttribute{Optional: true, Sensitive: true, Description: "Path to X.509 private key for mTLS."},
			"proxy":            schema.StringAttribute{Optional: true, Description: "Proxy URL for the webhook endpoint."},
		},
	}
}

func (r *minioLoggerWebhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioLoggerWebhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := plan.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating logger_webhook: %s", name))
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "endpoint", data.GetStringField("endpoint").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "auth_token", data.GetStringField("auth_token").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_cert", data.GetStringField("client_cert").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_key", data.GetStringField("client_key").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "proxy", data.GetStringField("proxy").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "batch_size", data.GetInt64Field("batch_size").ValueInt64())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["endpoint"]; ok {
			data.SetStringField("endpoint", types.StringValue(v))
		}
		if v, ok := cfgMap["client_cert"]; ok {
			data.SetStringField("client_cert", types.StringValue(v))
		}
		if v, ok := cfgMap["proxy"]; ok {
			data.SetStringField("proxy", types.StringValue(v))
		}
		if v, ok := cfgMap["batch_size"]; ok {
			if n, err := parseInt(v); err == nil {
				data.SetInt64Field("batch_size", types.Int64Value(int64(n)))
			}
		}
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "logger_webhook", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("endpoint", plan.Endpoint)
	notifyData.SetStringField("auth_token", plan.AuthToken)
	notifyData.SetInt64Field("batch_size", plan.BatchSize)
	notifyData.SetStringField("client_cert", plan.ClientCert)
	notifyData.SetStringField("client_key", plan.ClientKey)
	notifyData.SetStringField("proxy", plan.Proxy)
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
	plan.Enable = notifyData.Enable
	plan.QueueDir = notifyData.QueueDir
	plan.QueueLimit = notifyData.QueueLimit
	plan.Comment = notifyData.Comment
	plan.RestartRequired = notifyData.RestartRequired
	plan.Endpoint = notifyData.GetStringField("endpoint")
	plan.AuthToken = notifyData.GetStringField("auth_token")
	plan.BatchSize = notifyData.GetInt64Field("batch_size")
	plan.ClientCert = notifyData.GetStringField("client_cert")
	plan.ClientKey = notifyData.GetStringField("client_key")
	plan.Proxy = notifyData.GetStringField("proxy")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioLoggerWebhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioLoggerWebhookResourceModel
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
		notifyFrameworkBuildCfgAddParam(&parts, "proxy", data.GetStringField("proxy").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "batch_size", data.GetInt64Field("batch_size").ValueInt64())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["endpoint"]; ok {
			data.SetStringField("endpoint", types.StringValue(v))
		}
		if v, ok := cfgMap["client_cert"]; ok {
			data.SetStringField("client_cert", types.StringValue(v))
		}
		if v, ok := cfgMap["proxy"]; ok {
			data.SetStringField("proxy", types.StringValue(v))
		}
		if v, ok := cfgMap["batch_size"]; ok {
			if n, err := parseInt(v); err == nil {
				data.SetInt64Field("batch_size", types.Int64Value(int64(n)))
			}
		}
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "logger_webhook", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: state.Name, Enable: state.Enable, QueueDir: state.QueueDir, QueueLimit: state.QueueLimit, Comment: state.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("endpoint", state.Endpoint)
	notifyData.SetStringField("auth_token", state.AuthToken)
	notifyData.SetInt64Field("batch_size", state.BatchSize)
	notifyData.SetStringField("client_cert", state.ClientCert)
	notifyData.SetStringField("client_key", state.ClientKey)
	notifyData.SetStringField("proxy", state.Proxy)
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
	state.Enable = notifyData.Enable
	state.QueueDir = notifyData.QueueDir
	state.QueueLimit = notifyData.QueueLimit
	state.Comment = notifyData.Comment
	state.RestartRequired = notifyData.RestartRequired
	state.Endpoint = notifyData.GetStringField("endpoint")
	state.AuthToken = notifyData.GetStringField("auth_token")
	state.BatchSize = notifyData.GetInt64Field("batch_size")
	state.ClientCert = notifyData.GetStringField("client_cert")
	state.ClientKey = notifyData.GetStringField("client_key")
	state.Proxy = notifyData.GetStringField("proxy")
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *minioLoggerWebhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioLoggerWebhookResourceModel
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
		notifyFrameworkBuildCfgAddParam(&parts, "proxy", data.GetStringField("proxy").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "batch_size", data.GetInt64Field("batch_size").ValueInt64())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["endpoint"]; ok {
			data.SetStringField("endpoint", types.StringValue(v))
		}
		if v, ok := cfgMap["client_cert"]; ok {
			data.SetStringField("client_cert", types.StringValue(v))
		}
		if v, ok := cfgMap["proxy"]; ok {
			data.SetStringField("proxy", types.StringValue(v))
		}
		if v, ok := cfgMap["batch_size"]; ok {
			if n, err := parseInt(v); err == nil {
				data.SetInt64Field("batch_size", types.Int64Value(int64(n)))
			}
		}
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "logger_webhook", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("endpoint", plan.Endpoint)
	notifyData.SetStringField("auth_token", plan.AuthToken)
	notifyData.SetInt64Field("batch_size", plan.BatchSize)
	notifyData.SetStringField("client_cert", plan.ClientCert)
	notifyData.SetStringField("client_key", plan.ClientKey)
	notifyData.SetStringField("proxy", plan.Proxy)
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
	plan.Enable = notifyData.Enable
	plan.QueueDir = notifyData.QueueDir
	plan.QueueLimit = notifyData.QueueLimit
	plan.Comment = notifyData.Comment
	plan.RestartRequired = notifyData.RestartRequired
	plan.Endpoint = notifyData.GetStringField("endpoint")
	plan.AuthToken = notifyData.GetStringField("auth_token")
	plan.BatchSize = notifyData.GetInt64Field("batch_size")
	plan.ClientCert = notifyData.GetStringField("client_cert")
	plan.ClientKey = notifyData.GetStringField("client_key")
	plan.Proxy = notifyData.GetStringField("proxy")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioLoggerWebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioLoggerWebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := state.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting logger_webhook: %s", name))
	diags := notifyFrameworkDelete(ctx, r.client, "logger_webhook", name, &notifyFrameworkResourceData{Name: state.Name})
	resp.Diagnostics.Append(diags...)
}

func (r *minioLoggerWebhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
