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
	_ resource.Resource                = &minioNotifyNsqResource{}
	_ resource.ResourceWithConfigure   = &minioNotifyNsqResource{}
	_ resource.ResourceWithImportState = &minioNotifyNsqResource{}
)

type minioNotifyNsqResource struct {
	client *S3MinioClient
}

type minioNotifyNsqResourceModel struct {
	Name            types.String `tfsdk:"name"`
	Enable          types.Bool   `tfsdk:"enable"`
	QueueDir        types.String `tfsdk:"queue_dir"`
	QueueLimit      types.Int64  `tfsdk:"queue_limit"`
	Comment         types.String `tfsdk:"comment"`
	RestartRequired types.Bool   `tfsdk:"restart_required"`
	NsqdAddress     types.String `tfsdk:"nsqd_address"`
	Topic           types.String `tfsdk:"topic"`
	TLS             types.Bool   `tfsdk:"tls"`
	TLSSkipVerify   types.Bool   `tfsdk:"tls_skip_verify"`
}

func resourceMinioNotifyNsqFramework() resource.Resource {
	return &minioNotifyNsqResource{}
}

func (r *minioNotifyNsqResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notify_nsq"
}

func (r *minioNotifyNsqResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil { return }
	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *S3MinioClient, got: %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *minioNotifyNsqResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an NSQ notification target for MinIO bucket event notifications.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{Required: true, Description: "Target name identifier.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable": schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether this notification target is enabled.", Default: booldefault.StaticBool(true)},
			"queue_dir": schema.StringAttribute{Optional: true, Description: "Directory path for persistent event store when the target is offline."},
			"queue_limit": schema.Int64Attribute{Optional: true, Computed: true, Description: "Maximum number of undelivered messages to queue."},
			"comment": schema.StringAttribute{Optional: true, Description: "Comment or description for this notification target."},
			"restart_required": schema.BoolAttribute{Computed: true, Description: "Indicates whether a MinIO server restart is required."},
			"nsqd_address": schema.StringAttribute{Required: true, Description: "NSQ daemon address (e.g., 'localhost:4150')."},
			"topic": schema.StringAttribute{Required: true, Description: "NSQ topic to publish notifications to."},
			"tls": schema.BoolAttribute{Optional: true, Description: "Whether to enable TLS for the NSQ connection."},
			"tls_skip_verify": schema.BoolAttribute{Optional: true, Description: "Whether to skip TLS certificate verification."},
		},
	}
}

func (r *minioNotifyNsqResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioNotifyNsqResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }
	name := plan.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating notify_nsq: %s", name))
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "nsqd_address", data.GetStringField("nsqd_address").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "topic", data.GetStringField("topic").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "tls", data.GetBoolField("tls").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "tls_skip_verify", data.GetBoolField("tls_skip_verify").ValueBool())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["nsqd_address"]; ok { data.SetStringField("nsqd_address", types.StringValue(v)) }
		if v, ok := cfgMap["topic"]; ok { data.SetStringField("topic", types.StringValue(v)) }
		if v, ok := cfgMap["tls"]; ok { data.SetBoolField("tls", types.BoolValue(v == "on")) } else { data.SetBoolField("tls", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_skip_verify"]; ok { data.SetBoolField("tls_skip_verify", types.BoolValue(v == "on")) } else { data.SetBoolField("tls_skip_verify", types.BoolValue(false)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_nsq", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("nsqd_address", plan.NsqdAddress)
	notifyData.SetStringField("topic", plan.Topic)
	notifyData.SetBoolField("tls", plan.TLS)
	notifyData.SetBoolField("tls_skip_verify", plan.TLSSkipVerify)
	diags := notifyFrameworkCreate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	plan.Name = notifyData.Name; plan.Enable = notifyData.Enable; plan.QueueDir = notifyData.QueueDir; plan.QueueLimit = notifyData.QueueLimit; plan.Comment = notifyData.Comment; plan.RestartRequired = notifyData.RestartRequired
	plan.NsqdAddress = notifyData.GetStringField("nsqd_address"); plan.Topic = notifyData.GetStringField("topic"); plan.TLS = notifyData.GetBoolField("tls"); plan.TLSSkipVerify = notifyData.GetBoolField("tls_skip_verify")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyNsqResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioNotifyNsqResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "nsqd_address", data.GetStringField("nsqd_address").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "topic", data.GetStringField("topic").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "tls", data.GetBoolField("tls").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "tls_skip_verify", data.GetBoolField("tls_skip_verify").ValueBool())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["nsqd_address"]; ok { data.SetStringField("nsqd_address", types.StringValue(v)) }
		if v, ok := cfgMap["topic"]; ok { data.SetStringField("topic", types.StringValue(v)) }
		if v, ok := cfgMap["tls"]; ok { data.SetBoolField("tls", types.BoolValue(v == "on")) } else { data.SetBoolField("tls", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_skip_verify"]; ok { data.SetBoolField("tls_skip_verify", types.BoolValue(v == "on")) } else { data.SetBoolField("tls_skip_verify", types.BoolValue(false)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_nsq", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: state.Name, Enable: state.Enable, QueueDir: state.QueueDir, QueueLimit: state.QueueLimit, Comment: state.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("nsqd_address", state.NsqdAddress); notifyData.SetStringField("topic", state.Topic); notifyData.SetBoolField("tls", state.TLS); notifyData.SetBoolField("tls_skip_verify", state.TLSSkipVerify)
	diags := notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	if notifyData.Name.IsNull() { resp.State.RemoveResource(ctx); return }
	state.Name = notifyData.Name; state.Enable = notifyData.Enable; state.QueueDir = notifyData.QueueDir; state.QueueLimit = notifyData.QueueLimit; state.Comment = notifyData.Comment; state.RestartRequired = notifyData.RestartRequired
	state.NsqdAddress = notifyData.GetStringField("nsqd_address"); state.Topic = notifyData.GetStringField("topic"); state.TLS = notifyData.GetBoolField("tls"); state.TLSSkipVerify = notifyData.GetBoolField("tls_skip_verify")
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyNsqResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioNotifyNsqResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "nsqd_address", data.GetStringField("nsqd_address").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "topic", data.GetStringField("topic").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "tls", data.GetBoolField("tls").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "tls_skip_verify", data.GetBoolField("tls_skip_verify").ValueBool())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["nsqd_address"]; ok { data.SetStringField("nsqd_address", types.StringValue(v)) }
		if v, ok := cfgMap["topic"]; ok { data.SetStringField("topic", types.StringValue(v)) }
		if v, ok := cfgMap["tls"]; ok { data.SetBoolField("tls", types.BoolValue(v == "on")) } else { data.SetBoolField("tls", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_skip_verify"]; ok { data.SetBoolField("tls_skip_verify", types.BoolValue(v == "on")) } else { data.SetBoolField("tls_skip_verify", types.BoolValue(false)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_nsq", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("nsqd_address", plan.NsqdAddress); notifyData.SetStringField("topic", plan.Topic); notifyData.SetBoolField("tls", plan.TLS); notifyData.SetBoolField("tls_skip_verify", plan.TLSSkipVerify)
	diags := notifyFrameworkUpdate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	plan.Name = notifyData.Name; plan.Enable = notifyData.Enable; plan.QueueDir = notifyData.QueueDir; plan.QueueLimit = notifyData.QueueLimit; plan.Comment = notifyData.Comment; plan.RestartRequired = notifyData.RestartRequired
	plan.NsqdAddress = notifyData.GetStringField("nsqd_address"); plan.Topic = notifyData.GetStringField("topic"); plan.TLS = notifyData.GetBoolField("tls"); plan.TLSSkipVerify = notifyData.GetBoolField("tls_skip_verify")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyNsqResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioNotifyNsqResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	name := state.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting notify_nsq: %s", name))
	diags := notifyFrameworkDelete(ctx, r.client, "notify_nsq", name, &notifyFrameworkResourceData{Name: state.Name})
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyNsqResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
