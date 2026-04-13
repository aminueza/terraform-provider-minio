package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/minio/madmin-go/v3"
)

var (
	_ resource.Resource                = &minioNotifyAmqpResource{}
	_ resource.ResourceWithConfigure   = &minioNotifyAmqpResource{}
	_ resource.ResourceWithImportState = &minioNotifyAmqpResource{}
)

type minioNotifyAmqpResource struct {
	client *madmin.AdminClient
}

type minioNotifyAmqpResourceModel struct {
	Name            types.String `tfsdk:"name"`
	Enable          types.Bool   `tfsdk:"enable"`
	QueueDir        types.String `tfsdk:"queue_dir"`
	QueueLimit      types.Int64  `tfsdk:"queue_limit"`
	Comment         types.String `tfsdk:"comment"`
	RestartRequired types.Bool   `tfsdk:"restart_required"`
	URL             types.String `tfsdk:"url"`
	Exchange        types.String `tfsdk:"exchange"`
	ExchangeType    types.String `tfsdk:"exchange_type"`
	RoutingKey      types.String `tfsdk:"routing_key"`
	Mandatory       types.Bool   `tfsdk:"mandatory"`
	Durable         types.Bool   `tfsdk:"durable"`
	NoWait          types.Bool   `tfsdk:"no_wait"`
	Internal        types.Bool   `tfsdk:"internal"`
	AutoDeleted     types.Bool   `tfsdk:"auto_deleted"`
	DeliveryMode    types.Int64  `tfsdk:"delivery_mode"`
}

func resourceMinioNotifyAmqpFramework() resource.Resource {
	return &minioNotifyAmqpResource{}
}

func (r *minioNotifyAmqpResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notify_amqp"
}

func (r *minioNotifyAmqpResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *minioNotifyAmqpResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an AMQP/RabbitMQ notification target for MinIO bucket event notifications.",
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
				Description: "AMQP connection URL (e.g., 'amqp://user:pass@host:5672'). MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"exchange": schema.StringAttribute{
				Optional:    true,
				Description: "AMQP exchange name.",
			},
			"exchange_type": schema.StringAttribute{
				Optional:    true,
				Description: "AMQP exchange type (e.g., 'direct', 'fanout', 'topic', 'headers').",
			},
			"routing_key": schema.StringAttribute{
				Optional:    true,
				Description: "AMQP routing key for message delivery.",
			},
			"mandatory": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether to set the mandatory flag on published messages.",
			},
			"durable": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether the AMQP queue is durable (survives broker restart).",
			},
			"no_wait": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether to set the no-wait flag on queue declaration.",
			},
			"internal": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether the exchange is internal.",
			},
			"auto_deleted": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether the queue is auto-deleted when the last consumer unsubscribes.",
			},
			"delivery_mode": schema.Int64Attribute{
				Optional:    true,
				Description: "AMQP delivery mode: 1 for non-persistent, 2 for persistent.",
				Validators: []validator.Int64{
					int64validator.OneOf(1, 2),
				},
			},
		},
	}
}

func (r *minioNotifyAmqpResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan minioNotifyAmqpResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Creating notify_amqp: %s", name))

	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string

		notifyFrameworkBuildCfgAddParam(&parts, "url", data.GetStringField("url").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "exchange", data.GetStringField("exchange").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "exchange_type", data.GetStringField("exchange_type").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "routing_key", data.GetStringField("routing_key").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "mandatory", data.GetBoolField("mandatory").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "durable", data.GetBoolField("durable").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "no_wait", data.GetBoolField("no_wait").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "internal", data.GetBoolField("internal").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "auto_deleted", data.GetBoolField("auto_deleted").ValueBool())
		notifyFrameworkBuildCfgAddInt(&parts, "delivery_mode", data.GetInt64Field("delivery_mode").ValueInt64())

		notifyFrameworkBuildCommonCfg(&parts, data)

		return strings.Join(parts, " ")
	}

	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics

		if v, ok := cfgMap["exchange"]; ok {
			data.SetStringField("exchange", types.StringValue(v))
		}
		if v, ok := cfgMap["exchange_type"]; ok {
			data.SetStringField("exchange_type", types.StringValue(v))
		}
		if v, ok := cfgMap["routing_key"]; ok {
			data.SetStringField("routing_key", types.StringValue(v))
		}
		if v, ok := cfgMap["mandatory"]; ok {
			data.SetBoolField("mandatory", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("mandatory", types.BoolValue(false))
		}
		if v, ok := cfgMap["durable"]; ok {
			data.SetBoolField("durable", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("durable", types.BoolValue(false))
		}
		if v, ok := cfgMap["no_wait"]; ok {
			data.SetBoolField("no_wait", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("no_wait", types.BoolValue(false))
		}
		if v, ok := cfgMap["internal"]; ok {
			data.SetBoolField("internal", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("internal", types.BoolValue(false))
		}
		if v, ok := cfgMap["auto_deleted"]; ok {
			data.SetBoolField("auto_deleted", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("auto_deleted", types.BoolValue(false))
		}
		if v, ok := cfgMap["delivery_mode"]; ok {
			if n, err := parseInt(v); err == nil {
				data.SetInt64Field("delivery_mode", types.Int64Value(int64(n)))
			}
		}

		notifyFrameworkReadCommonFields(cfgMap, data)

		return diags
	}

	config := notifyFrameworkConfig{
		Subsystem:  "notify_amqp",
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
	notifyData.SetStringField("exchange", plan.Exchange)
	notifyData.SetStringField("exchange_type", plan.ExchangeType)
	notifyData.SetStringField("routing_key", plan.RoutingKey)
	notifyData.SetBoolField("mandatory", plan.Mandatory)
	notifyData.SetBoolField("durable", plan.Durable)
	notifyData.SetBoolField("no_wait", plan.NoWait)
	notifyData.SetBoolField("internal", plan.Internal)
	notifyData.SetBoolField("auto_deleted", plan.AutoDeleted)
	notifyData.SetInt64Field("delivery_mode", plan.DeliveryMode)

	diags := notifyFrameworkCreate(ctx, r.client, config, notifyData)
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
	plan.Exchange = notifyData.GetStringField("exchange")
	plan.ExchangeType = notifyData.GetStringField("exchange_type")
	plan.RoutingKey = notifyData.GetStringField("routing_key")
	plan.Mandatory = notifyData.GetBoolField("mandatory")
	plan.Durable = notifyData.GetBoolField("durable")
	plan.NoWait = notifyData.GetBoolField("no_wait")
	plan.Internal = notifyData.GetBoolField("internal")
	plan.AutoDeleted = notifyData.GetBoolField("auto_deleted")
	plan.DeliveryMode = notifyData.GetInt64Field("delivery_mode")

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
	plan.Exchange = notifyData.GetStringField("exchange")
	plan.ExchangeType = notifyData.GetStringField("exchange_type")
	plan.RoutingKey = notifyData.GetStringField("routing_key")
	plan.Mandatory = notifyData.GetBoolField("mandatory")
	plan.Durable = notifyData.GetBoolField("durable")
	plan.NoWait = notifyData.GetBoolField("no_wait")
	plan.Internal = notifyData.GetBoolField("internal")
	plan.AutoDeleted = notifyData.GetBoolField("auto_deleted")
	plan.DeliveryMode = notifyData.GetInt64Field("delivery_mode")

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyAmqpResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state minioNotifyAmqpResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "url", data.GetStringField("url").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "exchange", data.GetStringField("exchange").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "exchange_type", data.GetStringField("exchange_type").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "routing_key", data.GetStringField("routing_key").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "mandatory", data.GetBoolField("mandatory").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "durable", data.GetBoolField("durable").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "no_wait", data.GetBoolField("no_wait").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "internal", data.GetBoolField("internal").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "auto_deleted", data.GetBoolField("auto_deleted").ValueBool())
		notifyFrameworkBuildCfgAddInt(&parts, "delivery_mode", data.GetInt64Field("delivery_mode").ValueInt64())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}

	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["exchange"]; ok {
			data.SetStringField("exchange", types.StringValue(v))
		}
		if v, ok := cfgMap["exchange_type"]; ok {
			data.SetStringField("exchange_type", types.StringValue(v))
		}
		if v, ok := cfgMap["routing_key"]; ok {
			data.SetStringField("routing_key", types.StringValue(v))
		}
		if v, ok := cfgMap["mandatory"]; ok {
			data.SetBoolField("mandatory", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("mandatory", types.BoolValue(false))
		}
		if v, ok := cfgMap["durable"]; ok {
			data.SetBoolField("durable", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("durable", types.BoolValue(false))
		}
		if v, ok := cfgMap["no_wait"]; ok {
			data.SetBoolField("no_wait", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("no_wait", types.BoolValue(false))
		}
		if v, ok := cfgMap["internal"]; ok {
			data.SetBoolField("internal", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("internal", types.BoolValue(false))
		}
		if v, ok := cfgMap["auto_deleted"]; ok {
			data.SetBoolField("auto_deleted", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("auto_deleted", types.BoolValue(false))
		}
		if v, ok := cfgMap["delivery_mode"]; ok {
			if n, err := parseInt(v); err == nil {
				data.SetInt64Field("delivery_mode", types.Int64Value(int64(n)))
			}
		}
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}

	config := notifyFrameworkConfig{
		Subsystem:  "notify_amqp",
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
	notifyData.SetStringField("exchange", state.Exchange)
	notifyData.SetStringField("exchange_type", state.ExchangeType)
	notifyData.SetStringField("routing_key", state.RoutingKey)
	notifyData.SetBoolField("mandatory", state.Mandatory)
	notifyData.SetBoolField("durable", state.Durable)
	notifyData.SetBoolField("no_wait", state.NoWait)
	notifyData.SetBoolField("internal", state.Internal)
	notifyData.SetBoolField("auto_deleted", state.AutoDeleted)
	notifyData.SetInt64Field("delivery_mode", state.DeliveryMode)

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
	state.Exchange = notifyData.GetStringField("exchange")
	state.ExchangeType = notifyData.GetStringField("exchange_type")
	state.RoutingKey = notifyData.GetStringField("routing_key")
	state.Mandatory = notifyData.GetBoolField("mandatory")
	state.Durable = notifyData.GetBoolField("durable")
	state.NoWait = notifyData.GetBoolField("no_wait")
	state.Internal = notifyData.GetBoolField("internal")
	state.AutoDeleted = notifyData.GetBoolField("auto_deleted")
	state.DeliveryMode = notifyData.GetInt64Field("delivery_mode")

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyAmqpResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan minioNotifyAmqpResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	buildCfg := func(data *notifyFrameworkResourceData) string {
		var parts []string
		notifyFrameworkBuildCfgAddParam(&parts, "url", data.GetStringField("url").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "exchange", data.GetStringField("exchange").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "exchange_type", data.GetStringField("exchange_type").ValueString())
		notifyFrameworkBuildCfgAddParam(&parts, "routing_key", data.GetStringField("routing_key").ValueString())
		notifyFrameworkBuildCfgAddBool(&parts, "mandatory", data.GetBoolField("mandatory").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "durable", data.GetBoolField("durable").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "no_wait", data.GetBoolField("no_wait").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "internal", data.GetBoolField("internal").ValueBool())
		notifyFrameworkBuildCfgAddBool(&parts, "auto_deleted", data.GetBoolField("auto_deleted").ValueBool())
		notifyFrameworkBuildCfgAddInt(&parts, "delivery_mode", data.GetInt64Field("delivery_mode").ValueInt64())
		notifyFrameworkBuildCommonCfg(&parts, data)
		return strings.Join(parts, " ")
	}

	readFields := func(cfgMap map[string]string, data *notifyFrameworkResourceData) diag.Diagnostics {
		var diags diag.Diagnostics
		if v, ok := cfgMap["exchange"]; ok {
			data.SetStringField("exchange", types.StringValue(v))
		}
		if v, ok := cfgMap["exchange_type"]; ok {
			data.SetStringField("exchange_type", types.StringValue(v))
		}
		if v, ok := cfgMap["routing_key"]; ok {
			data.SetStringField("routing_key", types.StringValue(v))
		}
		if v, ok := cfgMap["mandatory"]; ok {
			data.SetBoolField("mandatory", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("mandatory", types.BoolValue(false))
		}
		if v, ok := cfgMap["durable"]; ok {
			data.SetBoolField("durable", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("durable", types.BoolValue(false))
		}
		if v, ok := cfgMap["no_wait"]; ok {
			data.SetBoolField("no_wait", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("no_wait", types.BoolValue(false))
		}
		if v, ok := cfgMap["internal"]; ok {
			data.SetBoolField("internal", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("internal", types.BoolValue(false))
		}
		if v, ok := cfgMap["auto_deleted"]; ok {
			data.SetBoolField("auto_deleted", types.BoolValue(v == "on"))
		} else {
			data.SetBoolField("auto_deleted", types.BoolValue(false))
		}
		if v, ok := cfgMap["delivery_mode"]; ok {
			if n, err := parseInt(v); err == nil {
				data.SetInt64Field("delivery_mode", types.Int64Value(int64(n)))
			}
		}
		notifyFrameworkReadCommonFields(cfgMap, data)
		return diags
	}

	config := notifyFrameworkConfig{
		Subsystem:  "notify_amqp",
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
	notifyData.SetStringField("exchange", plan.Exchange)
	notifyData.SetStringField("exchange_type", plan.ExchangeType)
	notifyData.SetStringField("routing_key", plan.RoutingKey)
	notifyData.SetBoolField("mandatory", plan.Mandatory)
	notifyData.SetBoolField("durable", plan.Durable)
	notifyData.SetBoolField("no_wait", plan.NoWait)
	notifyData.SetBoolField("internal", plan.Internal)
	notifyData.SetBoolField("auto_deleted", plan.AutoDeleted)
	notifyData.SetInt64Field("delivery_mode", plan.DeliveryMode)

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
	plan.Exchange = notifyData.GetStringField("exchange")
	plan.ExchangeType = notifyData.GetStringField("exchange_type")
	plan.RoutingKey = notifyData.GetStringField("routing_key")
	plan.Mandatory = notifyData.GetBoolField("mandatory")
	plan.Durable = notifyData.GetBoolField("durable")
	plan.NoWait = notifyData.GetBoolField("no_wait")
	plan.Internal = notifyData.GetBoolField("internal")
	plan.AutoDeleted = notifyData.GetBoolField("auto_deleted")
	plan.DeliveryMode = notifyData.GetInt64Field("delivery_mode")

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyAmqpResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state minioNotifyAmqpResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()
	tflog.Info(ctx, fmt.Sprintf("Deleting notify_amqp: %s", name))

	diags := notifyFrameworkDelete(ctx, r.client, "notify_amqp", name, &notifyFrameworkResourceData{
		Name: state.Name,
	})
	resp.Diagnostics.Append(diags...)
}

func (r *minioNotifyAmqpResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
