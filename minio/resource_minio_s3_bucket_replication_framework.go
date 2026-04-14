package minio

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwp "github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/replication"
)

var (
	_ resource.Resource                = &bucketReplicationResource{}
	_ resource.ResourceWithConfigure   = &bucketReplicationResource{}
	_ resource.ResourceWithImportState = &bucketReplicationResource{}
)

type bucketReplicationResource struct {
	client *S3MinioClient
}

type bucketReplicationResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Bucket        types.String `tfsdk:"bucket"`
	ResyncVersion types.Int64  `tfsdk:"resync_version"`
	LastResyncID  types.String `tfsdk:"last_resync_id"`
	Rules         types.List   `tfsdk:"rule"`
}

type replicationRuleModel struct {
	ID                        types.String `tfsdk:"id"`
	Arn                       types.String `tfsdk:"arn"`
	Enabled                   types.Bool   `tfsdk:"enabled"`
	Priority                  types.Int64  `tfsdk:"priority"`
	Prefix                    types.String `tfsdk:"prefix"`
	Tags                      types.Map    `tfsdk:"tags"`
	DeleteReplication         types.Bool   `tfsdk:"delete_replication"`
	DeleteMarkerReplication   types.Bool   `tfsdk:"delete_marker_replication"`
	ExistingObjectReplication types.Bool   `tfsdk:"existing_object_replication"`
	MetadataSync              types.Bool   `tfsdk:"metadata_sync"`
	Target                    types.List   `tfsdk:"target"`
}

type replicationTargetModel struct {
	Bucket            types.String `tfsdk:"bucket"`
	StorageClass      types.String `tfsdk:"storage_class"`
	Host              types.String `tfsdk:"host"`
	Secure            types.Bool   `tfsdk:"secure"`
	PathStyle         types.String `tfsdk:"path_style"`
	Path              types.String `tfsdk:"path"`
	Synchronous       types.Bool   `tfsdk:"synchronous"`
	DisableProxy      types.Bool   `tfsdk:"disable_proxy"`
	HealthCheckPeriod types.String `tfsdk:"health_check_period"`
	BandwidthLimit    types.String `tfsdk:"bandwidth_limit"`
	Region            types.String `tfsdk:"region"`
	AccessKey         types.String `tfsdk:"access_key"`
	SecretKey         types.String `tfsdk:"secret_key"`
}

var replicationTargetObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"bucket":              types.StringType,
		"storage_class":       types.StringType,
		"host":                types.StringType,
		"secure":              types.BoolType,
		"path_style":          types.StringType,
		"path":                types.StringType,
		"synchronous":         types.BoolType,
		"disable_proxy":       types.BoolType,
		"health_check_period": types.StringType,
		"bandwidth_limit":     types.StringType,
		"region":              types.StringType,
		"access_key":          types.StringType,
		"secret_key":          types.StringType,
	},
}

var replicationRuleObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":                          types.StringType,
		"arn":                         types.StringType,
		"enabled":                     types.BoolType,
		"priority":                    types.Int64Type,
		"prefix":                      types.StringType,
		"tags":                        types.MapType{ElemType: types.StringType},
		"delete_replication":          types.BoolType,
		"delete_marker_replication":   types.BoolType,
		"existing_object_replication": types.BoolType,
		"metadata_sync":               types.BoolType,
		"target":                      types.ListType{ElemType: replicationTargetObjectType},
	},
}

func newBucketReplicationResource() resource.Resource {
	return &bucketReplicationResource{}
}

func (r *bucketReplicationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket_replication"
}

func (r *bucketReplicationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages MinIO bucket replication rules for cross-replica synchronization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Bucket name (used as resource ID)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Required:    true,
				Description: "Name of the bucket on which to setup replication rules",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resync_version": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				Description: "Increment this value to trigger a resync of existing objects for all replication rules. Each increment triggers one resync.",
			},
			"last_resync_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the last resync operation.",
			},
			"rule": schema.ListAttribute{
				Description: "Rule definitions",
				Optional:    true,
				ElementType: replicationRuleObjectType,
			},
		},
	}
}

func (r *bucketReplicationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *bucketReplicationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan bucketReplicationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating bucket replication configuration", map[string]interface{}{
		"bucket": plan.Bucket.ValueString(),
	})

	if err := r.applyReplication(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error creating bucket replication configuration",
			err.Error(),
		)
		return
	}

	if diags := r.readReplication(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *bucketReplicationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state bucketReplicationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading bucket replication configuration", map[string]interface{}{
		"bucket": state.Bucket.ValueString(),
	})

	if diags := r.readReplication(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *bucketReplicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan bucketReplicationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating bucket replication configuration", map[string]interface{}{
		"bucket": plan.Bucket.ValueString(),
	})

	if err := r.applyReplication(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error updating bucket replication configuration",
			err.Error(),
		)
		return
	}

	if diags := r.readReplication(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *bucketReplicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state bucketReplicationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting bucket replication configuration", map[string]interface{}{
		"bucket": state.Bucket.ValueString(),
	})

	if err := r.deleteReplication(ctx, &state); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting bucket replication configuration",
			err.Error(),
		)
		return
	}
}

func (r *bucketReplicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, fwp.Root("bucket"), req, resp)
}

func (r *bucketReplicationResource) readReplication(ctx context.Context, model *bucketReplicationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rules, d := r.readReplicationRules(ctx, model.Bucket.ValueString())
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	model.Rules = rules
	model.ID = model.Bucket

	return diags
}

func (r *bucketReplicationResource) readReplicationRules(ctx context.Context, bucket string) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	client := r.client.S3Client
	admClient := r.client.S3Admin

	config, err := client.GetBucketReplication(ctx, bucket)
	if err != nil {
		if strings.Contains(err.Error(), "BucketReplicationNotFound") || strings.Contains(err.Error(), "replication configuration does not exist") {
			return types.ListNull(replicationRuleObjectType), diags
		}
		diags.AddError("Reading replication configuration", fmt.Sprintf("Failed to read replication config: %s", err))
		return types.ListNull(replicationRuleObjectType), diags
	}

	if len(config.Rules) == 0 {
		return types.ListNull(replicationRuleObjectType), diags
	}

	ruleObjects := make([]attr.Value, 0, len(config.Rules))

	targets, err := admClient.ListRemoteTargets(ctx, bucket, "")
	if err != nil {
		diags.AddError("Reading remote targets", fmt.Sprintf("Failed to read remote targets: %s", err))
		return types.ListNull(replicationRuleObjectType), diags
	}

	arnMap := make(map[string]madmin.BucketTarget)
	for _, target := range targets {
		arnMap[target.Arn] = target
	}

	for _, rule := range config.Rules {
		ruleObj, d := r.flattenReplicationRule(ctx, rule, arnMap)
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(replicationRuleObjectType), diags
		}
		ruleObjects = append(ruleObjects, ruleObj)
	}

	return types.ListValue(replicationRuleObjectType, ruleObjects)
}

func (r *bucketReplicationResource) flattenReplicationRule(ctx context.Context, rule replication.Rule, targets map[string]madmin.BucketTarget) (attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics

	ruleModel := replicationRuleModel{}

	ruleModel.ID = types.StringValue(rule.ID)

	if rule.Status == "Enabled" {
		ruleModel.Enabled = types.BoolValue(true)
	} else {
		ruleModel.Enabled = types.BoolValue(false)
	}

	if rule.Priority > 0 {
		ruleModel.Priority = types.Int64Value(int64(rule.Priority))
	}

	if rule.Filter.Prefix != "" {
		ruleModel.Prefix = types.StringValue(rule.Filter.Prefix)
	}

	tagsMap := make(map[string]attr.Value)
	hasTags := false

	if len(rule.Filter.And.Tags) > 0 {
		for _, tag := range rule.Filter.And.Tags {
			if !tag.IsEmpty() {
				hasTags = true
				tagsMap[tag.Key] = types.StringValue(tag.Value)
			}
		}
	} else if rule.Filter.Tag.Key != "" {
		hasTags = true
		tagsMap[rule.Filter.Tag.Key] = types.StringValue(rule.Filter.Tag.Value)
	}

	if hasTags {
		tagsObj, d := types.MapValue(types.StringType, tagsMap)
		diags.Append(d...)
		if diags.HasError() {
			return types.ObjectNull(replicationRuleObjectType.AttrTypes), diags
		}
		ruleModel.Tags = tagsObj
	}

	if rule.DeleteReplication.Status == "Enabled" {
		ruleModel.DeleteReplication = types.BoolValue(true)
	} else {
		ruleModel.DeleteReplication = types.BoolValue(false)
	}

	if rule.DeleteMarkerReplication.Status == "Enabled" {
		ruleModel.DeleteMarkerReplication = types.BoolValue(true)
	} else {
		ruleModel.DeleteMarkerReplication = types.BoolValue(false)
	}

	if rule.ExistingObjectReplication.Status == "Enabled" {
		ruleModel.ExistingObjectReplication = types.BoolValue(true)
	} else {
		ruleModel.ExistingObjectReplication = types.BoolValue(false)
	}

	if rule.SourceSelectionCriteria.ReplicaModifications.Status == "Enabled" {
		ruleModel.MetadataSync = types.BoolValue(true)
	} else {
		ruleModel.MetadataSync = types.BoolValue(false)
	}

	targetsList, d := r.flattenReplicationTargets(ctx, rule, targets)
	diags.Append(d...)
	if diags.HasError() {
		return types.ObjectNull(replicationRuleObjectType.AttrTypes), diags
	}
	ruleModel.Target = targetsList

	obj, d := types.ObjectValue(replicationRuleObjectType.AttrTypes, map[string]attr.Value{
		"id":                          ruleModel.ID,
		"arn":                         types.StringNull(),
		"enabled":                     ruleModel.Enabled,
		"priority":                    ruleModel.Priority,
		"prefix":                      ruleModel.Prefix,
		"tags":                        ruleModel.Tags,
		"delete_replication":          ruleModel.DeleteReplication,
		"delete_marker_replication":   ruleModel.DeleteMarkerReplication,
		"existing_object_replication": ruleModel.ExistingObjectReplication,
		"metadata_sync":               ruleModel.MetadataSync,
		"target":                      ruleModel.Target,
	})
	diags.Append(d...)
	if diags.HasError() {
		return types.ObjectNull(replicationRuleObjectType.AttrTypes), diags
	}

	return obj, diags
}

func (r *bucketReplicationResource) flattenReplicationTargets(ctx context.Context, rule replication.Rule, targets map[string]madmin.BucketTarget) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	if rule.Destination.Bucket == "" {
		return types.ListNull(replicationTargetObjectType), diags
	}

	var targetTarget madmin.BucketTarget
	found := false
	for _, t := range targets {
		if t.Arn == rule.Destination.Bucket {
			targetTarget = t
			found = true
			break
		}
	}

	if !found {
		return types.ListNull(replicationTargetObjectType), diags
	}

	targetModel := replicationTargetModel{}
	targetModel.Bucket = types.StringValue(rule.Destination.Bucket)

	if rule.Destination.StorageClass != "" {
		targetModel.StorageClass = types.StringValue(rule.Destination.StorageClass)
	}

	parts := strings.SplitN(targetTarget.TargetBucket, "/", 4)
	if len(parts) >= 3 {
		targetModel.Host = types.StringValue(targetTarget.Endpoint)
	}

	targetModel.Secure = types.BoolValue(targetTarget.Secure)
	targetModel.PathStyle = types.StringValue(strings.ToLower(targetTarget.Path))
	targetModel.DisableProxy = types.BoolValue(targetTarget.DisableProxy)
	targetModel.Synchronous = types.BoolValue(targetTarget.ReplicationSync)
	targetModel.HealthCheckPeriod = types.StringValue(shortDur(targetTarget.HealthCheckDuration))

	var bwUint64 uint64
	if targetTarget.BandwidthLimit > 0 {
		bwUint64 = uint64(targetTarget.BandwidthLimit)
		targetModel.BandwidthLimit = types.StringValue(humanize.Bytes(bwUint64))
	} else {
		targetModel.BandwidthLimit = types.StringValue("0")
	}

	targetModel.Region = types.StringValue(targetTarget.Region)
	targetModel.AccessKey = types.StringValue(targetTarget.Credentials.AccessKey)
	targetModel.SecretKey = types.StringNull()

	obj, d := types.ObjectValue(replicationTargetObjectType.AttrTypes, map[string]attr.Value{
		"bucket":              targetModel.Bucket,
		"storage_class":       targetModel.StorageClass,
		"host":                targetModel.Host,
		"secure":              targetModel.Secure,
		"path_style":          targetModel.PathStyle,
		"path":                targetModel.Path,
		"synchronous":         targetModel.Synchronous,
		"disable_proxy":       targetModel.DisableProxy,
		"health_check_period": targetModel.HealthCheckPeriod,
		"bandwidth_limit":     targetModel.BandwidthLimit,
		"region":              targetModel.Region,
		"access_key":          targetModel.AccessKey,
		"secret_key":          targetModel.SecretKey,
	})
	diags.Append(d...)
	if diags.HasError() {
		return types.ListNull(replicationTargetObjectType), diags
	}

	targetList, d := types.ListValue(replicationTargetObjectType, []attr.Value{obj})
	diags.Append(d...)
	if diags.HasError() {
		return types.ListNull(replicationTargetObjectType), diags
	}

	return targetList, diags
}

func (r *bucketReplicationResource) applyReplication(ctx context.Context, model *bucketReplicationResourceModel) error {
	var rules []replication.Rule

	if model.Rules.IsNull() || model.Rules.IsUnknown() {
		return nil
	}

	var ruleModels []replicationRuleModel
	diags := model.Rules.ElementsAs(ctx, &ruleModels, false)
	if diags.HasError() {
		return fmt.Errorf("failed to parse rules: %v", diags)
	}

	client := r.client.S3Client
	admClient := r.client.S3Admin
	bucket := model.Bucket.ValueString()

	for i, ruleModel := range ruleModels {
		rule, target, err := r.expandReplicationRule(ctx, &ruleModel, i)
		if err != nil {
			return fmt.Errorf("failed to expand rule %d: %w", i, err)
		}
		rules = append(rules, rule)

		if err := r.ensureRemoteTarget(ctx, admClient, bucket, &target); err != nil {
			return fmt.Errorf("failed to create remote target for rule %d: %w", i, err)
		}
	}

	config := replication.Config{
		Rules: rules,
	}

	err := client.SetBucketReplication(ctx, bucket, config)
	if err != nil {
		return fmt.Errorf("failed to set replication config: %w", err)
	}

	return nil
}

func (r *bucketReplicationResource) expandReplicationRule(ctx context.Context, model *replicationRuleModel, index int) (replication.Rule, madmin.BucketTarget, error) {
	rule := replication.Rule{
		ID:          model.ID.ValueString(),
		Priority:    index + 1,
		Status:      "Enabled",
		Filter:      replication.Filter{},
		Destination: replication.Destination{},
	}

	if !model.Enabled.IsNull() && !model.Enabled.IsUnknown() {
		if model.Enabled.ValueBool() {
			rule.Status = "Enabled"
		} else {
			rule.Status = "Disabled"
		}
	}

	if !model.Priority.IsNull() && !model.Priority.IsUnknown() {
		rule.Priority = int(model.Priority.ValueInt64())
	}

	if !model.Prefix.IsNull() && !model.Prefix.IsUnknown() {
		rule.Filter.Prefix = model.Prefix.ValueString()
	}

	if !model.Tags.IsNull() && !model.Tags.IsUnknown() {
		var tagsMap map[string]string
		diags := model.Tags.ElementsAs(ctx, &tagsMap, false)
		if diags.HasError() {
			return replication.Rule{}, madmin.BucketTarget{}, fmt.Errorf("failed to parse tags: %v", diags)
		}
		for k, v := range tagsMap {
			rule.Filter.And.Tags = append(rule.Filter.And.Tags, replication.Tag{
				Key:   k,
				Value: v,
			})
		}
	}

	if !model.DeleteReplication.IsNull() && !model.DeleteReplication.IsUnknown() {
		if model.DeleteReplication.ValueBool() {
			rule.DeleteReplication.Status = "Enabled"
		}
	}

	if !model.DeleteMarkerReplication.IsNull() && !model.DeleteMarkerReplication.IsUnknown() {
		if model.DeleteMarkerReplication.ValueBool() {
			rule.DeleteMarkerReplication.Status = "Enabled"
		}
	}

	if !model.ExistingObjectReplication.IsNull() && !model.ExistingObjectReplication.IsUnknown() {
		if model.ExistingObjectReplication.ValueBool() {
			rule.ExistingObjectReplication.Status = "Enabled"
		}
	}

	if !model.MetadataSync.IsNull() && !model.MetadataSync.IsUnknown() {
		if model.MetadataSync.ValueBool() {
			rule.SourceSelectionCriteria.ReplicaModifications.Status = "Enabled"
		}
	}

	var target madmin.BucketTarget
	if !model.Target.IsNull() && !model.Target.IsUnknown() {
		var targetModels []replicationTargetModel
		diags := model.Target.ElementsAs(ctx, &targetModels, false)
		if diags.HasError() {
			return replication.Rule{}, madmin.BucketTarget{}, fmt.Errorf("failed to parse targets: %v", diags)
		}

		if len(targetModels) > 0 {
			t := targetModels[0]
			rule.Destination.Bucket = t.Bucket.ValueString()

			if !t.StorageClass.IsNull() && !t.StorageClass.IsUnknown() {
				rule.Destination.StorageClass = t.StorageClass.ValueString()
			}

			tgtBucket := t.Bucket.ValueString()
			if !t.Path.IsNull() && !t.Path.IsUnknown() && t.Path.ValueString() != "" {
				tgtBucket = path.Clean("./" + t.Path.ValueString() + "/" + tgtBucket)
			}

			creds := &madmin.Credentials{
				AccessKey: t.AccessKey.ValueString(),
				SecretKey: t.SecretKey.ValueString(),
			}

			pathStyle := "auto"
			if !t.PathStyle.IsNull() && !t.PathStyle.IsUnknown() {
				pathStyle = t.PathStyle.ValueString()
			}

			healthCheckDuration := 30 * time.Second
			if !t.HealthCheckPeriod.IsNull() && !t.HealthCheckPeriod.IsUnknown() {
				if d, err := time.ParseDuration(t.HealthCheckPeriod.ValueString()); err == nil {
					healthCheckDuration = d
				}
			}

			bandwidthLimit := int64(0)
			if !t.BandwidthLimit.IsNull() && !t.BandwidthLimit.IsUnknown() && t.BandwidthLimit.ValueString() != "0" {
				if bw, err := humanize.ParseBytes(t.BandwidthLimit.ValueString()); err == nil {
					bandwidthLimit = int64(bw) // #nosec G115 -- bandwidth values from user config are within int64 range
				}
			}

			target = madmin.BucketTarget{
				TargetBucket:        tgtBucket,
				Secure:              t.Secure.ValueBool(),
				Credentials:         creds,
				Endpoint:            t.Host.ValueString(),
				Path:                pathStyle,
				API:                 "s3v4",
				Type:                madmin.ReplicationService,
				Region:              t.Region.ValueString(),
				BandwidthLimit:      bandwidthLimit,
				ReplicationSync:     t.Synchronous.ValueBool(),
				DisableProxy:        t.DisableProxy.ValueBool(),
				HealthCheckDuration: healthCheckDuration,
			}
		}
	}

	return rule, target, nil
}

func (r *bucketReplicationResource) ensureRemoteTarget(ctx context.Context, admClient *madmin.AdminClient, bucket string, target *madmin.BucketTarget) error {
	if target.TargetBucket == "" {
		return nil
	}

	existingTargets, err := admClient.ListRemoteTargets(ctx, bucket, "")
	if err != nil {
		return fmt.Errorf("failed to list remote targets: %w", err)
	}

	for _, existing := range existingTargets {
		if existing.Endpoint == target.Endpoint && existing.TargetBucket == target.TargetBucket {
			return nil
		}
	}

	_, err = admClient.SetRemoteTarget(ctx, bucket, target)
	if err != nil {
		return fmt.Errorf("failed to set remote target: %w", err)
	}

	return nil
}

func (r *bucketReplicationResource) deleteReplication(ctx context.Context, model *bucketReplicationResourceModel) error {
	client := r.client.S3Client
	admClient := r.client.S3Admin
	bucket := model.Bucket.ValueString()

	config, err := client.GetBucketReplication(ctx, bucket)
	if err != nil {
		if strings.Contains(err.Error(), "BucketReplicationNotFound") || strings.Contains(err.Error(), "replication configuration does not exist") {
			return nil
		}
		return fmt.Errorf("failed to get replication config: %w", err)
	}

	config.Rules = []replication.Rule{}
	if err := client.SetBucketReplication(ctx, bucket, config); err != nil {
		return fmt.Errorf("failed to clear replication config: %w", err)
	}

	targets, err := admClient.ListRemoteTargets(ctx, bucket, "")
	if err != nil {
		return fmt.Errorf("failed to list remote targets: %w", err)
	}

	for _, target := range targets {
		if err := admClient.RemoveRemoteTarget(ctx, bucket, target.Arn); err != nil {
			return fmt.Errorf("failed to remove remote target %s: %w", target.Arn, err)
		}
	}

	return nil
}
