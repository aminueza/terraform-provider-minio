package minio

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

var (
	_ resource.Resource                = &ilmPolicyResource{}
	_ resource.ResourceWithConfigure   = &ilmPolicyResource{}
	_ resource.ResourceWithImportState = &ilmPolicyResource{}
)

type ilmPolicyResource struct {
	client *S3MinioClient
}

type ilmPolicyResourceModel struct {
	ID     types.String         `tfsdk:"id"`
	Bucket types.String         `tfsdk:"bucket"`
	Rules  []ilmPolicyRuleModel `tfsdk:"rule"`
}

type ilmPolicyRuleModel struct {
	ID                             types.String                   `tfsdk:"id"`
	Status                         types.String                   `tfsdk:"status"`
	Expiration                     types.String                   `tfsdk:"expiration"`
	ExpiredObjectDeleteMarker      types.Bool                     `tfsdk:"expired_object_delete_marker"`
	Transition                     []ilmTransitionModel           `tfsdk:"transition"`
	NoncurrentTransition           []ilmNoncurrentTransitionModel `tfsdk:"noncurrent_transition"`
	NoncurrentExpiration           []ilmNoncurrentExpirationModel `tfsdk:"noncurrent_expiration"`
	AbortIncompleteMultipartUpload []ilmAbortIncompleteModel      `tfsdk:"abort_incomplete_multipart_upload"`
	Filter                         types.String                   `tfsdk:"filter"`
	Tags                           map[string]types.String        `tfsdk:"tags"`
}

type ilmTransitionModel struct {
	Days         types.String `tfsdk:"days"`
	Date         types.String `tfsdk:"date"`
	StorageClass types.String `tfsdk:"storage_class"`
}

type ilmNoncurrentTransitionModel struct {
	StorageClass  types.String `tfsdk:"storage_class"`
	Days          types.String `tfsdk:"days"`
	NewerVersions types.Int64  `tfsdk:"newer_versions"`
}

type ilmNoncurrentExpirationModel struct {
	Days          types.String `tfsdk:"days"`
	NewerVersions types.Int64  `tfsdk:"newer_versions"`
}

type ilmAbortIncompleteModel struct {
	DaysAfterInitiation types.String `tfsdk:"days_after_initiation"`
}

func newILMPolicyResource() resource.Resource {
	return &ilmPolicyResource{}
}

func (r *ilmPolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ilm_policy"
}

func (r *ilmPolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "`minio_ilm_policy` handles lifecycle settings for a given `minio_s3_bucket`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Bucket name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Required:    true,
				Description: "Name of the bucket.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"rule": schema.ListNestedAttribute{
				Required:    true,
				Description: "List of lifecycle rules.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Required:    true,
							Description: "Unique identifier for the rule.",
						},
						"status": schema.StringAttribute{
							Optional:    true,
							Description: "Rule status (Enabled or Disabled).",
						},
						"expiration": schema.StringAttribute{
							Optional:    true,
							Description: "Expiration date or days (e.g., '2022-01-01' or '30d').",
						},
						"expired_object_delete_marker": schema.BoolAttribute{
							Optional:    true,
							Description: "Enable expired object delete marker.",
						},
						"transition": schema.ListNestedAttribute{
							Optional:    true,
							Description: "Transition configuration.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"days": schema.StringAttribute{
										Optional:    true,
										Description: "Days after which to transition objects.",
									},
									"date": schema.StringAttribute{
										Optional:    true,
										Description: "Date on which to transition objects.",
									},
									"storage_class": schema.StringAttribute{
										Required:    true,
										Description: "Storage class to transition to.",
									},
								},
							},
						},
						"noncurrent_transition": schema.ListNestedAttribute{
							Optional:    true,
							Description: "Noncurrent version transition configuration.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"storage_class": schema.StringAttribute{
										Required:    true,
										Description: "Storage class to transition to.",
									},
									"days": schema.StringAttribute{
										Optional:    true,
										Description: "Days after which to transition noncurrent versions.",
									},
									"newer_versions": schema.Int64Attribute{
										Optional:    true,
										Description: "Keep this many newer versions.",
									},
								},
							},
						},
						"noncurrent_expiration": schema.ListNestedAttribute{
							Optional:    true,
							Description: "Noncurrent version expiration configuration.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"days": schema.StringAttribute{
										Optional:    true,
										Description: "Days after which to expire noncurrent versions.",
									},
									"newer_versions": schema.Int64Attribute{
										Optional:    true,
										Description: "Keep this many newer versions.",
									},
								},
							},
						},
						"abort_incomplete_multipart_upload": schema.ListNestedAttribute{
							Optional:    true,
							Description: "Abort incomplete multipart upload configuration.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"days_after_initiation": schema.StringAttribute{
										Optional:    true,
										Description: "Days after initiation to abort incomplete multipart uploads.",
									},
								},
							},
						},
						"filter": schema.StringAttribute{
							Optional:    true,
							Description: "Filter prefix for the rule.",
						},
						"tags": schema.MapAttribute{
							Optional:    true,
							Description: "Tags for filtering objects.",
							ElementType: types.StringType,
						},
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplaceIf(func(ctx context.Context, sp planmodifier.ListRequest, rrifr *listplanmodifier.RequiresReplaceIfFuncResponse) {
						rrifr.RequiresReplace = false
					}, "Update rules in place", "Rules can be updated without recreating the resource."),
				},
			},
		},
	}
}

func (r *ilmPolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ilmPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ilmPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for i, rule := range data.Rules {
		hasExpiration := !rule.Expiration.IsNull() && rule.Expiration.ValueString() != "" && rule.Expiration.ValueString() != "DeleteMarker"
		hasExpiredObjectDeleteMarker := !rule.ExpiredObjectDeleteMarker.IsNull() && rule.ExpiredObjectDeleteMarker.ValueBool()

		if hasExpiration && hasExpiredObjectDeleteMarker {
			resp.Diagnostics.AddError(
				"Invalid ILM policy configuration",
				fmt.Sprintf("rule[%d]: 'expiration' and 'expired_object_delete_marker' are mutually exclusive", i),
			)
			return
		}

		if hasExpiredObjectDeleteMarker && len(rule.Tags) > 0 {
			resp.Diagnostics.AddError(
				"Invalid ILM policy configuration",
				fmt.Sprintf("rule[%d]: delete-marker expiration is mutually exclusive with 'tags'", i),
			)
			return
		}
	}

	if err := r.applyILMPolicy(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error creating ILM policy",
			err.Error(),
		)
		return
	}

	data.ID = data.Bucket
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ilmPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ilmPolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// During import, ID is set but Bucket might not be
	if data.Bucket.IsNull() || data.Bucket.ValueString() == "" {
		data.Bucket = data.ID
	}

	if err := r.readILMPolicy(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error reading ILM policy",
			err.Error(),
		)
		return
	}

	if data.ID.ValueString() == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ilmPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ilmPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for i, rule := range data.Rules {
		hasExpiration := !rule.Expiration.IsNull() && rule.Expiration.ValueString() != "" && rule.Expiration.ValueString() != "DeleteMarker"
		hasExpiredObjectDeleteMarker := !rule.ExpiredObjectDeleteMarker.IsNull() && rule.ExpiredObjectDeleteMarker.ValueBool()

		if hasExpiration && hasExpiredObjectDeleteMarker {
			resp.Diagnostics.AddError(
				"Invalid ILM policy configuration",
				fmt.Sprintf("rule[%d]: 'expiration' and 'expired_object_delete_marker' are mutually exclusive", i),
			)
			return
		}

		if hasExpiredObjectDeleteMarker && len(rule.Tags) > 0 {
			resp.Diagnostics.AddError(
				"Invalid ILM policy configuration",
				fmt.Sprintf("rule[%d]: delete-marker expiration is mutually exclusive with 'tags'", i),
			)
			return
		}
	}

	if err := r.applyILMPolicy(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error updating ILM policy",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ilmPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ilmPolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	c := r.client.S3Client

	config := lifecycle.NewConfiguration()

	if err := c.SetBucketLifecycle(ctx, data.Bucket.ValueString(), config); err != nil {
		if !isNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Error deleting ILM policy",
				err.Error(),
			)
			return
		}
	}

	resp.State.RemoveResource(ctx)
}

func (r *ilmPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *ilmPolicyResource) applyILMPolicy(ctx context.Context, data *ilmPolicyResourceModel) error {
	c := r.client.S3Client
	bucket := data.Bucket.ValueString()

	exists, err := c.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("validating bucket: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket %s does not exist", bucket)
	}

	oldConfig, err := c.GetBucketLifecycle(ctx, bucket)
	if err != nil && !isNotFoundError(err) && !isS3CompatNotSupported(r.client, err) {
		return fmt.Errorf("getting existing lifecycle: %w", err)
	}

	config := lifecycle.NewConfiguration()

	for _, ruleModel := range data.Rules {
		hasAbort := len(ruleModel.AbortIncompleteMultipartUpload) > 0
		hasSupportedAction := !ruleModel.Expiration.IsNull() ||
			len(ruleModel.Transition) > 0 ||
			len(ruleModel.NoncurrentExpiration) > 0 ||
			len(ruleModel.NoncurrentTransition) > 0 ||
			!ruleModel.ExpiredObjectDeleteMarker.IsNull()

		if hasAbort && !hasSupportedAction {
			continue
		}

		rule, err := r.createLifecycleRuleFromModel(ruleModel)
		if err != nil {
			return fmt.Errorf("creating lifecycle rule: %w", err)
		}

		config.Rules = append(config.Rules, rule)
	}

	if len(config.Rules) > 0 {
		if err := c.SetBucketLifecycle(ctx, bucket, config); err != nil {
			if oldConfig != nil {
				if rbErr := c.SetBucketLifecycle(ctx, bucket, oldConfig); rbErr != nil {
					return fmt.Errorf("setting lifecycle (rollback also failed): %v, rollback error: %w", err, rbErr)
				}
			}
			return fmt.Errorf("setting lifecycle: %w", err)
		}
	}

	data.ID = data.Bucket

	return r.readILMPolicy(ctx, data)
}

func (r *ilmPolicyResource) createLifecycleRuleFromModel(model ilmPolicyRuleModel) (lifecycle.Rule, error) {
	id := model.ID.ValueString()
	if id == "" {
		return lifecycle.Rule{}, fmt.Errorf("rule id is required")
	}

	status := model.Status.ValueString()
	if status == "" {
		status = "Enabled"
	}

	var filter lifecycle.Filter
	tags := make([]lifecycle.Tag, 0)

	if len(model.Tags) > 0 {
		prefix := model.Filter.ValueString()
		filter.And.Prefix = prefix
		for k, v := range model.Tags {
			tags = append(tags, lifecycle.Tag{Key: k, Value: v.ValueString()})
		}
		filter.And.Tags = tags
	} else {
		prefix := model.Filter.ValueString()
		filter.Prefix = prefix
	}

	if filter.IsNull() {
		filter.ObjectSizeGreaterThan = emptyFilterSentinel
	}

	expiration := r.parseExpiration(model)
	transition := r.parseTransition(model.Transition)
	noncurrentExpiration := r.parseNoncurrentExpiration(model.NoncurrentExpiration)
	noncurrentTransition := r.parseNoncurrentTransition(model.NoncurrentTransition)
	abortIncomplete := r.parseAbortIncomplete(model.AbortIncompleteMultipartUpload)

	return lifecycle.Rule{
		ID:                             id,
		Status:                         status,
		Expiration:                     expiration,
		Transition:                     transition,
		NoncurrentVersionExpiration:    noncurrentExpiration,
		NoncurrentVersionTransition:    noncurrentTransition,
		AbortIncompleteMultipartUpload: abortIncomplete,
		RuleFilter:                     filter,
	}, nil
}

func (r *ilmPolicyResource) parseExpiration(model ilmPolicyRuleModel) lifecycle.Expiration {
	expirationStr := model.Expiration.ValueString()
	expiredObjDeleteMarker := model.ExpiredObjectDeleteMarker.ValueBool()

	if expirationStr == "DeleteMarker" {
		return lifecycle.Expiration{DeleteMarker: true}
	}

	if expiredObjDeleteMarker {
		return lifecycle.Expiration{DeleteMarker: true}
	}

	if expirationStr == "" {
		return lifecycle.Expiration{}
	}

	var days int
	if _, err := fmt.Sscanf(expirationStr, "%dd", &days); err == nil {
		return lifecycle.Expiration{Days: lifecycle.ExpirationDays(days)}
	}

	if date, err := time.Parse("2006-01-02", expirationStr); err == nil {
		return lifecycle.Expiration{Date: lifecycle.ExpirationDate{Time: date}}
	}

	return lifecycle.Expiration{}
}

func (r *ilmPolicyResource) parseTransition(transitions []ilmTransitionModel) lifecycle.Transition {
	if len(transitions) == 0 {
		return lifecycle.Transition{}
	}

	t := transitions[0]
	if t.StorageClass.IsNull() || t.StorageClass.ValueString() == "" {
		return lifecycle.Transition{}
	}

	if !t.Days.IsNull() && t.Days.ValueString() != "" {
		var days int
		if _, err := fmt.Sscanf(t.Days.ValueString(), "%dd", &days); err == nil {
			return lifecycle.Transition{
				Days:         lifecycle.ExpirationDays(days),
				StorageClass: t.StorageClass.ValueString(),
			}
		}
	}

	if !t.Date.IsNull() && t.Date.ValueString() != "" {
		if parsedDate, err := time.Parse("2006-01-02", t.Date.ValueString()); err == nil {
			return lifecycle.Transition{
				Date:         lifecycle.ExpirationDate{Time: parsedDate},
				StorageClass: t.StorageClass.ValueString(),
			}
		}
	}

	return lifecycle.Transition{}
}

func (r *ilmPolicyResource) parseNoncurrentExpiration(expirations []ilmNoncurrentExpirationModel) lifecycle.NoncurrentVersionExpiration {
	if len(expirations) == 0 {
		return lifecycle.NoncurrentVersionExpiration{}
	}

	e := expirations[0]
	if e.Days.IsNull() || e.Days.ValueString() == "" {
		return lifecycle.NoncurrentVersionExpiration{}
	}

	var days int
	if _, err := fmt.Sscanf(e.Days.ValueString(), "%dd", &days); err != nil {
		return lifecycle.NoncurrentVersionExpiration{}
	}

	newerVersions := int(e.NewerVersions.ValueInt64())

	return lifecycle.NoncurrentVersionExpiration{
		NoncurrentDays:          lifecycle.ExpirationDays(days),
		NewerNoncurrentVersions: newerVersions,
	}
}

func (r *ilmPolicyResource) parseNoncurrentTransition(transitions []ilmNoncurrentTransitionModel) lifecycle.NoncurrentVersionTransition {
	if len(transitions) == 0 {
		return lifecycle.NoncurrentVersionTransition{}
	}

	t := transitions[0]
	if t.Days.IsNull() || t.Days.ValueString() == "" {
		return lifecycle.NoncurrentVersionTransition{}
	}

	var days int
	if _, err := fmt.Sscanf(t.Days.ValueString(), "%dd", &days); err != nil {
		return lifecycle.NoncurrentVersionTransition{}
	}

	newerVersions := int(t.NewerVersions.ValueInt64())

	return lifecycle.NoncurrentVersionTransition{
		NoncurrentDays:          lifecycle.ExpirationDays(days),
		StorageClass:            t.StorageClass.ValueString(),
		NewerNoncurrentVersions: newerVersions,
	}
}

func (r *ilmPolicyResource) parseAbortIncomplete(abortList []ilmAbortIncompleteModel) lifecycle.AbortIncompleteMultipartUpload {
	if len(abortList) == 0 {
		return lifecycle.AbortIncompleteMultipartUpload{}
	}

	a := abortList[0]
	if a.DaysAfterInitiation.IsNull() || a.DaysAfterInitiation.ValueString() == "" {
		return lifecycle.AbortIncompleteMultipartUpload{}
	}

	var days int
	if _, err := fmt.Sscanf(a.DaysAfterInitiation.ValueString(), "%dd", &days); err != nil {
		return lifecycle.AbortIncompleteMultipartUpload{}
	}

	return lifecycle.AbortIncompleteMultipartUpload{
		DaysAfterInitiation: lifecycle.ExpirationDays(days),
	}
}

func (r *ilmPolicyResource) readILMPolicy(ctx context.Context, data *ilmPolicyResourceModel) error {
	c := r.client.S3Client

	config, err := c.GetBucketLifecycle(ctx, data.Bucket.ValueString())
	if err != nil {
		if isS3CompatNotSupported(r.client, err) {
			data.ID = types.StringValue("")
			return nil
		}
		if isNotFoundError(err) {
			data.ID = types.StringValue("")
			return nil
		}
		return fmt.Errorf("reading lifecycle configuration: %w", err)
	}

	rules := make([]ilmPolicyRuleModel, 0, len(config.Rules))

	for _, rule := range config.Rules {
		var expirationStr string
		var expiredObjectDeleteMarker types.Bool

		// Check if this is a pure expired_object_delete_marker rule (no days/date)
		isPureExpiredDM := bool(rule.Expiration.DeleteMarker) && rule.Expiration.Days == 0 && rule.Expiration.Date.IsZero()

		// Check if there's noncurrent version expiration (indicates expired_object_delete_marker is set)
		hasNoncurrentExpiration := rule.NoncurrentVersionExpiration.NoncurrentDays != 0

		if rule.Expiration.IsDeleteMarkerExpirationEnabled() {
			if rule.Expiration.Days != 0 {
				expirationStr = fmt.Sprintf("%dd", rule.Expiration.Days)
				expiredObjectDeleteMarker = types.BoolValue(true)
			} else if !rule.Expiration.Date.IsZero() {
				expirationStr = rule.Expiration.Date.Format("2006-01-02")
				expiredObjectDeleteMarker = types.BoolValue(true)
			} else if isPureExpiredDM && !hasNoncurrentExpiration {
				// Pure expired_object_delete_marker without noncurrent expiration
				expirationStr = "DeleteMarker"
				expiredObjectDeleteMarker = types.BoolNull()
			} else if isPureExpiredDM && hasNoncurrentExpiration {
				// expired_object_delete_marker with noncurrent expiration - don't set expiration
				expiredObjectDeleteMarker = types.BoolValue(true)
			} else {
				expiredObjectDeleteMarker = types.BoolNull()
			}
		} else if rule.Expiration.Days != 0 {
			expirationStr = fmt.Sprintf("%dd", rule.Expiration.Days)
			expiredObjectDeleteMarker = types.BoolNull()
		} else if !rule.Expiration.IsNull() && !rule.Expiration.Date.IsZero() {
			expirationStr = rule.Expiration.Date.Format("2006-01-02")
			expiredObjectDeleteMarker = types.BoolNull()
		} else {
			expiredObjectDeleteMarker = types.BoolNull()
		}

		var transitionList []ilmTransitionModel
		if !rule.Transition.IsNull() {
			transition := ilmTransitionModel{
				StorageClass: types.StringValue(rule.Transition.StorageClass),
			}
			if !rule.Transition.IsDaysNull() {
				transition.Days = types.StringValue(fmt.Sprintf("%dd", rule.Transition.Days))
			} else if !rule.Transition.IsDateNull() {
				transition.Date = types.StringValue(rule.Transition.Date.Format("2006-01-02"))
			}
			transitionList = append(transitionList, transition)
		}

		var noncurrentExpirationList []ilmNoncurrentExpirationModel
		if rule.NoncurrentVersionExpiration.NoncurrentDays != 0 {
			newerVersions := types.Int64Value(int64(rule.NoncurrentVersionExpiration.NewerNoncurrentVersions))
			if rule.NoncurrentVersionExpiration.NewerNoncurrentVersions == 0 {
				newerVersions = types.Int64Null()
			}
			noncurrentExpirationList = append(noncurrentExpirationList, ilmNoncurrentExpirationModel{
				Days:          types.StringValue(fmt.Sprintf("%dd", rule.NoncurrentVersionExpiration.NoncurrentDays)),
				NewerVersions: newerVersions,
			})
		}

		var noncurrentTransitionList []ilmNoncurrentTransitionModel
		if rule.NoncurrentVersionTransition.NoncurrentDays != 0 {
			newerVersions := types.Int64Value(int64(rule.NoncurrentVersionTransition.NewerNoncurrentVersions))
			if rule.NoncurrentVersionTransition.NewerNoncurrentVersions == 0 {
				newerVersions = types.Int64Null()
			}
			noncurrentTransitionList = append(noncurrentTransitionList, ilmNoncurrentTransitionModel{
				Days:          types.StringValue(fmt.Sprintf("%dd", rule.NoncurrentVersionTransition.NoncurrentDays)),
				StorageClass:  types.StringValue(rule.NoncurrentVersionTransition.StorageClass),
				NewerVersions: newerVersions,
			})
		}

		var abortList []ilmAbortIncompleteModel
		if rule.AbortIncompleteMultipartUpload.DaysAfterInitiation != 0 {
			abortList = append(abortList, ilmAbortIncompleteModel{
				DaysAfterInitiation: types.StringValue(fmt.Sprintf("%dd", rule.AbortIncompleteMultipartUpload.DaysAfterInitiation)),
			})
		}

		var prefix string
		tags := make(map[string]types.String)
		if len(rule.RuleFilter.And.Tags) > 0 {
			prefix = rule.RuleFilter.And.Prefix
			for _, tag := range rule.RuleFilter.And.Tags {
				tags[tag.Key] = types.StringValue(tag.Value)
			}
		} else {
			prefix = rule.RuleFilter.Prefix
		}

		status := types.StringValue(rule.Status)
		if rule.Status == "" || rule.Status == "Enabled" {
			status = types.StringNull()
		}

		var tagsValue map[string]types.String
		if len(rule.RuleFilter.And.Tags) > 0 {
			tagsValue = make(map[string]types.String)
			for _, tag := range rule.RuleFilter.And.Tags {
				tagsValue[tag.Key] = types.StringValue(tag.Value)
			}
		}

		ruleModel := ilmPolicyRuleModel{
			ID:                             types.StringValue(rule.ID),
			Status:                         status,
			Expiration:                     getStringOrNull(expirationStr),
			ExpiredObjectDeleteMarker:      expiredObjectDeleteMarker,
			Transition:                     transitionList,
			NoncurrentExpiration:           noncurrentExpirationList,
			NoncurrentTransition:           noncurrentTransitionList,
			AbortIncompleteMultipartUpload: abortList,
			Filter:                         getStringOrNull(prefix),
			Tags:                           tagsValue,
		}

		rules = append(rules, ruleModel)
	}

	data.Rules = rules

	return nil
}

func getStringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}
