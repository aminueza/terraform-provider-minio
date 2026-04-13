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
	_ resource.Resource                = &minioNotifyPostgresResource{}
	_ resource.ResourceWithConfigure   = &minioNotifyPostgresResource{}
	_ resource.ResourceWithImportState = &minioNotifyPostgresResource{}
)

type minioNotifyPostgresResource struct {
	client *madmin.AdminClient
}

type minioNotifyPostgresResourceModel struct {
	Name               types.String `tfsdk:"name"`
	Enable             types.Bool   `tfsdk:"enable"`
	QueueDir           types.String `tfsdk:"queue_dir"`
	QueueLimit         types.Int64  `tfsdk:"queue_limit"`
	Comment            types.String `tfsdk:"comment"`
	RestartRequired    types.Bool   `tfsdk:"restart_required"`
	ConnectionString   types.String `tfsdk:"connection_string"`
	Table              types.String `tfsdk:"table"`
	Format             types.String `tfsdk:"format"`
	MaxOpenConnections types.Int64  `tfsdk:"max_open_connections"`
}

func resourceMinioNotifyPostgresFramework() resource.Resource {
	return &minioNotifyPostgresResource{}
}

func (r *minioNotifyPostgresResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notify_postgres"
}

func (r *minioNotifyPostgresResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*madmin.AdminClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			"Expected *madmin.AdminClient",
		)
		return
	}

	r.client = client
}

func (r *minioNotifyPostgresResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a PostgreSQL notification target for MinIO bucket event notifications.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Target name identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enable": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether this notification target is enabled.",
				Default:     booldefault.StaticBool(true),
			},
			"queue_dir": schema.StringAttribute{
				Optional:    true,
				Description: "Directory path for persistent event store when the target is offline.",
			},
			"queue_limit": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum number of undelivered messages to queue.",
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Description: "Comment or description for this notification target.",
			},
			"restart_required": schema.BoolAttribute{
				Computed:    true,
				Description: "Indicates whether a MinIO server restart is required.",
			},
			"connection_string": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "PostgreSQL connection string (e.g., 'host=localhost port=5432 dbname=minio user=minio password=secret sslmode=disable'). MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"table": schema.StringAttribute{
				Required:    true,
				Description: "Name of the PostgreSQL table for event records.",
			},
			"format": schema.StringAttribute{
				Required:    true,
				Description: "Output format for event records: 'namespace' or 'access'.",
			},
			"max_open_connections": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum number of open connections to the PostgreSQL database.",
			},
		},
	}
}

func (r *minioNotifyPostgresResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioNotifyPostgresResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating notify_postgres: %s", name))

	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "connection_string", data.GetStringField("connection_string").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "table", data.GetStringField("table").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "format", data.GetStringField("format").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "max_open_connections", data.GetInt64Field("max_open_connections").ValueInt64())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}

	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["table"]; ok && v != "" {
			data.SetStringField("table", types.StringValue(v))
		}
		if v, ok := cfgMap["format"]; ok && v != "" {
			data.SetStringField("format", types.StringValue(v))
		}
		if v, ok := cfgMap["max_open_connections"]; ok {
			if n, err := parseInt(v); err == nil {
				data.SetInt64Field("max_open_connections", types.Int64Value(int64(n)))
			}
		}
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}

	config := notifyFrameworkConfig{
		Subsystem:  "notify_postgres",
		BuildCfg:   buildCfg,
		ReadFields: readFields,
	}

	notifyData := &notifyFrameworkResourceData{
		Name:        plan.Name,
		Enable:      plan.Enable,
		QueueDir:    plan.QueueDir,
		QueueLimit:  plan.QueueLimit,
		Comment:     plan.Comment,
		ExtraFields: make(map[string]interface{}),
	}
	notifyData.SetStringField("connection_string", plan.ConnectionString)
	notifyData.SetStringField("table", plan.Table)
	notifyData.SetStringField("format", plan.Format)
	notifyData.SetInt64Field("max_open_connections", plan.MaxOpenConnections)

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
	plan.ConnectionString = notifyData.GetStringField("connection_string")
	plan.Table = notifyData.GetStringField("table")
	plan.Format = notifyData.GetStringField("format")
	plan.MaxOpenConnections = notifyData.GetInt64Field("max_open_connections")

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyPostgresResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioNotifyPostgresResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "connection_string", data.GetStringField("connection_string").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "table", data.GetStringField("table").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "format", data.GetStringField("format").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "max_open_connections", data.GetInt64Field("max_open_connections").ValueInt64())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}

	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["table"]; ok && v != "" {
			data.SetStringField("table", types.StringValue(v))
		}
		if v, ok := cfgMap["format"]; ok && v != "" {
			data.SetStringField("format", types.StringValue(v))
		}
		if v, ok := cfgMap["max_open_connections"]; ok {
			if n, err := parseInt(v); err == nil {
				data.SetInt64Field("max_open_connections", types.Int64Value(int64(n)))
			}
		}
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}

	config := notifyFrameworkConfig{
		Subsystem:  "notify_postgres",
		BuildCfg:   buildCfg,
		ReadFields: readFields,
	}

	notifyData := &notifyFrameworkResourceData{
		Name:        state.Name,
		Enable:      state.Enable,
		QueueDir:    state.QueueDir,
		QueueLimit:  state.QueueLimit,
		Comment:     state.Comment,
		ExtraFields: make(map[string]interface{}),
	}
	notifyData.SetStringField("connection_string", state.ConnectionString)
	notifyData.SetStringField("table", state.Table)
	notifyData.SetStringField("format", state.Format)
	notifyData.SetInt64Field("max_open_connections", state.MaxOpenConnections)

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
	state.ConnectionString = notifyData.GetStringField("connection_string")
	state.Table = notifyData.GetStringField("table")
	state.Format = notifyData.GetStringField("format")
	state.MaxOpenConnections = notifyData.GetInt64Field("max_open_connections")

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyPostgresResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioNotifyPostgresResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "connection_string", data.GetStringField("connection_string").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "table", data.GetStringField("table").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "format", data.GetStringField("format").ValueString())
		notifyFrameworkBuildCfgAddInt(&parts, "max_open_connections", data.GetInt64Field("max_open_connections").ValueInt64())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}

	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["table"]; ok && v != "" {
			data.SetStringField("table", types.StringValue(v))
		}
		if v, ok := cfgMap["format"]; ok && v != "" {
			data.SetStringField("format", types.StringValue(v))
		}
		if v, ok := cfgMap["max_open_connections"]; ok {
			if n, err := parseInt(v); err == nil {
				data.SetInt64Field("max_open_connections", types.Int64Value(int64(n)))
			}
		}
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}

	config := notifyFrameworkConfig{
		Subsystem:  "notify_postgres",
		BuildCfg:   buildCfg,
		ReadFields: readFields,
	}

	notifyData := &notifyFrameworkResourceData{
		Name:        plan.Name,
		Enable:      plan.Enable,
		QueueDir:    plan.QueueDir,
		QueueLimit:  plan.QueueLimit,
		Comment:     plan.Comment,
		ExtraFields: make(map[string]interface{}),
	}
	notifyData.SetStringField("connection_string", plan.ConnectionString)
	notifyData.SetStringField("table", plan.Table)
	notifyData.SetStringField("format", plan.Format)
	notifyData.SetInt64Field("max_open_connections", plan.MaxOpenConnections)

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
	plan.ConnectionString = notifyData.GetStringField("connection_string")
	plan.Table = notifyData.GetStringField("table")
	plan.Format = notifyData.GetStringField("format")
	plan.MaxOpenConnections = notifyData.GetInt64Field("max_open_connections")

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyPostgresResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioNotifyPostgresResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting notify_postgres: %s", name))

	diags := notifyFrameworkDelete(ctx, r.client, "notify_postgres", name, &notifyFrameworkResourceData{
		Name: state.Name,
	})
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyPostgresResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
