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
	_ resource.Resource                = &minioNotifyMqttResource{}
	_ resource.ResourceWithConfigure   = &minioNotifyMqttResource{}
	_ resource.ResourceWithImportState = &minioNotifyMqttResource{}
)

type minioNotifyMqttResource struct {
	client *S3MinioClient
}

type minioNotifyMqttResourceModel struct {
	Name                 types.String `tfsdk:"name"`
	Enable               types.Bool   `tfsdk:"enable"`
	QueueDir             types.String `tfsdk:"queue_dir"`
	QueueLimit           types.Int64  `tfsdk:"queue_limit"`
	Comment              types.String `tfsdk:"comment"`
	RestartRequired      types.Bool   `tfsdk:"restart_required"`
	Broker               types.String `tfsdk:"broker"`
	Topic                types.String `tfsdk:"topic"`
	Username             types.String `tfsdk:"username"`
	Password             types.String `tfsdk:"password"`
	Qos                  types.Int64  `tfsdk:"qos"`
	KeepAliveInterval    types.String `tfsdk:"keep_alive_interval"`
	ReconnectInterval    types.String `tfsdk:"reconnect_interval"`
}

func resourceMinioNotifyMqttFramework() resource.Resource {
	return &minioNotifyMqttResource{}
}

func (r *minioNotifyMqttResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notify_mqtt"
}

func (r *minioNotifyMqttResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil { return }
	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *S3MinioClient, got: %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *minioNotifyMqttResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an MQTT notification target for MinIO bucket event notifications.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{Required: true, Description: "Target name identifier.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable": schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether this notification target is enabled.", Default: booldefault.StaticBool(true)},
			"queue_dir": schema.StringAttribute{Optional: true, Description: "Directory path for persistent event store when the target is offline."},
			"queue_limit": schema.Int64Attribute{Optional: true, Computed: true, Description: "Maximum number of undelivered messages to queue."},
			"comment": schema.StringAttribute{Optional: true, Description: "Comment or description for this notification target."},
			"restart_required": schema.BoolAttribute{Computed: true, Description: "Indicates whether a MinIO server restart is required."},
			"broker": schema.StringAttribute{Required: true, Description: "MQTT broker URL (e.g., 'tcp://host:1883')."},
			"topic": schema.StringAttribute{Required: true, Description: "MQTT topic to publish event notifications to."},
			"username": schema.StringAttribute{Optional: true, Description: "Username for MQTT broker authentication."},
			"password": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Password for MQTT broker authentication. MinIO does not return this value on read; Terraform keeps the value from your configuration."},
			"qos": schema.Int64Attribute{Optional: true, Description: "MQTT Quality of Service level: 0 (at most once), 1 (at least once), or 2 (exactly once)."},
			"keep_alive_interval": schema.StringAttribute{Optional: true, Description: "MQTT keep-alive interval duration (e.g., '10s')."},
			"reconnect_interval": schema.StringAttribute{Optional: true, Description: "MQTT reconnect interval duration (e.g., '5s')."},
		},
	}
}

func (r *minioNotifyMqttResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioNotifyMqttResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }
	name := plan.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating notify_mqtt: %s", name))
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "broker", data.GetStringField("broker").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "topic", data.GetStringField("topic").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "username", data.GetStringField("username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "qos", data.GetInt64Field("qos").ValueInt64())
		notifyFrameworkBuildCfgAddParam(&parts, "keep_alive_interval", data.GetStringField("keep_alive_interval").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "reconnect_interval", data.GetStringField("reconnect_interval").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["broker"]; ok { data.SetStringField("broker", types.StringValue(v)) }
		if v, ok := cfgMap["topic"]; ok { data.SetStringField("topic", types.StringValue(v)) }
		if v, ok := cfgMap["username"]; ok { data.SetStringField("username", types.StringValue(v)) }
		if v, ok := cfgMap["qos"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("qos", types.Int64Value(int64(n))) } }
		if v, ok := cfgMap["keep_alive_interval"]; ok { data.SetStringField("keep_alive_interval", types.StringValue(v)) }
		if v, ok := cfgMap["reconnect_interval"]; ok { data.SetStringField("reconnect_interval", types.StringValue(v)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_mqtt", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("broker", plan.Broker); notifyData.SetStringField("topic", plan.Topic); notifyData.SetStringField("username", plan.Username); notifyData.SetStringField("password", plan.Password)
	notifyData.SetInt64Field("qos", plan.Qos); notifyData.SetStringField("keep_alive_interval", plan.KeepAliveInterval); notifyData.SetStringField("reconnect_interval", plan.ReconnectInterval)
	diags := notifyFrameworkCreate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	plan.Name = notifyData.Name; plan.Enable = notifyData.Enable; plan.QueueDir = notifyData.QueueDir; plan.QueueLimit = notifyData.QueueLimit; plan.Comment = notifyData.Comment; plan.RestartRequired = notifyData.RestartRequired
	plan.Broker = notifyData.GetStringField("broker"); plan.Topic = notifyData.GetStringField("topic"); plan.Username = notifyData.GetStringField("username"); plan.Password = notifyData.GetStringField("password")
	plan.Qos = notifyData.GetInt64Field("qos"); plan.KeepAliveInterval = notifyData.GetStringField("keep_alive_interval"); plan.ReconnectInterval = notifyData.GetStringField("reconnect_interval")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyMqttResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioNotifyMqttResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "broker", data.GetStringField("broker").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "topic", data.GetStringField("topic").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "username", data.GetStringField("username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "qos", data.GetInt64Field("qos").ValueInt64())
		notifyFrameworkBuildCfgAddParam(&parts, "keep_alive_interval", data.GetStringField("keep_alive_interval").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "reconnect_interval", data.GetStringField("reconnect_interval").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["broker"]; ok { data.SetStringField("broker", types.StringValue(v)) }
		if v, ok := cfgMap["topic"]; ok { data.SetStringField("topic", types.StringValue(v)) }
		if v, ok := cfgMap["username"]; ok { data.SetStringField("username", types.StringValue(v)) }
		if v, ok := cfgMap["qos"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("qos", types.Int64Value(int64(n))) } }
		if v, ok := cfgMap["keep_alive_interval"]; ok { data.SetStringField("keep_alive_interval", types.StringValue(v)) }
		if v, ok := cfgMap["reconnect_interval"]; ok { data.SetStringField("reconnect_interval", types.StringValue(v)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_mqtt", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: state.Name, Enable: state.Enable, QueueDir: state.QueueDir, QueueLimit: state.QueueLimit, Comment: state.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("broker", state.Broker); notifyData.SetStringField("topic", state.Topic); notifyData.SetStringField("username", state.Username); notifyData.SetStringField("password", state.Password)
	notifyData.SetInt64Field("qos", state.Qos); notifyData.SetStringField("keep_alive_interval", state.KeepAliveInterval); notifyData.SetStringField("reconnect_interval", state.ReconnectInterval)
	diags := notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	if notifyData.Name.IsNull() { resp.State.RemoveResource(ctx); return }
	state.Name = notifyData.Name; state.Enable = notifyData.Enable; state.QueueDir = notifyData.QueueDir; state.QueueLimit = notifyData.QueueLimit; state.Comment = notifyData.Comment; state.RestartRequired = notifyData.RestartRequired
	state.Broker = notifyData.GetStringField("broker"); state.Topic = notifyData.GetStringField("topic"); state.Username = notifyData.GetStringField("username"); state.Password = notifyData.GetStringField("password")
	state.Qos = notifyData.GetInt64Field("qos"); state.KeepAliveInterval = notifyData.GetStringField("keep_alive_interval"); state.ReconnectInterval = notifyData.GetStringField("reconnect_interval")
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyMqttResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioNotifyMqttResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "broker", data.GetStringField("broker").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "topic", data.GetStringField("topic").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "username", data.GetStringField("username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "qos", data.GetInt64Field("qos").ValueInt64())
		notifyFrameworkBuildCfgAddParam(&parts, "keep_alive_interval", data.GetStringField("keep_alive_interval").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "reconnect_interval", data.GetStringField("reconnect_interval").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["broker"]; ok { data.SetStringField("broker", types.StringValue(v)) }
		if v, ok := cfgMap["topic"]; ok { data.SetStringField("topic", types.StringValue(v)) }
		if v, ok := cfgMap["username"]; ok { data.SetStringField("username", types.StringValue(v)) }
		if v, ok := cfgMap["qos"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("qos", types.Int64Value(int64(n))) } }
		if v, ok := cfgMap["keep_alive_interval"]; ok { data.SetStringField("keep_alive_interval", types.StringValue(v)) }
		if v, ok := cfgMap["reconnect_interval"]; ok { data.SetStringField("reconnect_interval", types.StringValue(v)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_mqtt", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("broker", plan.Broker); notifyData.SetStringField("topic", plan.Topic); notifyData.SetStringField("username", plan.Username); notifyData.SetStringField("password", plan.Password)
	notifyData.SetInt64Field("qos", plan.Qos); notifyData.SetStringField("keep_alive_interval", plan.KeepAliveInterval); notifyData.SetStringField("reconnect_interval", plan.ReconnectInterval)
	diags := notifyFrameworkUpdate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	plan.Name = notifyData.Name; plan.Enable = notifyData.Enable; plan.QueueDir = notifyData.QueueDir; plan.QueueLimit = notifyData.QueueLimit; plan.Comment = notifyData.Comment; plan.RestartRequired = notifyData.RestartRequired
	plan.Broker = notifyData.GetStringField("broker"); plan.Topic = notifyData.GetStringField("topic"); plan.Username = notifyData.GetStringField("username"); plan.Password = notifyData.GetStringField("password")
	plan.Qos = notifyData.GetInt64Field("qos"); plan.KeepAliveInterval = notifyData.GetStringField("keep_alive_interval"); plan.ReconnectInterval = notifyData.GetStringField("reconnect_interval")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyMqttResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioNotifyMqttResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	name := state.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting notify_mqtt: %s", name))
	diags := notifyFrameworkDelete(ctx, r.client, "notify_mqtt", name, &notifyFrameworkResourceData{Name: state.Name})
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyMqttResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
