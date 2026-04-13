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
	_ resource.Resource                = &minioNotifyElasticsearchResource{}
	_ resource.ResourceWithConfigure   = &minioNotifyElasticsearchResource{}
	_ resource.ResourceWithImportState = &minioNotifyElasticsearchResource{}
)

type minioNotifyElasticsearchResource struct {
	client *madmin.AdminClient
}

type minioNotifyElasticsearchResourceModel struct {
	Name            types.String `tfsdk:"name"`
	Enable          types.Bool   `tfsdk:"enable"`
	QueueDir        types.String `tfsdk:"queue_dir"`
	QueueLimit      types.Int64  `tfsdk:"queue_limit"`
	Comment         types.String `tfsdk:"comment"`
	RestartRequired types.Bool   `tfsdk:"restart_required"`
	URL             types.String `tfsdk:"url"`
	Index           types.String `tfsdk:"index"`
	Format          types.String `tfsdk:"format"`
	Username        types.String `tfsdk:"username"`
	Password        types.String `tfsdk:"password"`
}

func resourceMinioNotifyElasticsearchFramework() resource.Resource {
	return &minioNotifyElasticsearchResource{}
}

func (r *minioNotifyElasticsearchResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notify_elasticsearch"
}

func (r *minioNotifyElasticsearchResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *minioNotifyElasticsearchResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Elasticsearch notification target for MinIO bucket event notifications.",
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
			"url": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Elasticsearch server URL (e.g., 'http://localhost:9200'). MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"index": schema.StringAttribute{
				Required:    true,
				Description: "Name of the Elasticsearch index for event records.",
			},
			"format": schema.StringAttribute{
				Required:    true,
				Description: "Output format for event records: 'namespace' or 'access'.",
			},
			"username": schema.StringAttribute{
				Optional:    true,
				Description: "Username for Elasticsearch authentication.",
			},
			"password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Password for Elasticsearch authentication. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
		},
	}
}

func (r *minioNotifyElasticsearchResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioNotifyElasticsearchResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating notify_elasticsearch: %s", name))

	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "url", data.GetStringField("url").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "index", data.GetStringField("index").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "format", data.GetStringField("format").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "username", data.GetStringField("username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}

	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["index"]; ok && v != "" {
			data.SetStringField("index", types.StringValue(v))
		}
		if v, ok := cfgMap["format"]; ok && v != "" {
			data.SetStringField("format", types.StringValue(v))
		}
		if v, ok := cfgMap["username"]; ok && v != "" {
			data.SetStringField("username", types.StringValue(v))
		}
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}

	config := notifyFrameworkConfig{
		Subsystem:  "notify_elasticsearch",
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
	notifyData.SetStringField("url", plan.URL)
	notifyData.SetStringField("index", plan.Index)
	notifyData.SetStringField("format", plan.Format)
	notifyData.SetStringField("username", plan.Username)
	notifyData.SetStringField("password", plan.Password)

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
	plan.URL = notifyData.GetStringField("url")
	plan.Index = notifyData.GetStringField("index")
	plan.Format = notifyData.GetStringField("format")
	plan.Username = notifyData.GetStringField("username")
	plan.Password = notifyData.GetStringField("password")

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyElasticsearchResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioNotifyElasticsearchResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "url", data.GetStringField("url").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "index", data.GetStringField("index").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "format", data.GetStringField("format").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "username", data.GetStringField("username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}

	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["index"]; ok && v != "" {
			data.SetStringField("index", types.StringValue(v))
		}
		if v, ok := cfgMap["format"]; ok && v != "" {
			data.SetStringField("format", types.StringValue(v))
		}
		if v, ok := cfgMap["username"]; ok && v != "" {
			data.SetStringField("username", types.StringValue(v))
		}
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}

	config := notifyFrameworkConfig{
		Subsystem:  "notify_elasticsearch",
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
	notifyData.SetStringField("url", state.URL)
	notifyData.SetStringField("index", state.Index)
	notifyData.SetStringField("format", state.Format)
	notifyData.SetStringField("username", state.Username)
	notifyData.SetStringField("password", state.Password)

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
	state.URL = notifyData.GetStringField("url")
	state.Index = notifyData.GetStringField("index")
	state.Format = notifyData.GetStringField("format")
	state.Username = notifyData.GetStringField("username")
	state.Password = notifyData.GetStringField("password")

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyElasticsearchResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioNotifyElasticsearchResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "url", data.GetStringField("url").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "index", data.GetStringField("index").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "format", data.GetStringField("format").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "username", data.GetStringField("username").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "password", data.GetStringField("password").ValueString())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}

	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["index"]; ok && v != "" {
			data.SetStringField("index", types.StringValue(v))
		}
		if v, ok := cfgMap["format"]; ok && v != "" {
			data.SetStringField("format", types.StringValue(v))
		}
		if v, ok := cfgMap["username"]; ok && v != "" {
			data.SetStringField("username", types.StringValue(v))
		}
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}

	config := notifyFrameworkConfig{
		Subsystem:  "notify_elasticsearch",
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
	notifyData.SetStringField("url", plan.URL)
	notifyData.SetStringField("index", plan.Index)
	notifyData.SetStringField("format", plan.Format)
	notifyData.SetStringField("username", plan.Username)
	notifyData.SetStringField("password", plan.Password)

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
	plan.URL = notifyData.GetStringField("url")
	plan.Index = notifyData.GetStringField("index")
	plan.Format = notifyData.GetStringField("format")
	plan.Username = notifyData.GetStringField("username")
	plan.Password = notifyData.GetStringField("password")

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyElasticsearchResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioNotifyElasticsearchResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting notify_elasticsearch: %s", name))

	diags := notifyFrameworkDelete(ctx, r.client, "notify_elasticsearch", name, &notifyFrameworkResourceData{
		Name: state.Name,
	})
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyElasticsearchResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
