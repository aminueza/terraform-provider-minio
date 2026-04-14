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
	_ resource.Resource                = &minioNotifyRedisResource{}
	_ resource.ResourceWithConfigure   = &minioNotifyRedisResource{}
	_ resource.ResourceWithImportState = &minioNotifyRedisResource{}
)

type minioNotifyRedisResource struct {
	client *S3MinioClient
}

type minioNotifyRedisResourceModel struct {
	Name            types.String `tfsdk:"name"`
	Enable          types.Bool   `tfsdk:"enable"`
	QueueDir        types.String `tfsdk:"queue_dir"`
	QueueLimit      types.Int64  `tfsdk:"queue_limit"`
	Comment         types.String `tfsdk:"comment"`
	RestartRequired types.Bool   `tfsdk:"restart_required"`
	Address         types.String `tfsdk:"address"`
	Key             types.String `tfsdk:"key"`
	Format          types.String `tfsdk:"format"`
	Password        types.String `tfsdk:"password"`
	User            types.String `tfsdk:"user"`
}

func resourceMinioNotifyRedisFramework() resource.Resource {
	return &minioNotifyRedisResource{}
}

func (r *minioNotifyRedisResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notify_redis"
}

func (r *minioNotifyRedisResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *minioNotifyRedisResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Redis notification target for MinIO bucket event notifications.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{Required: true, Description: "Target name identifier.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable": schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether this notification target is enabled.", Default: booldefault.StaticBool(true)},
			"queue_dir": schema.StringAttribute{Optional: true, Description: "Directory path for persistent event store when the target is offline."},
			"queue_limit": schema.Int64Attribute{Optional: true, Computed: true, Description: "Maximum number of undelivered messages to queue."},
			"comment": schema.StringAttribute{Optional: true, Description: "Comment or description for this notification target."},
			"restart_required": schema.BoolAttribute{Computed: true, Description: "Indicates whether a MinIO server restart is required."},
			"address": schema.StringAttribute{Required: true, Description: "Redis server address (e.g., 'localhost:6379')."},
			"key": schema.StringAttribute{Required: true, Description: "Redis key name used to store or publish event records."},
			"format": schema.StringAttribute{Required: true, Description: "Output format for event records: 'namespace' or 'access'."},
			"password": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Password for Redis authentication. MinIO does not return this value on read; Terraform keeps the value from your configuration."},
			"user": schema.StringAttribute{Optional: true, Description: "Username for Redis ACL authentication."},
		},
	}
}

func (r *minioNotifyRedisResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioNotifyRedisResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }
	name := plan.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating notify_redis: %s", name))
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "address", data.GetStringField("address").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "key", data.GetStringField("key").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "format", data.GetStringField("format").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "user", data.GetStringField("user").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["address"]; ok && v != "" { data.SetStringField("address", types.StringValue(v)) }
		if v, ok := cfgMap["key"]; ok && v != "" { data.SetStringField("key", types.StringValue(v)) }
		if v, ok := cfgMap["format"]; ok && v != "" { data.SetStringField("format", types.StringValue(v)) }
		if v, ok := cfgMap["user"]; ok && v != "" { data.SetStringField("user", types.StringValue(v)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_redis", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("address", plan.Address)
	notifyData.SetStringField("key", plan.Key)
	notifyData.SetStringField("format", plan.Format)
	notifyData.SetStringField("password", plan.Password)
	notifyData.SetStringField("user", plan.User)
	diags := notifyFrameworkCreate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	plan.Name = notifyData.Name; plan.Enable = notifyData.Enable; plan.QueueDir = notifyData.QueueDir; plan.QueueLimit = notifyData.QueueLimit; plan.Comment = notifyData.Comment; plan.RestartRequired = notifyData.RestartRequired
	plan.Address = notifyData.GetStringField("address"); plan.Key = notifyData.GetStringField("key"); plan.Format = notifyData.GetStringField("format"); plan.Password = notifyData.GetStringField("password"); plan.User = notifyData.GetStringField("user")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyRedisResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioNotifyRedisResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "address", data.GetStringField("address").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "key", data.GetStringField("key").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "format", data.GetStringField("format").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "user", data.GetStringField("user").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["address"]; ok && v != "" { data.SetStringField("address", types.StringValue(v)) }
		if v, ok := cfgMap["key"]; ok && v != "" { data.SetStringField("key", types.StringValue(v)) }
		if v, ok := cfgMap["format"]; ok && v != "" { data.SetStringField("format", types.StringValue(v)) }
		if v, ok := cfgMap["user"]; ok && v != "" { data.SetStringField("user", types.StringValue(v)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_redis", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: state.Name, Enable: state.Enable, QueueDir: state.QueueDir, QueueLimit: state.QueueLimit, Comment: state.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("address", state.Address); notifyData.SetStringField("key", state.Key); notifyData.SetStringField("format", state.Format); notifyData.SetStringField("password", state.Password); notifyData.SetStringField("user", state.User)
	diags := notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	if notifyData.Name.IsNull() { resp.State.RemoveResource(ctx); return }
	state.Name = notifyData.Name; state.Enable = notifyData.Enable; state.QueueDir = notifyData.QueueDir; state.QueueLimit = notifyData.QueueLimit; state.Comment = notifyData.Comment; state.RestartRequired = notifyData.RestartRequired
	state.Address = notifyData.GetStringField("address"); state.Key = notifyData.GetStringField("key"); state.Format = notifyData.GetStringField("format"); state.Password = notifyData.GetStringField("password"); state.User = notifyData.GetStringField("user")
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyRedisResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioNotifyRedisResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "address", data.GetStringField("address").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "key", data.GetStringField("key").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "format", data.GetStringField("format").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "user", data.GetStringField("user").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["address"]; ok && v != "" { data.SetStringField("address", types.StringValue(v)) }
		if v, ok := cfgMap["key"]; ok && v != "" { data.SetStringField("key", types.StringValue(v)) }
		if v, ok := cfgMap["format"]; ok && v != "" { data.SetStringField("format", types.StringValue(v)) }
		if v, ok := cfgMap["user"]; ok && v != "" { data.SetStringField("user", types.StringValue(v)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_redis", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("address", plan.Address); notifyData.SetStringField("key", plan.Key); notifyData.SetStringField("format", plan.Format); notifyData.SetStringField("password", plan.Password); notifyData.SetStringField("user", plan.User)
	diags := notifyFrameworkUpdate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	plan.Name = notifyData.Name; plan.Enable = notifyData.Enable; plan.QueueDir = notifyData.QueueDir; plan.QueueLimit = notifyData.QueueLimit; plan.Comment = notifyData.Comment; plan.RestartRequired = notifyData.RestartRequired
	plan.Address = notifyData.GetStringField("address"); plan.Key = notifyData.GetStringField("key"); plan.Format = notifyData.GetStringField("format"); plan.Password = notifyData.GetStringField("password"); plan.User = notifyData.GetStringField("user")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyRedisResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioNotifyRedisResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	name := state.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting notify_redis: %s", name))
	diags := notifyFrameworkDelete(ctx, r.client, "notify_redis", name, &notifyFrameworkResourceData{Name: state.Name})
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyRedisResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
