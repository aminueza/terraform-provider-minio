package minio

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwp "github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/replication"
	"github.com/minio/minio-go/v7/pkg/s3utils"
)

var (
	_ resource.Resource                = &bucketReplicationResource{}
	_ resource.ResourceWithConfigure   = &bucketReplicationResource{}
	_ resource.ResourceWithImportState = &bucketReplicationResource{}
	_ resource.ResourceWithModifyPlan  = &bucketReplicationResource{}
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
		},
		Blocks: map[string]schema.Block{
			"rule": schema.ListNestedBlock{
				Description: "Rule definitions",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Rule ID",
						},
						"arn": schema.StringAttribute{
							Computed:    true,
							Description: "Rule ARN",
						},
						"enabled": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(true),
							Description: "Whether the rule is enabled",
						},
						"priority": schema.Int64Attribute{
							Optional:    true,
							Computed:    true,
							Description: "Rule priority (lower number = higher priority)",
						},
						"prefix": schema.StringAttribute{
							Optional:    true,
							Description: "Object prefix to replicate",
						},
						"tags": schema.MapAttribute{
							Optional:    true,
							ElementType: types.StringType,
							Description: "Tags to filter objects for replication",
						},
						"delete_replication": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Whether to replicate delete operations",
						},
						"delete_marker_replication": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Whether to replicate delete markers",
						},
						"existing_object_replication": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Whether to replicate existing objects",
						},
						"metadata_sync": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Whether to sync object metadata",
						},
					},
					Blocks: map[string]schema.Block{
						"target": schema.ListNestedBlock{
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"bucket": schema.StringAttribute{
										Required:    true,
										Description: "Target bucket name",
									},
									"storage_class": schema.StringAttribute{
										Optional:    true,
										Description: "Storage class for replicated objects",
									},
									"host": schema.StringAttribute{
										Required:    true,
										Description: "Target endpoint host",
									},
									"secure": schema.BoolAttribute{
										Optional:    true,
										Computed:    true,
										Default:     booldefault.StaticBool(true),
										Description: "Use HTTPS for target connection",
									},
									"path_style": schema.StringAttribute{
										Optional:    true,
										Computed:    true,
										Default:     stringdefault.StaticString("auto"),
										Description: "Path style for target (auto, on, off)",
									},
									"path": schema.StringAttribute{
										Optional:    true,
										Description: "Target path prefix",
									},
									"synchronous": schema.BoolAttribute{
										Optional:    true,
										Computed:    true,
										Description: "Whether replication is synchronous",
									},
									"disable_proxy": schema.BoolAttribute{
										Optional:    true,
										Computed:    true,
										Description: "Disable proxy for target",
									},
									"health_check_period": schema.StringAttribute{
										Optional:    true,
										Computed:    true,
										Default:     stringdefault.StaticString("30s"),
										Description: "Health check period (e.g., '30s')",
									},
									"bandwidth_limit": schema.StringAttribute{
										Optional:    true,
										Description: "Bandwidth limit (e.g., '100MB')",
									},
									"region": schema.StringAttribute{
										Optional:    true,
										Description: "Target region",
									},
									"access_key": schema.StringAttribute{
										Required:    true,
										Description: "Target access key",
									},
									"secret_key": schema.StringAttribute{
										Optional:    true,
										Sensitive:   true,
										WriteOnly:   true,
										Description: "Target secret key (write-only, not returned by MinIO API)",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ModifyPlan preserves secret_key from state for replication targets, since MinIO API does not
// return secret_key on read. Without this, every Read would clear the user's secret_key value.
func (r *bucketReplicationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Skip on destroy.
	if req.Plan.Raw.IsNull() {
		return
	}

	// No state on first create; nothing to preserve.
	if req.State.Raw.IsNull() {
		return
	}

	var planModel, stateModel bucketReplicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planModel)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &stateModel)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if planModel.Rules.IsNull() || planModel.Rules.IsUnknown() {
		return
	}
	if stateModel.Rules.IsNull() || stateModel.Rules.IsUnknown() {
		return
	}

	var planRules []replicationRuleModel
	resp.Diagnostics.Append(planModel.Rules.ElementsAs(ctx, &planRules, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var stateRules []replicationRuleModel
	resp.Diagnostics.Append(stateModel.Rules.ElementsAs(ctx, &stateRules, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stateRulesByID := make(map[string]replicationRuleModel, len(stateRules))
	for _, rule := range stateRules {
		if id := rule.ID.ValueString(); id != "" {
			stateRulesByID[id] = rule
		}
	}

	modified := false
	for i, planRule := range planRules {
		stateRule, exists := stateRulesByID[planRule.ID.ValueString()]
		if !exists {
			continue
		}

		var planTargets []replicationTargetModel
		resp.Diagnostics.Append(planRule.Target.ElementsAs(ctx, &planTargets, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		var stateTargets []replicationTargetModel
		resp.Diagnostics.Append(stateRule.Target.ElementsAs(ctx, &stateTargets, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		stateTargetsByBucket := make(map[string]replicationTargetModel, len(stateTargets))
		for _, t := range stateTargets {
			if b := t.Bucket.ValueString(); b != "" {
				stateTargetsByBucket[b] = t
			}
		}

		targetModified := false
		for j, planTarget := range planTargets {
			bucket := planTarget.Bucket.ValueString()
			if bucket == "" {
				continue
			}
			// User set a new explicit key; don't override.
			if !planTarget.SecretKey.IsNull() && !planTarget.SecretKey.IsUnknown() && planTarget.SecretKey.ValueString() != "" {
				continue
			}
			stateTarget, ok := stateTargetsByBucket[bucket]
			if !ok {
				continue
			}
			if !stateTarget.SecretKey.IsNull() && !stateTarget.SecretKey.IsUnknown() && stateTarget.SecretKey.ValueString() != "" {
				planTargets[j].SecretKey = stateTarget.SecretKey
				targetModified = true
			}
		}

		if targetModified {
			newTargets, diags := types.ListValueFrom(ctx, replicationTargetObjectType, planTargets)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}
			planRules[i].Target = newTargets
			modified = true
		}
	}

	if modified {
		newRules, diags := types.ListValueFrom(ctx, replicationRuleObjectType, planRules)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, fwp.Root("rule"), newRules)...)
	}
}

// overlaySecretKeysFromConfig copies secret_key values from the user's config onto the plan model.
// Since secret_key is WriteOnly it is never present in plan or state, but we still need it during
// Create/Update to authenticate with the remote MinIO when setting a replication target.
// Rules are matched by index (order in config matches order in plan), and targets by bucket name.
func (r *bucketReplicationResource) overlaySecretKeysFromConfig(ctx context.Context, config tfsdk.Config, plan *bucketReplicationResourceModel) diag.Diagnostics {
	return r.overlayFromConfig(ctx, config, plan, func(dst, src *replicationTargetModel) bool {
		if !src.SecretKey.IsNull() && !src.SecretKey.IsUnknown() && src.SecretKey.ValueString() != "" {
			dst.SecretKey = src.SecretKey
			return true
		}
		return false
	})
}

// preserveBandwidthLimitFormat restores the user's original bandwidth_limit string from config whenever
// it represents the same number of bytes as what MinIO returned (e.g. user wrote "100M", MinIO returns
// "100 MB"). Without this, Terraform flags an inconsistency between plan and post-apply state.
func (r *bucketReplicationResource) preserveBandwidthLimitFormat(ctx context.Context, config tfsdk.Config, plan *bucketReplicationResourceModel) diag.Diagnostics {
	return r.overlayFromConfig(ctx, config, plan, func(dst, src *replicationTargetModel) bool {
		if src.BandwidthLimit.IsNull() || src.BandwidthLimit.IsUnknown() {
			return false
		}
		if dst.BandwidthLimit.IsNull() || dst.BandwidthLimit.IsUnknown() {
			return false
		}
		if src.BandwidthLimit.ValueString() == dst.BandwidthLimit.ValueString() {
			return false
		}
		srcBytes, errS := humanize.ParseBytes(src.BandwidthLimit.ValueString())
		dstBytes, errD := humanize.ParseBytes(dst.BandwidthLimit.ValueString())
		if errS != nil || errD != nil || srcBytes != dstBytes {
			return false
		}
		dst.BandwidthLimit = src.BandwidthLimit
		return true
	})
}

// overlayFromConfig walks the config's rules/targets in parallel with plan's and invokes `apply` for
// each matching target pair. Rules match by index, targets match by bucket name. Returns diagnostics
// and commits any modifications back into plan.Rules.
func (r *bucketReplicationResource) overlayFromConfig(
	ctx context.Context,
	config tfsdk.Config,
	plan *bucketReplicationResourceModel,
	apply func(dst, src *replicationTargetModel) bool,
) diag.Diagnostics {
	var diags diag.Diagnostics

	if plan.Rules.IsNull() || plan.Rules.IsUnknown() {
		return diags
	}

	var configModel bucketReplicationResourceModel
	diags.Append(config.Get(ctx, &configModel)...)
	if diags.HasError() {
		return diags
	}

	if configModel.Rules.IsNull() || configModel.Rules.IsUnknown() {
		return diags
	}

	var planRules []replicationRuleModel
	diags.Append(plan.Rules.ElementsAs(ctx, &planRules, false)...)
	if diags.HasError() {
		return diags
	}

	var configRules []replicationRuleModel
	diags.Append(configModel.Rules.ElementsAs(ctx, &configRules, false)...)
	if diags.HasError() {
		return diags
	}

	modified := false
	for i := range planRules {
		if i >= len(configRules) {
			break
		}

		var planTargets []replicationTargetModel
		diags.Append(planRules[i].Target.ElementsAs(ctx, &planTargets, false)...)
		if diags.HasError() {
			return diags
		}

		var configTargets []replicationTargetModel
		diags.Append(configRules[i].Target.ElementsAs(ctx, &configTargets, false)...)
		if diags.HasError() {
			return diags
		}

		configTargetsByBucket := make(map[string]replicationTargetModel, len(configTargets))
		for _, t := range configTargets {
			if b := t.Bucket.ValueString(); b != "" {
				configTargetsByBucket[b] = t
			}
		}

		targetModified := false
		for j := range planTargets {
			bucket := planTargets[j].Bucket.ValueString()
			if bucket == "" {
				continue
			}
			configTarget, ok := configTargetsByBucket[bucket]
			if !ok {
				continue
			}
			if apply(&planTargets[j], &configTarget) {
				targetModified = true
			}
		}

		if targetModified {
			newTargets, d := types.ListValueFrom(ctx, replicationTargetObjectType, planTargets)
			diags.Append(d...)
			if diags.HasError() {
				return diags
			}
			planRules[i].Target = newTargets
			modified = true
		}
	}

	if modified {
		newRules, d := types.ListValueFrom(ctx, replicationRuleObjectType, planRules)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		plan.Rules = newRules
	}

	return diags
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

	// secret_key is WriteOnly, so it's never present in plan or state — read it from config.
	resp.Diagnostics.Append(r.overlaySecretKeysFromConfig(ctx, req.Config, &plan)...)
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

	resp.Diagnostics.Append(r.preserveBandwidthLimitFormat(ctx, req.Config, &plan)...)
	if resp.Diagnostics.HasError() {
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

	// Preserve secret_key from state before reading, since MinIO API doesn't return it.
	var stateRules []replicationRuleModel
	if !state.Rules.IsNull() && !state.Rules.IsUnknown() {
		diags := state.Rules.ElementsAs(ctx, &stateRules, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if diags := r.readReplication(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// Restore secret_key from state for each matching rule/target.
	if !state.Rules.IsNull() && !state.Rules.IsUnknown() && len(stateRules) > 0 {
		var readRules []replicationRuleModel
		diags := state.Rules.ElementsAs(ctx, &readRules, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		// Match rules by index — MinIO often returns an empty rule ID, so ID-based matching is unreliable.
		// Rules are position-stable from the user's config, so index matching preserves state correctly.
		for i, readRule := range readRules {
			if i >= len(stateRules) {
				break
			}
			stateRule := stateRules[i]

			var stateTargets []replicationTargetModel
			diags := stateRule.Target.ElementsAs(ctx, &stateTargets, false)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}

			var readTargets []replicationTargetModel
			diags = readRule.Target.ElementsAs(ctx, &readTargets, false)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}

			// Build map of state targets by bucket.
			stateTargetsByBucket := make(map[string]replicationTargetModel)
			for _, target := range stateTargets {
				bucket := target.Bucket.ValueString()
				if bucket != "" {
					stateTargetsByBucket[bucket] = target
				}
			}

			// Restore secret_key for matching targets.
			for j, readTarget := range readTargets {
				bucket := readTarget.Bucket.ValueString()
				if bucket == "" {
					continue
				}

				stateTarget, exists := stateTargetsByBucket[bucket]
				if !exists {
					continue
				}

				// Restore secret_key from state if it exists.
				if !stateTarget.SecretKey.IsNull() && !stateTarget.SecretKey.IsUnknown() && stateTarget.SecretKey.ValueString() != "" {
					readTargets[j].SecretKey = stateTarget.SecretKey
				}

				// Preserve bandwidth_limit's original string form when byte-equivalent.
				// MinIO normalizes "100M" → "100 MB"; keeping state's format avoids a spurious drift.
				if !stateTarget.BandwidthLimit.IsNull() && !stateTarget.BandwidthLimit.IsUnknown() &&
					!readTarget.BandwidthLimit.IsNull() && !readTarget.BandwidthLimit.IsUnknown() &&
					stateTarget.BandwidthLimit.ValueString() != readTarget.BandwidthLimit.ValueString() {
					stateBytes, errS := humanize.ParseBytes(stateTarget.BandwidthLimit.ValueString())
					readBytes, errR := humanize.ParseBytes(readTarget.BandwidthLimit.ValueString())
					if errS == nil && errR == nil && stateBytes == readBytes {
						readTargets[j].BandwidthLimit = stateTarget.BandwidthLimit
					}
				}
			}

			// Update read rule targets.
			targets, diags := types.ListValueFrom(ctx, replicationTargetObjectType, readTargets)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}
			readRules[i].Target = targets
		}

		// Update state rules with restored secret_key values.
		rules, diags := types.ListValueFrom(ctx, replicationRuleObjectType, readRules)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Rules = rules
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *bucketReplicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan bucketReplicationResourceModel
	var state bucketReplicationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// secret_key is WriteOnly, so it's never present in plan or state — read it from config.
	resp.Diagnostics.Append(r.overlaySecretKeysFromConfig(ctx, req.Config, &plan)...)
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

	resp.Diagnostics.Append(r.preserveBandwidthLimitFormat(ctx, req.Config, &plan)...)
	if resp.Diagnostics.HasError() {
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
	model.LastResyncID = types.StringValue("")

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
	} else {
		ruleModel.Tags = types.MapNull(types.StringType)
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
	// Extract bucket name from TargetBucket (format: "bucket-name" or "arn:...")
	// The ARN in rule.Destination.Bucket is used internally, but we want the actual bucket name
	parts := strings.SplitN(targetTarget.TargetBucket, "/", 4)
	if len(parts) >= 1 {
		targetModel.Bucket = types.StringValue(parts[0])
	}

	if rule.Destination.StorageClass != "" {
		targetModel.StorageClass = types.StringValue(rule.Destination.StorageClass)
	}

	// Set host from endpoint unconditionally
	targetModel.Host = types.StringValue(targetTarget.Endpoint)

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

	if targetTarget.Region != "" {
		targetModel.Region = types.StringValue(targetTarget.Region)
	}
	targetModel.AccessKey = types.StringValue(targetTarget.Credentials.AccessKey)

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
		"secret_key":          types.StringNull(), // MinIO API never returns the secret; ModifyPlan/Read restore from state
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

	// Set remote targets first and collect ARNs
	arns := make([]string, len(ruleModels))
	targets := make([]madmin.BucketTarget, len(ruleModels))
	for i, ruleModel := range ruleModels {
		_, target, err := r.expandReplicationRule(ctx, &ruleModel, i)
		if err != nil {
			return fmt.Errorf("failed to expand rule %d: %w", i, err)
		}
		targets[i] = target

		arn, err := r.setRemoteTarget(ctx, admClient, bucket, &target)
		if err != nil {
			return fmt.Errorf("failed to set remote target for rule %d: %w", i, err)
		}
		arns[i] = arn
	}

	// Now build rules with ARNs
	var rules []replication.Rule
	for i, ruleModel := range ruleModels {
		rule, _, err := r.expandReplicationRule(ctx, &ruleModel, i)
		if err != nil {
			return fmt.Errorf("failed to expand rule %d: %w", i, err)
		}
		// Use the ARN from the remote target as the destination bucket
		rule.Destination.Bucket = arns[i]
		rules = append(rules, rule)
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
		} else {
			rule.SourceSelectionCriteria.ReplicaModifications.Status = "Disabled"
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

			err := s3utils.CheckValidBucketName(t.Bucket.ValueString())
			if err != nil {
				log.Printf("[WARN] Invalid bucket name for %q: %v", t.Bucket.ValueString(), err)
				return replication.Rule{}, madmin.BucketTarget{}, fmt.Errorf("invalid bucket name %q: %w", t.Bucket.ValueString(), err)
			}

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

func (r *bucketReplicationResource) setRemoteTarget(ctx context.Context, admClient *madmin.AdminClient, bucket string, target *madmin.BucketTarget) (string, error) {
	if target.TargetBucket == "" {
		return "", nil
	}

	existingTargets, err := admClient.ListRemoteTargets(ctx, bucket, "")
	if err != nil {
		return "", fmt.Errorf("failed to list remote targets: %w", err)
	}

	for _, existing := range existingTargets {
		if existing.Endpoint == target.Endpoint && existing.TargetBucket == target.TargetBucket {
			// Return existing ARN if target already exists
			return existing.Arn, nil
		}
	}

	arn, err := admClient.SetRemoteTarget(ctx, bucket, target)
	if err != nil {
		return "", fmt.Errorf("failed to set remote target: %w", err)
	}

	return arn, nil
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

	// Verify remote targets were actually removed, retrying while MinIO still reports them.
	// Remote target removal is eventually consistent.
	for attempt := 0; attempt < 20; attempt++ {
		remainingTargets, err := admClient.ListRemoteTargets(ctx, bucket, "")
		if err != nil {
			// If listing fails, assume targets are gone
			break
		}
		if len(remainingTargets) == 0 {
			// All targets successfully removed
			break
		}
		if attempt == 19 {
			return fmt.Errorf("remote targets still exist after removal: %d targets remaining", len(remainingTargets))
		}
		time.Sleep(time.Second)
	}

	return nil
}
