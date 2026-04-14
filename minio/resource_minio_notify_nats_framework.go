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
	"github.com/minio/madmin-go/v3"
)

var (
	_ resource.Resource                = &minioNotifyNatsResource{}
	_ resource.ResourceWithConfigure   = &minioNotifyNatsResource{}
	_ resource.ResourceWithImportState = &minioNotifyNatsResource{}
)

type minioNotifyNatsResource struct {
	client *madmin.AdminClient
}

type minioNotifyNatsResourceModel struct {
	Name                         types.String `tfsdk:"name"`
	Enable                       types.Bool   `tfsdk:"enable"`
	QueueDir                     types.String `tfsdk:"queue_dir"`
	QueueLimit                   types.Int64  `tfsdk:"queue_limit"`
	Comment                      types.String `tfsdk:"comment"`
	RestartRequired              types.Bool   `tfsdk:"restart_required"`
	Address                      types.String `tfsdk:"address"`
	Subject                      types.String `tfsdk:"subject"`
	Username                     types.String `tfsdk:"username"`
	Password                     types.String `tfsdk:"password"`
	Token                        types.String `tfsdk:"token"`
	UserCredentials              types.String `tfsdk:"user_credentials"`
	TLS                          types.Bool   `tfsdk:"tls"`
	TLSSkipVerify                types.Bool   `tfsdk:"tls_skip_verify"`
	PingInterval                 types.String `tfsdk:"ping_interval"`
	JetStream                    types.Bool   `tfsdk:"jetstream"`
	Streaming                    types.Bool   `tfsdk:"streaming"`
	StreamingAsync               types.Bool   `tfsdk:"streaming_async"`
	StreamingMaxPubAcksInFlight  types.Int64  `tfsdk:"streaming_max_pub_acks_in_flight"`
	StreamingClusterID           types.String `tfsdk:"streaming_cluster_id"`
	CertAuthority                types.String `tfsdk:"cert_authority"`
	ClientCert                   types.String `tfsdk:"client_cert"`
	ClientKey                    types.String `tfsdk:"client_key"`
}

func resourceMinioNotifyNatsFramework() resource.Resource {
	return &minioNotifyNatsResource{}
}

func (r *minioNotifyNatsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notify_nats"
}

func (r *minioNotifyNatsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil { return }
	client, ok := req.ProviderData.(*madmin.AdminClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Expected *madmin.AdminClient")
		return
	}
	r.client = client
}

func (r *minioNotifyNatsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a NATS notification target for MinIO bucket event notifications.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{Required: true, Description: "Target name identifier.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable": schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether this notification target is enabled.", Default: booldefault.StaticBool(true)},
			"queue_dir": schema.StringAttribute{Optional: true, Description: "Directory path for persistent event store when the target is offline."},
			"queue_limit": schema.Int64Attribute{Optional: true, Computed: true, Description: "Maximum number of undelivered messages to queue."},
			"comment": schema.StringAttribute{Optional: true, Description: "Comment or description for this notification target."},
			"restart_required": schema.BoolAttribute{Computed: true, Description: "Indicates whether a MinIO server restart is required."},
			"address": schema.StringAttribute{Required: true, Description: "NATS server address (e.g., 'nats://localhost:4222')."},
			"subject": schema.StringAttribute{Required: true, Description: "NATS subject to publish notifications to."},
			"username": schema.StringAttribute{Optional: true, Description: "Username for NATS authentication."},
			"password": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Password for NATS authentication. MinIO does not return this value on read."},
			"token": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Token for NATS authentication. MinIO does not return this value on read."},
			"user_credentials": schema.StringAttribute{Optional: true, Description: "Path to NATS user credentials file."},
			"tls": schema.BoolAttribute{Optional: true, Description: "Whether to enable TLS for the NATS connection."},
			"tls_skip_verify": schema.BoolAttribute{Optional: true, Description: "Whether to skip TLS certificate verification."},
			"ping_interval": schema.StringAttribute{Optional: true, Description: "Duration interval between NATS ping requests (e.g., '0s')."},
			"jetstream": schema.BoolAttribute{Optional: true, Description: "Whether to enable JetStream support for NATS."},
			"streaming": schema.BoolAttribute{Optional: true, Description: "Whether to enable NATS Streaming (STAN) mode."},
			"streaming_async": schema.BoolAttribute{Optional: true, Description: "Whether to enable asynchronous publishing for NATS Streaming."},
			"streaming_max_pub_acks_in_flight": schema.Int64Attribute{Optional: true, Description: "Maximum number of unacknowledged messages in flight for NATS Streaming."},
			"streaming_cluster_id": schema.StringAttribute{Optional: true, Description: "Cluster ID for NATS Streaming."},
			"cert_authority": schema.StringAttribute{Optional: true, Description: "Path to the certificate authority file for TLS verification."},
			"client_cert": schema.StringAttribute{Optional: true, Description: "Path to the client certificate for mTLS authentication."},
			"client_key": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Path to the client private key for mTLS authentication. MinIO does not return this value on read."},
		},
	}
}

func (r *minioNotifyNatsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioNotifyNatsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }
	name := plan.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating notify_nats: %s", name))
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "address", data.GetStringField("address").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "subject", data.GetStringField("subject").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "username", data.GetStringField("username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "token", data.GetStringField("token").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "user_credentials", data.GetStringField("user_credentials").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "tls", data.GetBoolField("tls").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "tls_skip_verify", data.GetBoolField("tls_skip_verify").ValueBool())
		notifyFrameworkBuildCfgAddParam(&parts, "ping_interval", data.GetStringField("ping_interval").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "jetstream", data.GetBoolField("jetstream").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "streaming", data.GetBoolField("streaming").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "streaming_async", data.GetBoolField("streaming_async").ValueBool())
		notifyFrameworkBuildCfgAddInt(&parts, "streaming_max_pub_acks_in_flight", data.GetInt64Field("streaming_max_pub_acks_in_flight").ValueInt64())
		notifyFrameworkBuildCfgAddParam(&parts, "streaming_cluster_id", data.GetStringField("streaming_cluster_id").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "cert_authority", data.GetStringField("cert_authority").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_cert", data.GetStringField("client_cert").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_key", data.GetStringField("client_key").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["address"]; ok { data.SetStringField("address", types.StringValue(v)) }
		if v, ok := cfgMap["subject"]; ok { data.SetStringField("subject", types.StringValue(v)) }
		if v, ok := cfgMap["username"]; ok && v != "" { data.SetStringField("username", types.StringValue(v)) }
		if v, ok := cfgMap["user_credentials"]; ok && v != "" { data.SetStringField("user_credentials", types.StringValue(v)) }
		if v, ok := cfgMap["tls"]; ok { data.SetBoolField("tls", types.BoolValue(v == "on")) } else { data.SetBoolField("tls", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_skip_verify"]; ok { data.SetBoolField("tls_skip_verify", types.BoolValue(v == "on")) } else { data.SetBoolField("tls_skip_verify", types.BoolValue(false)) }
		if v, ok := cfgMap["ping_interval"]; ok && v != "" { data.SetStringField("ping_interval", types.StringValue(v)) }
		if v, ok := cfgMap["jetstream"]; ok { data.SetBoolField("jetstream", types.BoolValue(v == "on")) } else { data.SetBoolField("jetstream", types.BoolValue(false)) }
		if v, ok := cfgMap["streaming"]; ok { data.SetBoolField("streaming", types.BoolValue(v == "on")) } else { data.SetBoolField("streaming", types.BoolValue(false)) }
		if v, ok := cfgMap["streaming_async"]; ok { data.SetBoolField("streaming_async", types.BoolValue(v == "on")) } else { data.SetBoolField("streaming_async", types.BoolValue(false)) }
		if v, ok := cfgMap["streaming_max_pub_acks_in_flight"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("streaming_max_pub_acks_in_flight", types.Int64Value(int64(n))) } }
		if v, ok := cfgMap["streaming_cluster_id"]; ok && v != "" { data.SetStringField("streaming_cluster_id", types.StringValue(v)) }
		if v, ok := cfgMap["cert_authority"]; ok && v != "" { data.SetStringField("cert_authority", types.StringValue(v)) }
		if v, ok := cfgMap["client_cert"]; ok && v != "" { data.SetStringField("client_cert", types.StringValue(v)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_nats", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("address", plan.Address); notifyData.SetStringField("subject", plan.Subject)
	notifyData.SetStringField("username", plan.Username); notifyData.SetStringField("password", plan.Password)
	notifyData.SetStringField("token", plan.Token); notifyData.SetStringField("user_credentials", plan.UserCredentials)
	notifyData.SetBoolField("tls", plan.TLS); notifyData.SetBoolField("tls_skip_verify", plan.TLSSkipVerify)
	notifyData.SetStringField("ping_interval", plan.PingInterval); notifyData.SetBoolField("jetstream", plan.JetStream)
	notifyData.SetBoolField("streaming", plan.Streaming); notifyData.SetBoolField("streaming_async", plan.StreamingAsync)
	notifyData.SetInt64Field("streaming_max_pub_acks_in_flight", plan.StreamingMaxPubAcksInFlight)
	notifyData.SetStringField("streaming_cluster_id", plan.StreamingClusterID)
	notifyData.SetStringField("cert_authority", plan.CertAuthority); notifyData.SetStringField("client_cert", plan.ClientCert)
	notifyData.SetStringField("client_key", plan.ClientKey)
	diags := notifyFrameworkCreate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	plan.Name = notifyData.Name; plan.Enable = notifyData.Enable; plan.QueueDir = notifyData.QueueDir; plan.QueueLimit = notifyData.QueueLimit; plan.Comment = notifyData.Comment; plan.RestartRequired = notifyData.RestartRequired
	plan.Address = notifyData.GetStringField("address"); plan.Subject = notifyData.GetStringField("subject")
	plan.Username = notifyData.GetStringField("username"); plan.Password = notifyData.GetStringField("password")
	plan.Token = notifyData.GetStringField("token"); plan.UserCredentials = notifyData.GetStringField("user_credentials")
	plan.TLS = notifyData.GetBoolField("tls"); plan.TLSSkipVerify = notifyData.GetBoolField("tls_skip_verify")
	plan.PingInterval = notifyData.GetStringField("ping_interval"); plan.JetStream = notifyData.GetBoolField("jetstream")
	plan.Streaming = notifyData.GetBoolField("streaming"); plan.StreamingAsync = notifyData.GetBoolField("streaming_async")
	plan.StreamingMaxPubAcksInFlight = notifyData.GetInt64Field("streaming_max_pub_acks_in_flight")
	plan.StreamingClusterID = notifyData.GetStringField("streaming_cluster_id")
	plan.CertAuthority = notifyData.GetStringField("cert_authority"); plan.ClientCert = notifyData.GetStringField("client_cert")
	plan.ClientKey = notifyData.GetStringField("client_key")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyNatsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioNotifyNatsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "address", data.GetStringField("address").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "subject", data.GetStringField("subject").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "username", data.GetStringField("username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "token", data.GetStringField("token").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "user_credentials", data.GetStringField("user_credentials").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "tls", data.GetBoolField("tls").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "tls_skip_verify", data.GetBoolField("tls_skip_verify").ValueBool())
		notifyFrameworkBuildCfgAddParam(&parts, "ping_interval", data.GetStringField("ping_interval").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "jetstream", data.GetBoolField("jetstream").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "streaming", data.GetBoolField("streaming").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "streaming_async", data.GetBoolField("streaming_async").ValueBool())
		notifyFrameworkBuildCfgAddInt(&parts, "streaming_max_pub_acks_in_flight", data.GetInt64Field("streaming_max_pub_acks_in_flight").ValueInt64())
		notifyFrameworkBuildCfgAddParam(&parts, "streaming_cluster_id", data.GetStringField("streaming_cluster_id").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "cert_authority", data.GetStringField("cert_authority").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_cert", data.GetStringField("client_cert").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_key", data.GetStringField("client_key").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["address"]; ok { data.SetStringField("address", types.StringValue(v)) }
		if v, ok := cfgMap["subject"]; ok { data.SetStringField("subject", types.StringValue(v)) }
		if v, ok := cfgMap["username"]; ok && v != "" { data.SetStringField("username", types.StringValue(v)) }
		if v, ok := cfgMap["user_credentials"]; ok && v != "" { data.SetStringField("user_credentials", types.StringValue(v)) }
		if v, ok := cfgMap["tls"]; ok { data.SetBoolField("tls", types.BoolValue(v == "on")) } else { data.SetBoolField("tls", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_skip_verify"]; ok { data.SetBoolField("tls_skip_verify", types.BoolValue(v == "on")) } else { data.SetBoolField("tls_skip_verify", types.BoolValue(false)) }
		if v, ok := cfgMap["ping_interval"]; ok && v != "" { data.SetStringField("ping_interval", types.StringValue(v)) }
		if v, ok := cfgMap["jetstream"]; ok { data.SetBoolField("jetstream", types.BoolValue(v == "on")) } else { data.SetBoolField("jetstream", types.BoolValue(false)) }
		if v, ok := cfgMap["streaming"]; ok { data.SetBoolField("streaming", types.BoolValue(v == "on")) } else { data.SetBoolField("streaming", types.BoolValue(false)) }
		if v, ok := cfgMap["streaming_async"]; ok { data.SetBoolField("streaming_async", types.BoolValue(v == "on")) } else { data.SetBoolField("streaming_async", types.BoolValue(false)) }
		if v, ok := cfgMap["streaming_max_pub_acks_in_flight"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("streaming_max_pub_acks_in_flight", types.Int64Value(int64(n))) } }
		if v, ok := cfgMap["streaming_cluster_id"]; ok && v != "" { data.SetStringField("streaming_cluster_id", types.StringValue(v)) }
		if v, ok := cfgMap["cert_authority"]; ok && v != "" { data.SetStringField("cert_authority", types.StringValue(v)) }
		if v, ok := cfgMap["client_cert"]; ok && v != "" { data.SetStringField("client_cert", types.StringValue(v)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_nats", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: state.Name, Enable: state.Enable, QueueDir: state.QueueDir, QueueLimit: state.QueueLimit, Comment: state.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("address", state.Address); notifyData.SetStringField("subject", state.Subject)
	notifyData.SetStringField("username", state.Username); notifyData.SetStringField("password", state.Password)
	notifyData.SetStringField("token", state.Token); notifyData.SetStringField("user_credentials", state.UserCredentials)
	notifyData.SetBoolField("tls", state.TLS); notifyData.SetBoolField("tls_skip_verify", state.TLSSkipVerify)
	notifyData.SetStringField("ping_interval", state.PingInterval); notifyData.SetBoolField("jetstream", state.JetStream)
	notifyData.SetBoolField("streaming", state.Streaming); notifyData.SetBoolField("streaming_async", state.StreamingAsync)
	notifyData.SetInt64Field("streaming_max_pub_acks_in_flight", state.StreamingMaxPubAcksInFlight)
	notifyData.SetStringField("streaming_cluster_id", state.StreamingClusterID)
	notifyData.SetStringField("cert_authority", state.CertAuthority); notifyData.SetStringField("client_cert", state.ClientCert)
	notifyData.SetStringField("client_key", state.ClientKey)
	diags := notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	if notifyData.Name.IsNull() { resp.State.RemoveResource(ctx); return }
	state.Name = notifyData.Name; state.Enable = notifyData.Enable; state.QueueDir = notifyData.QueueDir; state.QueueLimit = notifyData.QueueLimit; state.Comment = notifyData.Comment; state.RestartRequired = notifyData.RestartRequired
	state.Address = notifyData.GetStringField("address"); state.Subject = notifyData.GetStringField("subject")
	state.Username = notifyData.GetStringField("username"); state.Password = notifyData.GetStringField("password")
	state.Token = notifyData.GetStringField("token"); state.UserCredentials = notifyData.GetStringField("user_credentials")
	state.TLS = notifyData.GetBoolField("tls"); state.TLSSkipVerify = notifyData.GetBoolField("tls_skip_verify")
	state.PingInterval = notifyData.GetStringField("ping_interval"); state.JetStream = notifyData.GetBoolField("jetstream")
	state.Streaming = notifyData.GetBoolField("streaming"); state.StreamingAsync = notifyData.GetBoolField("streaming_async")
	state.StreamingMaxPubAcksInFlight = notifyData.GetInt64Field("streaming_max_pub_acks_in_flight")
	state.StreamingClusterID = notifyData.GetStringField("streaming_cluster_id")
	state.CertAuthority = notifyData.GetStringField("cert_authority"); state.ClientCert = notifyData.GetStringField("client_cert")
	state.ClientKey = notifyData.GetStringField("client_key")
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyNatsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioNotifyNatsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }
	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "address", data.GetStringField("address").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "subject", data.GetStringField("subject").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "username", data.GetStringField("username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "token", data.GetStringField("token").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "user_credentials", data.GetStringField("user_credentials").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "tls", data.GetBoolField("tls").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "tls_skip_verify", data.GetBoolField("tls_skip_verify").ValueBool())
		notifyFrameworkBuildCfgAddParam(&parts, "ping_interval", data.GetStringField("ping_interval").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "jetstream", data.GetBoolField("jetstream").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "streaming", data.GetBoolField("streaming").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "streaming_async", data.GetBoolField("streaming_async").ValueBool())
		notifyFrameworkBuildCfgAddInt(&parts, "streaming_max_pub_acks_in_flight", data.GetInt64Field("streaming_max_pub_acks_in_flight").ValueInt64())
		notifyFrameworkBuildCfgAddParam(&parts, "streaming_cluster_id", data.GetStringField("streaming_cluster_id").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "cert_authority", data.GetStringField("cert_authority").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_cert", data.GetStringField("client_cert").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "client_key", data.GetStringField("client_key").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}
	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["address"]; ok { data.SetStringField("address", types.StringValue(v)) }
		if v, ok := cfgMap["subject"]; ok { data.SetStringField("subject", types.StringValue(v)) }
		if v, ok := cfgMap["username"]; ok && v != "" { data.SetStringField("username", types.StringValue(v)) }
		if v, ok := cfgMap["user_credentials"]; ok && v != "" { data.SetStringField("user_credentials", types.StringValue(v)) }
		if v, ok := cfgMap["tls"]; ok { data.SetBoolField("tls", types.BoolValue(v == "on")) } else { data.SetBoolField("tls", types.BoolValue(false)) }
		if v, ok := cfgMap["tls_skip_verify"]; ok { data.SetBoolField("tls_skip_verify", types.BoolValue(v == "on")) } else { data.SetBoolField("tls_skip_verify", types.BoolValue(false)) }
		if v, ok := cfgMap["ping_interval"]; ok && v != "" { data.SetStringField("ping_interval", types.StringValue(v)) }
		if v, ok := cfgMap["jetstream"]; ok { data.SetBoolField("jetstream", types.BoolValue(v == "on")) } else { data.SetBoolField("jetstream", types.BoolValue(false)) }
		if v, ok := cfgMap["streaming"]; ok { data.SetBoolField("streaming", types.BoolValue(v == "on")) } else { data.SetBoolField("streaming", types.BoolValue(false)) }
		if v, ok := cfgMap["streaming_async"]; ok { data.SetBoolField("streaming_async", types.BoolValue(v == "on")) } else { data.SetBoolField("streaming_async", types.BoolValue(false)) }
		if v, ok := cfgMap["streaming_max_pub_acks_in_flight"]; ok { if n, err := parseInt(v); err == nil { data.SetInt64Field("streaming_max_pub_acks_in_flight", types.Int64Value(int64(n))) } }
		if v, ok := cfgMap["streaming_cluster_id"]; ok && v != "" { data.SetStringField("streaming_cluster_id", types.StringValue(v)) }
		if v, ok := cfgMap["cert_authority"]; ok && v != "" { data.SetStringField("cert_authority", types.StringValue(v)) }
		if v, ok := cfgMap["client_cert"]; ok && v != "" { data.SetStringField("client_cert", types.StringValue(v)) }
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}
	config := notifyFrameworkConfig{Subsystem: "notify_nats", BuildCfg: buildCfg, ReadFields: readFields}
	notifyData := &notifyFrameworkResourceData{Name: plan.Name, Enable: plan.Enable, QueueDir: plan.QueueDir, QueueLimit: plan.QueueLimit, Comment: plan.Comment, ExtraFields: make(map[string]interface{})}
	notifyData.SetStringField("address", plan.Address); notifyData.SetStringField("subject", plan.Subject)
	notifyData.SetStringField("username", plan.Username); notifyData.SetStringField("password", plan.Password)
	notifyData.SetStringField("token", plan.Token); notifyData.SetStringField("user_credentials", plan.UserCredentials)
	notifyData.SetBoolField("tls", plan.TLS); notifyData.SetBoolField("tls_skip_verify", plan.TLSSkipVerify)
	notifyData.SetStringField("ping_interval", plan.PingInterval); notifyData.SetBoolField("jetstream", plan.JetStream)
	notifyData.SetBoolField("streaming", plan.Streaming); notifyData.SetBoolField("streaming_async", plan.StreamingAsync)
	notifyData.SetInt64Field("streaming_max_pub_acks_in_flight", plan.StreamingMaxPubAcksInFlight)
	notifyData.SetStringField("streaming_cluster_id", plan.StreamingClusterID)
	notifyData.SetStringField("cert_authority", plan.CertAuthority); notifyData.SetStringField("client_cert", plan.ClientCert)
	notifyData.SetStringField("client_key", plan.ClientKey)
	diags := notifyFrameworkUpdate(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	diags = notifyFrameworkRead(ctx, r.client, config, notifyData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	plan.Name = notifyData.Name; plan.Enable = notifyData.Enable; plan.QueueDir = notifyData.QueueDir; plan.QueueLimit = notifyData.QueueLimit; plan.Comment = notifyData.Comment; plan.RestartRequired = notifyData.RestartRequired
	plan.Address = notifyData.GetStringField("address"); plan.Subject = notifyData.GetStringField("subject")
	plan.Username = notifyData.GetStringField("username"); plan.Password = notifyData.GetStringField("password")
	plan.Token = notifyData.GetStringField("token"); plan.UserCredentials = notifyData.GetStringField("user_credentials")
	plan.TLS = notifyData.GetBoolField("tls"); plan.TLSSkipVerify = notifyData.GetBoolField("tls_skip_verify")
	plan.PingInterval = notifyData.GetStringField("ping_interval"); plan.JetStream = notifyData.GetBoolField("jetstream")
	plan.Streaming = notifyData.GetBoolField("streaming"); plan.StreamingAsync = notifyData.GetBoolField("streaming_async")
	plan.StreamingMaxPubAcksInFlight = notifyData.GetInt64Field("streaming_max_pub_acks_in_flight")
	plan.StreamingClusterID = notifyData.GetStringField("streaming_cluster_id")
	plan.CertAuthority = notifyData.GetStringField("cert_authority"); plan.ClientCert = notifyData.GetStringField("client_cert")
	plan.ClientKey = notifyData.GetStringField("client_key")
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyNatsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioNotifyNatsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	name := state.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting notify_nats: %s", name))
	diags := notifyFrameworkDelete(ctx, r.client, "notify_nats", name, &notifyFrameworkResourceData{Name: state.Name})
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyNatsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
