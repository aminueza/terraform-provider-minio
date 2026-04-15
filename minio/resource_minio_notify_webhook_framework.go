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
	_ resource.Resource                = &minioNotifyWebhookResource{}
	_ resource.ResourceWithConfigure   = &minioNotifyWebhookResource{}
	_ resource.ResourceWithImportState = &minioNotifyWebhookResource{}
)

type minioNotifyWebhookResource struct {
	client *S3MinioClient
}

type minioNotifyWebhookResourceModel struct {
	Name            types.String `tfsdk:"name"`
	Enable          types.Bool   `tfsdk:"enable"`
	QueueDir        types.String `tfsdk:"queue_dir"`
	QueueLimit      types.Int64  `tfsdk:"queue_limit"`
	Comment         types.String `tfsdk:"comment"`
	RestartRequired types.Bool   `tfsdk:"restart_required"`
	Endpoint        types.String `tfsdk:"endpoint"`
	AuthToken       types.String `tfsdk:"auth_token"`
	ClientCert      types.String `tfsdk:"client_cert"`
	ClientKey       types.String `tfsdk:"client_key"`
}

func resourceMinioNotifyWebhookFramework() resource.Resource {
	return &minioNotifyWebhookResource{}
}

func (r *minioNotifyWebhookResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notify_webhook"
}

func (r *minioNotifyWebhookResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *minioNotifyWebhookResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a webhook notification target for MinIO bucket event notifications. Webhook targets receive bucket events (object created, deleted, etc.) via HTTP POST requests.",
		Attributes: map[string]schema.Attribute{
			"name":             schema.StringAttribute{Required: true, Description: "Unique name for the webhook notification target.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable":           schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether this notification target is enabled.", Default: booldefault.StaticBool(true)},
			"queue_dir":        schema.StringAttribute{Optional: true, Description: "Directory path for persistent event store when the target is offline."},
			"queue_limit":      schema.Int64Attribute{Optional: true, Computed: true, Description: "Maximum size of the queue for outgoing messages."},
			"comment":          schema.StringAttribute{Optional: true, Description: "Comment or description for this notification target."},
			"restart_required": schema.BoolAttribute{Computed: true, Description: "Indicates whether a MinIO server restart is required."},
			"endpoint":         schema.StringAttribute{Required: true, Description: "HTTP(S) endpoint URL to send bucket event notifications to."},
			"auth_token":       schema.StringAttribute{Optional: true, Sensitive: true, Description: "Authentication token for the webhook endpoint. MinIO does not return this value on read."},
			"client_cert":      schema.StringAttribute{Optional: true, Description: "Path to the X.509 client certificate for mTLS authentication."},
			"client_key":       schema.StringAttribute{Optional: true, Sensitive: true, Description: "Path to the X.509 private key for mTLS authentication. MinIO does not return this value on read."},
		},
	}
}

func (r *minioNotifyWebhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioNotifyWebhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := plan.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating notify_webhook: %s", name))
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "endpoint", data.GetStringField("endpoint").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "auth_token", data.GetStringField("auth_token").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_cert", data.GetStringField("client_cert").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_key", data.GetStringField("client_key").ValueString())
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
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_webhook", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("endpoint", plan.Endpoint)
	notifyData.SetStringField("auth_token", plan.AuthToken)
	notifyData.SetStringField("client_cert", plan.ClientCert)
	notifyData.SetStringField("client_key", plan.ClientKey)
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
	if notifyData.QueueLimit.IsUnknown() {
		plan.QueueLimit = plan.QueueLimit
	} else {
		plan.QueueLimit = notifyData.QueueLimit
	}
	plan.Comment = notifyData.Comment
	plan.RestartRequired = notifyData.RestartRequired
	plan.Endpoint = notifyData.GetStringField("endpoint")
	plan.AuthToken = notifyData.GetStringField("auth_token")
	plan.ClientCert = notifyData.GetStringField("client_cert")
	plan.ClientKey = notifyData.GetStringField("client_key")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyWebhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioNotifyWebhookResourceModel
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
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_webhook", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: state.Name, Enable: state.Enable, QueueDir: state.QueueDir, QueueLimit: state.QueueLimit, Comment: state.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("endpoint", state.Endpoint)
	notifyData.SetStringField("auth_token", state.AuthToken)
	notifyData.SetStringField("client_cert", state.ClientCert)
	notifyData.SetStringField("client_key", state.ClientKey)
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
	if notifyData.QueueLimit.IsNull() || notifyData.QueueLimit.IsUnknown() {
		state.QueueLimit = state.QueueLimit
	} else {
		state.QueueLimit = notifyData.QueueLimit
	}
	state.Comment = notifyData.Comment
	state.RestartRequired = notifyData.RestartRequired
	state.Endpoint = notifyData.GetStringField("endpoint")
	state.AuthToken = notifyData.GetStringField("auth_token")
	state.ClientCert = notifyData.GetStringField("client_cert")
	state.ClientKey = notifyData.GetStringField("client_key")
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyWebhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioNotifyWebhookResourceModel
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
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_webhook", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("endpoint", plan.Endpoint)
	notifyData.SetStringField("auth_token", plan.AuthToken)
	notifyData.SetStringField("client_cert", plan.ClientCert)
	notifyData.SetStringField("client_key", plan.ClientKey)
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
	if notifyData.QueueLimit.IsUnknown() {
		plan.QueueLimit = plan.QueueLimit
	} else {
		plan.QueueLimit = notifyData.QueueLimit
	}
	plan.Comment = notifyData.Comment
	plan.RestartRequired = notifyData.RestartRequired
	plan.Endpoint = notifyData.GetStringField("endpoint")
	plan.AuthToken = notifyData.GetStringField("auth_token")
	plan.ClientCert = notifyData.GetStringField("client_cert")
	plan.ClientKey = notifyData.GetStringField("client_key")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyWebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioNotifyWebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := state.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting notify_webhook: %s", name))
	diags := notifyFrameworkDelete(ctx, r.client, "notify_webhook", name, &notifyFrameworkResourceData{Name: state.Name})
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyWebhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
