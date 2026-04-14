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
	_ resource.Resource                = &minioNotifyKafkaResource{}
	_ resource.ResourceWithConfigure   = &minioNotifyKafkaResource{}
	_ resource.ResourceWithImportState = &minioNotifyKafkaResource{}
)

type minioNotifyKafkaResource struct {
	client *S3MinioClient
}

type minioNotifyKafkaResourceModel struct {
	Name               types.String `tfsdk:"name"`
	Enable             types.Bool   `tfsdk:"enable"`
	QueueDir           types.String `tfsdk:"queue_dir"`
	QueueLimit         types.Int64  `tfsdk:"queue_limit"`
	Comment            types.String `tfsdk:"comment"`
	RestartRequired    types.Bool   `tfsdk:"restart_required"`
	Brokers            types.String `tfsdk:"brokers"`
	Topic              types.String `tfsdk:"topic"`
	SaslUsername       types.String `tfsdk:"sasl_username"`
	SaslPassword       types.String `tfsdk:"sasl_password"`
	SaslMechanism      types.String `tfsdk:"sasl_mechanism"`
	TLS                types.Bool   `tfsdk:"tls"`
	TLSSkipVerify      types.Bool   `tfsdk:"tls_skip_verify"`
	TLSClientAuth      types.Int64  `tfsdk:"tls_client_auth"`
	ClientTLSCert      types.String `tfsdk:"client_tls_cert"`
	ClientTLSKey       types.String `tfsdk:"client_tls_key"`
	Version            types.String `tfsdk:"version"`
	BatchSize          types.Int64  `tfsdk:"batch_size"`
}

func resourceMinioNotifyKafkaFramework() resource.Resource {
	return &minioNotifyKafkaResource{}
}

func (r *minioNotifyKafkaResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notify_kafka"
}

func (r *minioNotifyKafkaResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil { return }
	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *S3MinioClient, got: %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *minioNotifyKafkaResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Kafka notification target for MinIO bucket event notifications.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{Required: true, Description: "Target name identifier.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable": schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether this notification target is enabled.", Default: booldefault.StaticBool(true)},
			"queue_dir": schema.StringAttribute{Optional: true, Description: "Directory path for persistent event store when the target is offline."},
			"queue_limit": schema.Int64Attribute{Optional: true, Computed: true, Description: "Maximum number of undelivered messages to queue."},
			"comment": schema.StringAttribute{Optional: true, Description: "Comment or description for this notification target."},
			"restart_required": schema.BoolAttribute{Computed: true, Description: "Indicates whether a MinIO server restart is required."},
			"brokers": schema.StringAttribute{Required: true, Description: "Comma-separated list of Kafka broker addresses (e.g., 'host1:9092,host2:9092')."},
			"topic": schema.StringAttribute{Required: true, Description: "Kafka topic to publish event notifications to."},
			"sasl_username": schema.StringAttribute{Optional: true, Description: "SASL username for Kafka authentication."},
			"sasl_password": schema.StringAttribute{Optional: true, Sensitive: true, Description: "SASL password for Kafka authentication. MinIO does not return this value on read."},
			"sasl_mechanism": schema.StringAttribute{Optional: true, Description: "SASL authentication mechanism: 'plain', 'scram-sha-256', or 'scram-sha-512'."},
			"tls": schema.BoolAttribute{Optional: true, Description: "Whether to enable TLS for the Kafka connection."},
			"tls_skip_verify": schema.BoolAttribute{Optional: true, Description: "Whether to skip TLS certificate verification."},
			"tls_client_auth": schema.Int64Attribute{Optional: true, Description: "TLS client authentication type (0=NoClientCert, 1=RequestClientCert, etc.)."},
			"client_tls_cert": schema.StringAttribute{Optional: true, Description: "Path to the client TLS certificate for mTLS authentication."},
			"client_tls_key": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Path to the client TLS private key. MinIO does not return this value on read."},
			"version": schema.StringAttribute{Optional: true, Description: "Kafka cluster version (e.g., '2.8.0')."},
			"batch_size": schema.Int64Attribute{Optional: true, Computed: true, Description: "Number of messages to batch before sending to Kafka."},
		},
	}
}

func (r *minioNotifyKafkaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioNotifyKafkaResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }
	name := plan.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating notify_kafka: %s", name))
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "brokers", data.GetStringField("brokers").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "topic", data.GetStringField("topic").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "sasl_username", data.GetStringField("sasl_username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "sasl_password", data.GetStringField("sasl_password").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "sasl_mechanism", data.GetStringField("sasl_mechanism").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "tls", data.GetBoolField("tls").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "tls_skip_verify", data.GetBoolField("tls_skip_verify").ValueBool())
		notifyFrameworkBuildCfgAddInt(&parts, "tls_client_auth", data.GetInt64Field("tls_client_auth").ValueInt64())
		notifyFrameworkBuildCfgAddParam(&parts, "client_tls_cert", data.GetStringField("client_tls_cert").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_tls_key", data.GetStringField("client_tls_key").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "version", data.GetStringField("version").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "batch_size", data.GetInt64Field("batch_size").ValueInt64())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["brokers"]; ok { data.SetStringField("brokers", types.StringValue(v)) }
		if v, ok := cfgMap["topic"]; ok { data.SetStringField("topic", types.StringValue(v)) }
		if v, ok := cfgMap["sasl_username"]; ok { data.SetStringField("sasl_username", types.StringValue(v)) }
		if v, ok := cfgMap["sasl_mechanism"]; ok { data.SetStringField("sasl_mechanism", types.StringValue(v)) }
		if v, ok := cfgMap["tls"]; ok { data.SetBoolField("tls", types.BoolValue(v == "on")) } else { data.SetBoolField("tls", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_skip_verify"]; ok { data.SetBoolField("tls_skip_verify", types.BoolValue(v == "on")) } else { data.SetBoolField("tls_skip_verify", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_client_auth"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("tls_client_auth", types.Int64Value(int64(n))) } }
		if v, ok := cfgMap["client_tls_cert"]; ok { data.SetStringField("client_tls_cert", types.StringValue(v)) }
		if v, ok := cfgMap["version"]; ok { data.SetStringField("version", types.StringValue(v)) }
		if v, ok := cfgMap["batch_size"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("batch_size", types.Int64Value(int64(n))) } }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_kafka", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("brokers", plan.Brokers); notifyData.SetStringField("topic", plan.Topic)
	notifyData.SetStringField("sasl_username", plan.SaslUsername); notifyData.SetStringField("sasl_password", plan.SaslPassword)
	notifyData.SetStringField("sasl_mechanism", plan.SaslMechanism); notifyData.SetBoolField("tls", plan.TLS)
	notifyData.SetBoolField("tls_skip_verify", plan.TLSSkipVerify); notifyData.SetInt64Field("tls_client_auth", plan.TLSClientAuth)
	notifyData.SetStringField("client_tls_cert", plan.ClientTLSCert); notifyData.SetStringField("client_tls_key", plan.ClientTLSKey)
	notifyData.SetStringField("version", plan.Version); notifyData.SetInt64Field("batch_size", plan.BatchSize)
	diags := notifyFrameworkCreate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	plan.Name = notifyData.Name; plan.Enable = notifyData.Enable; plan.QueueDir = notifyData.QueueDir; plan.QueueLimit = notifyData.QueueLimit; plan.Comment = notifyData.Comment; plan.RestartRequired = notifyData.RestartRequired
	plan.Brokers = notifyData.GetStringField("brokers"); plan.Topic = notifyData.GetStringField("topic")
	plan.SaslUsername = notifyData.GetStringField("sasl_username"); plan.SaslPassword = notifyData.GetStringField("sasl_password")
	plan.SaslMechanism = notifyData.GetStringField("sasl_mechanism"); plan.TLS = notifyData.GetBoolField("tls")
	plan.TLSSkipVerify = notifyData.GetBoolField("tls_skip_verify"); plan.TLSClientAuth = notifyData.GetInt64Field("tls_client_auth")
	plan.ClientTLSCert = notifyData.GetStringField("client_tls_cert"); plan.ClientTLSKey = notifyData.GetStringField("client_tls_key")
	plan.Version = notifyData.GetStringField("version"); plan.BatchSize = notifyData.GetInt64Field("batch_size")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyKafkaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioNotifyKafkaResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "brokers", data.GetStringField("brokers").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "topic", data.GetStringField("topic").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "sasl_username", data.GetStringField("sasl_username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "sasl_password", data.GetStringField("sasl_password").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "sasl_mechanism", data.GetStringField("sasl_mechanism").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "tls", data.GetBoolField("tls").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "tls_skip_verify", data.GetBoolField("tls_skip_verify").ValueBool())
		notifyFrameworkBuildCfgAddInt(&parts, "tls_client_auth", data.GetInt64Field("tls_client_auth").ValueInt64())
		notifyFrameworkBuildCfgAddParam(&parts, "client_tls_cert", data.GetStringField("client_tls_cert").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_tls_key", data.GetStringField("client_tls_key").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "version", data.GetStringField("version").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "batch_size", data.GetInt64Field("batch_size").ValueInt64())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["brokers"]; ok { data.SetStringField("brokers", types.StringValue(v)) }
		if v, ok := cfgMap["topic"]; ok { data.SetStringField("topic", types.StringValue(v)) }
		if v, ok := cfgMap["sasl_username"]; ok { data.SetStringField("sasl_username", types.StringValue(v)) }
		if v, ok := cfgMap["sasl_mechanism"]; ok { data.SetStringField("sasl_mechanism", types.StringValue(v)) }
		if v, ok := cfgMap["tls"]; ok { data.SetBoolField("tls", types.BoolValue(v == "on")) } else { data.SetBoolField("tls", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_skip_verify"]; ok { data.SetBoolField("tls_skip_verify", types.BoolValue(v == "on")) } else { data.SetBoolField("tls_skip_verify", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_client_auth"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("tls_client_auth", types.Int64Value(int64(n))) } }
		if v, ok := cfgMap["client_tls_cert"]; ok { data.SetStringField("client_tls_cert", types.StringValue(v)) }
		if v, ok := cfgMap["version"]; ok { data.SetStringField("version", types.StringValue(v)) }
		if v, ok := cfgMap["batch_size"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("batch_size", types.Int64Value(int64(n))) } }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_kafka", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: state.Name, Enable: state.Enable, QueueDir: state.QueueDir, QueueLimit: state.QueueLimit, Comment: state.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("brokers", state.Brokers); notifyData.SetStringField("topic", state.Topic)
	notifyData.SetStringField("sasl_username", state.SaslUsername); notifyData.SetStringField("sasl_password", state.SaslPassword)
	notifyData.SetStringField("sasl_mechanism", state.SaslMechanism); notifyData.SetBoolField("tls", state.TLS)
	notifyData.SetBoolField("tls_skip_verify", state.TLSSkipVerify); notifyData.SetInt64Field("tls_client_auth", state.TLSClientAuth)
	notifyData.SetStringField("client_tls_cert", state.ClientTLSCert); notifyData.SetStringField("client_tls_key", state.ClientTLSKey)
	notifyData.SetStringField("version", state.Version); notifyData.SetInt64Field("batch_size", state.BatchSize)
	diags := notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	if notifyData.Name.IsNull() { resp.State.RemoveResource(ctx); return }
	state.Name = notifyData.Name; state.Enable = notifyData.Enable; state.QueueDir = notifyData.QueueDir; state.QueueLimit = notifyData.QueueLimit; state.Comment = notifyData.Comment; state.RestartRequired = notifyData.RestartRequired
	state.Brokers = notifyData.GetStringField("brokers"); state.Topic = notifyData.GetStringField("topic")
	state.SaslUsername = notifyData.GetStringField("sasl_username"); state.SaslPassword = notifyData.GetStringField("sasl_password")
	state.SaslMechanism = notifyData.GetStringField("sasl_mechanism"); state.TLS = notifyData.GetBoolField("tls")
	state.TLSSkipVerify = notifyData.GetBoolField("tls_skip_verify"); state.TLSClientAuth = notifyData.GetInt64Field("tls_client_auth")
	state.ClientTLSCert = notifyData.GetStringField("client_tls_cert"); state.ClientTLSKey = notifyData.GetStringField("client_tls_key")
	state.Version = notifyData.GetStringField("version"); state.BatchSize = notifyData.GetInt64Field("batch_size")
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyKafkaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioNotifyKafkaResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "brokers", data.GetStringField("brokers").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "topic", data.GetStringField("topic").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "sasl_username", data.GetStringField("sasl_username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "sasl_password", data.GetStringField("sasl_password").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "sasl_mechanism", data.GetStringField("sasl_mechanism").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "tls", data.GetBoolField("tls").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "tls_skip_verify", data.GetBoolField("tls_skip_verify").ValueBool())
		notifyFrameworkBuildCfgAddInt(&parts, "tls_client_auth", data.GetInt64Field("tls_client_auth").ValueInt64())
		notifyFrameworkBuildCfgAddParam(&parts, "client_tls_cert", data.GetStringField("client_tls_cert").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_tls_key", data.GetStringField("client_tls_key").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "version", data.GetStringField("version").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "batch_size", data.GetInt64Field("batch_size").ValueInt64())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["brokers"]; ok { data.SetStringField("brokers", types.StringValue(v)) }
		if v, ok := cfgMap["topic"]; ok { data.SetStringField("topic", types.StringValue(v)) }
		if v, ok := cfgMap["sasl_username"]; ok { data.SetStringField("sasl_username", types.StringValue(v)) }
		if v, ok := cfgMap["sasl_mechanism"]; ok { data.SetStringField("sasl_mechanism", types.StringValue(v)) }
		if v, ok := cfgMap["tls"]; ok { data.SetBoolField("tls", types.BoolValue(v == "on")) } else { data.SetBoolField("tls", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_skip_verify"]; ok { data.SetBoolField("tls_skip_verify", types.BoolValue(v == "on")) } else { data.SetBoolField("tls_skip_verify", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_client_auth"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("tls_client_auth", types.Int64Value(int64(n))) } }
		if v, ok := cfgMap["client_tls_cert"]; ok { data.SetStringField("client_tls_cert", types.StringValue(v)) }
		if v, ok := cfgMap["version"]; ok { data.SetStringField("version", types.StringValue(v)) }
		if v, ok := cfgMap["batch_size"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("batch_size", types.Int64Value(int64(n))) } }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_kafka", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("brokers", plan.Brokers); notifyData.SetStringField("topic", plan.Topic)
	notifyData.SetStringField("sasl_username", plan.SaslUsername); notifyData.SetStringField("sasl_password", plan.SaslPassword)
	notifyData.SetStringField("sasl_mechanism", plan.SaslMechanism); notifyData.SetBoolField("tls", plan.TLS)
	notifyData.SetBoolField("tls_skip_verify", plan.TLSSkipVerify); notifyData.SetInt64Field("tls_client_auth", plan.TLSClientAuth)
	notifyData.SetStringField("client_tls_cert", plan.ClientTLSCert); notifyData.SetStringField("client_tls_key", plan.ClientTLSKey)
	notifyData.SetStringField("version", plan.Version); notifyData.SetInt64Field("batch_size", plan.BatchSize)
	diags := notifyFrameworkUpdate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	plan.Name = notifyData.Name; plan.Enable = notifyData.Enable; plan.QueueDir = notifyData.QueueDir; plan.QueueLimit = notifyData.QueueLimit; plan.Comment = notifyData.Comment; plan.RestartRequired = notifyData.RestartRequired
	plan.Brokers = notifyData.GetStringField("brokers"); plan.Topic = notifyData.GetStringField("topic")
	plan.SaslUsername = notifyData.GetStringField("sasl_username"); plan.SaslPassword = notifyData.GetStringField("sasl_password")
	plan.SaslMechanism = notifyData.GetStringField("sasl_mechanism"); plan.TLS = notifyData.GetBoolField("tls")
	plan.TLSSkipVerify = notifyData.GetBoolField("tls_skip_verify"); plan.TLSClientAuth = notifyData.GetInt64Field("tls_client_auth")
	plan.ClientTLSCert = notifyData.GetStringField("client_tls_cert"); plan.ClientTLSKey = notifyData.GetStringField("client_tls_key")
	plan.Version = notifyData.GetStringField("version"); plan.BatchSize = notifyData.GetInt64Field("batch_size")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyKafkaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioNotifyKafkaResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	name := state.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting notify_kafka: %s", name))
	diags := notifyFrameworkDelete(ctx, r.client, "notify_kafka", name, &notifyFrameworkResourceData{Name: state.Name})
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyKafkaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
