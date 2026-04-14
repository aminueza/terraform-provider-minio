package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/minio-go/v7/pkg/notification"
)

var (
	_ resource.Resource                = &bucketNotificationResource{}
	_ resource.ResourceWithConfigure   = &bucketNotificationResource{}
	_ resource.ResourceWithImportState = &bucketNotificationResource{}
)

type bucketNotificationResource struct {
	client *S3MinioClient
}

type bucketNotificationResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Bucket types.String `tfsdk:"bucket"`
	Queue  types.List   `tfsdk:"queue"`
}

type queueNotificationModel struct {
	ID           types.String `tfsdk:"id"`
	FilterPrefix types.String `tfsdk:"filter_prefix"`
	FilterSuffix types.String `tfsdk:"filter_suffix"`
	QueueARN     types.String `tfsdk:"queue_arn"`
	Events       types.Set    `tfsdk:"events"`
}

var queueNotificationObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":            types.StringType,
		"filter_prefix": types.StringType,
		"filter_suffix": types.StringType,
		"queue_arn":     types.StringType,
		"events":        types.SetType{ElemType: types.StringType},
	},
}

func (r *bucketNotificationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket_notification"
}

func (r *bucketNotificationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages event notification configuration for an S3 bucket. Sends bucket events to configured queue targets.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Bucket name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Description: "Name of the bucket.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"queue": schema.ListAttribute{
				Description: "List of queue notification configurations.",
				Optional:    true,
				ElementType: queueNotificationObjectType,
			},
		},
	}
}

func (r *bucketNotificationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *S3MinioClient, got: %T", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *bucketNotificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data bucketNotificationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setNotification(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = data.Bucket

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketNotificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data bucketNotificationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketNotificationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data bucketNotificationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setNotification(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketNotificationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data bucketNotificationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.S3Client.SetBucketNotification(ctx, data.Bucket.ValueString(), notification.Configuration{})
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "NoSuchBucket") {
			return
		}
		resp.Diagnostics.AddError(
			"Error removing bucket notifications",
			err.Error(),
		)
		return
	}
}

func (r *bucketNotificationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("bucket"), req, resp)
}

func (r *bucketNotificationResource) setNotification(ctx context.Context, data *bucketNotificationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	config := r.buildNotificationConfig(ctx, data, &diags)
	if diags.HasError() {
		return diags
	}

	err := r.client.S3Client.SetBucketNotification(ctx, data.Bucket.ValueString(), config)
	if err != nil {
		diags.AddError("Error putting bucket notification configuration", err.Error())
		return diags
	}

	return diags
}

func (r *bucketNotificationResource) read(ctx context.Context, data *bucketNotificationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	notificationConfig, err := r.client.S3Client.GetBucketNotification(ctx, data.Bucket.ValueString())
	if err != nil {
		if isS3CompatNotSupported(r.client, err) {
			data.ID = types.StringNull()
			return diags
		}
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "NoSuchBucket") {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError("Error loading bucket notification configuration", err.Error())
		return diags
	}

	queueList, d := r.flattenQueueNotificationConfiguration(ctx, notificationConfig.QueueConfigs)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	data.Queue = queueList

	if data.ID.IsNull() {
		data.ID = data.Bucket
	}

	return diags
}

func (r *bucketNotificationResource) buildNotificationConfig(ctx context.Context, data *bucketNotificationResourceModel, diags *diag.Diagnostics) notification.Configuration {
	var config notification.Configuration

	if data.Queue.IsNull() || data.Queue.IsUnknown() {
		return config
	}

	var queueList []queueNotificationModel
	diags.Append(data.Queue.ElementsAs(ctx, &queueList, false)...)
	if diags.HasError() {
		return config
	}

	for _, queueModel := range queueList {
		queueConfig := notification.Config{Filter: &notification.Filter{}}

		if !queueModel.QueueARN.IsNull() {
			if arn, err := notification.NewArnFromString(queueModel.QueueARN.ValueString()); err == nil {
				queueConfig.Arn = arn
			}
		}

		if !queueModel.ID.IsNull() {
			queueConfig.ID = queueModel.ID.ValueString()
		}

		if !queueModel.Events.IsNull() {
			var events []string
			diags.Append(queueModel.Events.ElementsAs(ctx, &events, false)...)
			if diags.HasError() {
				return config
			}
			for _, event := range events {
				queueConfig.AddEvents(notification.EventType(event))
			}
		}

		if !queueModel.FilterPrefix.IsNull() {
			queueConfig.AddFilterPrefix(queueModel.FilterPrefix.ValueString())
		}
		if !queueModel.FilterSuffix.IsNull() {
			queueConfig.AddFilterSuffix(queueModel.FilterSuffix.ValueString())
		}

		config.AddQueue(queueConfig)
	}

	return config
}

func (r *bucketNotificationResource) flattenQueueNotificationConfiguration(ctx context.Context, configs []notification.QueueConfig) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	result := make([]attr.Value, 0, len(configs))

	for _, config := range configs {
		queueModel := queueNotificationModel{}

		if config.ID != "" {
			queueModel.ID = types.StringValue(config.ID)
		}

		if config.Queue != "" {
			queueModel.QueueARN = types.StringValue(config.Queue)
		}

		if config.Filter != nil && config.Filter.S3Key.FilterRules != nil {
			for _, f := range config.Filter.S3Key.FilterRules {
				if f.Name == "prefix" {
					queueModel.FilterPrefix = types.StringValue(f.Value)
				}
				if f.Name == "suffix" {
					queueModel.FilterSuffix = types.StringValue(f.Value)
				}
			}
		}

		if len(config.Events) > 0 {
			events := make([]attr.Value, 0, len(config.Events))
			for _, event := range config.Events {
				events = append(events, types.StringValue(string(event)))
			}
			queueModel.Events = types.SetValueMust(types.StringType, events)
		}

		obj, d := types.ObjectValue(queueNotificationObjectType.AttrTypes, map[string]attr.Value{
			"id":            queueModel.ID,
			"filter_prefix": queueModel.FilterPrefix,
			"filter_suffix": queueModel.FilterSuffix,
			"queue_arn":     queueModel.QueueARN,
			"events":        queueModel.Events,
		})
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(queueNotificationObjectType), diags
		}
		result = append(result, obj)
	}

	return types.ListValue(queueNotificationObjectType, result)
}

func NewMinioARNValidator() validator.String {
	return minioARNValidator{}
}

type minioARNValidator struct{}

func (v minioARNValidator) Description(ctx context.Context) string {
	return "value must be a valid MinIO ARN"
}

func (v minioARNValidator) MarkdownDescription(ctx context.Context) string {
	return "value must be a valid MinIO ARN"
}

func (v minioARNValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	_, err := notification.NewArnFromString(req.ConfigValue.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid MinIO ARN",
			fmt.Sprintf("value: %s is not a valid ARN", req.ConfigValue.ValueString()),
		)
	}
}

func newBucketNotificationResource() resource.Resource {
	return &bucketNotificationResource{}
}
