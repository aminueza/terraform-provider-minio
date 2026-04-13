package minio

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwp "github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/replication"
	"github.com/minio/minio-go/v7/pkg/s3utils"
	"github.com/rs/xid"
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
	ID                        types.String             `tfsdk:"id"`
	Arn                       types.String             `tfsdk:"arn"`
	Enabled                   types.Bool               `tfsdk:"enabled"`
	Priority                  types.Int64              `tfsdk:"priority"`
	Prefix                    types.String             `tfsdk:"prefix"`
	Tags                      types.Map                `tfsdk:"tags"`
	DeleteReplication         types.Bool               `tfsdk:"delete_replication"`
	DeleteMarkerReplication   types.Bool               `tfsdk:"delete_marker_replication"`
	ExistingObjectReplication types.Bool               `tfsdk:"existing_object_replication"`
	MetadataSync              types.Bool               `tfsdk:"metadata_sync"`
	Target                    []replicationTargetModel `tfsdk:"target"`
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

func newBucketReplicationResource() func() resource.Resource {
	return func() resource.Resource {
		return &bucketReplicationResource{}
	}
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
				Default:     int64default.StaticInt64(0),
				Description: "Increment this value to trigger a resync of existing objects for all replication rules. Each increment triggers one resync.",
			},
			"last_resync_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the last resync operation.",
			},
			"rule": schema.ListNestedAttribute{
				Description: "Rule definitions",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Rule ID generated by MinIO",
						},
						"arn": schema.StringAttribute{
							Computed:    true,
							Description: "Rule ARN generated by MinIO",
						},
						"enabled": schema.BoolAttribute{
							Optional:    true,
							Default:     booldefault.StaticBool(true),
							Description: "Whether or not this rule is enabled",
						},
						"priority": schema.Int64Attribute{
							Optional: true,
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
							},
							Description: "Rule priority. If omitted, the inverted index will be used as priority.",
						},
						"prefix": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString(""),
							Description: "Bucket prefix object must be in to be synchronized",
						},
						"tags": schema.MapAttribute{
							ElementType: types.StringType,
							Optional:    true,
							Description: "Tags which objects must have to be synchronized",
						},
						"delete_replication": schema.BoolAttribute{
							Optional:    true,
							Description: "Whether or not to propagate deletion",
						},
						"delete_marker_replication": schema.BoolAttribute{
							Optional:    true,
							Description: "Whether or not to synchronize marker deletion",
						},
						"existing_object_replication": schema.BoolAttribute{
							Optional:    true,
							Description: "Whether or not to synchronize objects created prior to the replication configuration",
						},
						"metadata_sync": schema.BoolAttribute{
							Optional:    true,
							Description: "Whether or not to synchronize buckets and objects metadata (such as locks). This must be enabled to achieve two-way replication",
						},
						"target": schema.ListNestedAttribute{
							Description: "Target bucket configuration",
							Required:    true,
							Validators: []validator.List{
								listvalidator.SizeBetween(1, 1),
							},
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"bucket": schema.StringAttribute{
										Required:    true,
										Description: "The name of the existing target bucket to replicate into",
									},
									"storage_class": schema.StringAttribute{
										Optional:    true,
										Description: "The storage class to use for the object on this target",
									},
									"host": schema.StringAttribute{
										Required:    true,
										Description: "The target host. This host must be reachable by the MinIO instance itself",
									},
									"secure": schema.BoolAttribute{
										Optional:    true,
										Default:     booldefault.StaticBool(true),
										Description: "Whether to use HTTPS with this target",
									},
									"path_style": schema.StringAttribute{
										Optional: true,
										Default:  stringdefault.StaticString("auto"),
										Validators: []validator.String{
											stringvalidator.OneOf("on", "off", "auto"),
										},
										Description: "Whether to use path-style or virtual-hosted-style requests. `auto` allows MinIO to choose automatically",
									},
									"path": schema.StringAttribute{
										Optional:    true,
										Description: "Path of the MinIO endpoint. Useful if MinIO API isn't served at the root",
									},
									"synchronous": schema.BoolAttribute{
										Optional:    true,
										Computed:    true,
										Description: "Use synchronous replication",
									},
									"disable_proxy": schema.BoolAttribute{
										Optional:    true,
										Default:     booldefault.StaticBool(false),
										Description: "Disable proxy for this target",
									},
									"health_check_period": schema.StringAttribute{
										Optional: true,
										Default:  stringdefault.StaticString("30s"),
										Validators: []validator.String{
											stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9]+\s?[s|m|h]$`), "must be a valid golang duration"),
										},
										Description: "Period where the health of this target will be checked. Must be a valid duration, such as `5s` or `2m`",
									},
									"bandwidth_limit": schema.StringAttribute{
										Optional:    true,
										Default:     stringdefault.StaticString("0"),
										Description: "Maximum bandwidth in bytes per second that MinIO can use when synchronizing this target. Minimum is 100MB",
									},
									"region": schema.StringAttribute{
										Optional:    true,
										Description: "Region of the target MinIO. This will be used to generate the target ARN",
									},
									"access_key": schema.StringAttribute{
										Required: true,
										Validators: []validator.String{
											stringvalidator.LengthAtLeast(1),
										},
										Description: "Access key for the replication service account in the target MinIO",
									},
									"secret_key": schema.StringAttribute{
										Optional:  true,
										Sensitive: true,
										Validators: []validator.String{
											stringvalidator.LengthAtLeast(1),
										},
										Description: "Secret key for the replication service account in the target MinIO. This is write-only and cannot be read from the API",
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

	if err := r.applyReplication(ctx, &plan, false); err != nil {
		resp.Diagnostics.AddError(
			"Error creating bucket replication configuration",
			err.Error(),
		)
		return
	}

	// Read back the configuration to get computed values
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

	if err := r.applyReplication(ctx, &plan, true); err != nil {
		resp.Diagnostics.AddError(
			"Error updating bucket replication configuration",
			err.Error(),
		)
		return
	}

	// Read back the configuration to get computed values
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

	ctx = context.Background()

	rcfg, err := r.client.S3Client.GetBucketReplication(ctx, state.Bucket.ValueString())
	if err != nil {
		if isNoSuchBucketError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error reading bucket replication configuration for deletion",
			err.Error(),
		)
		return
	}

	rcfg.Rules = []replication.Rule{}
	err = r.client.S3Client.SetBucketReplication(ctx, state.Bucket.ValueString(), rcfg)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error disabling bucket replication",
			err.Error(),
		)
		return
	}
}

func (r *bucketReplicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, fwp.Root("bucket"), req, resp)
}

func (r *bucketReplicationResource) applyReplication(ctx context.Context, plan *bucketReplicationResourceModel, isUpdate bool) error {
	bucket := plan.Bucket.ValueString()

	// Get existing replication config
	rcfg, err := r.client.S3Client.GetBucketReplication(ctx, bucket)
	if err != nil {
		// If bucket doesn't exist or no config, start with empty config
		rcfg = replication.Config{}
	}

	// Expand rules from plan
	var rules []S3MinioBucketReplicationRule
	var diags diag.Diagnostics

	if !plan.Rules.IsNull() && !plan.Rules.IsUnknown() {
		var rulesList []replicationRuleModel
		diags.Append(plan.Rules.ElementsAs(ctx, &rulesList, false)...)
		if diags.HasError() {
			return fmt.Errorf("error reading rules: %v", diags.Errors())
		}

		rules = expandReplicationRules(rulesList)
	}

	// Track used ARNs
	usedARNs := make([]string, len(rules))

	// List existing remote targets
	existingRemoteTargets, err := r.client.S3Admin.ListRemoteTargets(ctx, bucket, "")
	if err != nil {
		return fmt.Errorf("error listing remote targets: %w", err)
	}

	// Process each rule
	for i, rule := range rules {
		// Validate bucket name
		if err := s3utils.CheckValidBucketName(rule.Target.Bucket); err != nil {
			return fmt.Errorf("invalid target bucket name %q: %w", rule.Target.Bucket, err)
		}

		// Build target path
		tgtBucket := rule.Target.Bucket
		if rule.Target.Path != "" {
			tgtBucket = path.Clean("./" + rule.Target.Path + "/" + tgtBucket)
		}

		// Create credentials
		creds := &madmin.Credentials{
			AccessKey: rule.Target.AccessKey,
			SecretKey: rule.Target.SecretKey,
		}

		// Build bucket target
		bktTarget := &madmin.BucketTarget{
			TargetBucket:        tgtBucket,
			Secure:              rule.Target.Secure,
			Credentials:         creds,
			Endpoint:            rule.Target.Host,
			Path:                rule.Target.PathStyle.String(),
			API:                 "s3v4",
			Type:                madmin.ReplicationService,
			Region:              rule.Target.Region,
			BandwidthLimit:      rule.Target.BandwidthLimit,
			ReplicationSync:     rule.Target.Synchronous,
			DisableProxy:        rule.Target.DisableProxy,
			HealthCheckDuration: rule.Target.HealthCheckPeriod,
		}

		// Find existing remote target
		var existingRemoteTarget *madmin.BucketTarget
		if rule.Arn != "" {
			for j, target := range existingRemoteTargets {
				if target.Arn == rule.Arn {
					existingRemoteTarget = &existingRemoteTargets[j]
					break
				}
			}
		}

		var arn string

		// Create or update remote target
		if existingRemoteTarget == nil || existingRemoteTarget.ReplicationSync != bktTarget.ReplicationSync {
			if existingRemoteTarget != nil {
				tflog.Debug(ctx, "Synchronous mode change detected, re-creating remote target", map[string]interface{}{
					"bucket": bucket,
					"old":    existingRemoteTarget.ReplicationSync,
					"new":    bktTarget.ReplicationSync,
				})
			} else {
				tflog.Debug(ctx, "Adding new remote target", map[string]interface{}{
					"bucket": bucket,
					"target": tgtBucket,
				})
			}

			arn, err = r.client.S3Admin.SetRemoteTarget(ctx, bucket, bktTarget)
			if err != nil {
				return fmt.Errorf("error creating remote target: %w", err)
			}
		} else {
			// Update existing target
			var remoteTargetUpdate []madmin.TargetUpdateType

			if *existingRemoteTarget.Credentials != *bktTarget.Credentials {
				existingRemoteTarget.Credentials = bktTarget.Credentials
				remoteTargetUpdate = append(remoteTargetUpdate, madmin.CredentialsUpdateType)
			}
			if existingRemoteTarget.ReplicationSync != bktTarget.ReplicationSync {
				existingRemoteTarget.ReplicationSync = bktTarget.ReplicationSync
				remoteTargetUpdate = append(remoteTargetUpdate, madmin.SyncUpdateType)
			}
			if existingRemoteTarget.DisableProxy != bktTarget.DisableProxy {
				existingRemoteTarget.DisableProxy = bktTarget.DisableProxy
				remoteTargetUpdate = append(remoteTargetUpdate, madmin.ProxyUpdateType)
			}
			if existingRemoteTarget.BandwidthLimit != bktTarget.BandwidthLimit {
				existingRemoteTarget.BandwidthLimit = bktTarget.BandwidthLimit
				remoteTargetUpdate = append(remoteTargetUpdate, madmin.BandwidthLimitUpdateType)
			}
			if existingRemoteTarget.HealthCheckDuration != bktTarget.HealthCheckDuration {
				existingRemoteTarget.HealthCheckDuration = bktTarget.HealthCheckDuration
				remoteTargetUpdate = append(remoteTargetUpdate, madmin.HealthCheckDurationUpdateType)
			}
			if existingRemoteTarget.Path != bktTarget.Path {
				existingRemoteTarget.Path = bktTarget.Path
				remoteTargetUpdate = append(remoteTargetUpdate, madmin.PathUpdateType)
			}

			if len(remoteTargetUpdate) > 0 {
				tflog.Debug(ctx, "Updating remote target", map[string]interface{}{
					"bucket": bucket,
					"arn":    existingRemoteTarget.Arn,
				})
				arn, err = r.client.S3Admin.UpdateRemoteTarget(ctx, existingRemoteTarget, remoteTargetUpdate...)
				if err != nil {
					return fmt.Errorf("error updating remote target: %w", err)
				}
			} else {
				arn = existingRemoteTarget.Arn
			}
		}

		// Build tags string
		tagList := []string{}
		for k, v := range rule.Tags {
			escapedValue, err := url.Parse(v)
			if err != nil {
				return fmt.Errorf("error parsing tag value %q: %w", v, err)
			}
			tagList = append(tagList, fmt.Sprintf("%s=%s", k, escapedValue.String()))
		}

		// Build replication options
		opts := replication.Options{
			TagString:               strings.Join(tagList, "&"),
			IsTagSet:                len(tagList) != 0,
			StorageClass:            rule.Target.StorageClass,
			Priority:                strconv.Itoa(int(math.Abs(float64(rule.Priority)))),
			Prefix:                  rule.Prefix,
			RuleStatus:              toEnableFlag(rule.Enabled),
			ID:                      rule.Id,
			DestBucket:              arn,
			ReplicateDeleteMarkers:  toEnableFlag(rule.DeleteMarkerReplication),
			ReplicateDeletes:        toEnableFlag(rule.DeleteReplication),
			ReplicaSync:             toEnableFlag(rule.MetadataSync),
			ExistingObjectReplicate: toEnableFlag(rule.ExistingObjectReplication),
		}

		// Add or edit rule
		if strings.TrimSpace(opts.ID) == "" {
			rule.Id = xid.New().String()
			opts.ID = rule.Id
			opts.Op = replication.AddOption
			if err := rcfg.AddRule(opts); err != nil {
				return fmt.Errorf("error adding replication rule: %w", err)
			}
		} else {
			opts.Op = replication.SetOption
			if err := rcfg.EditRule(opts); err != nil {
				return fmt.Errorf("error editing replication rule: %w", err)
			}
		}

		usedARNs[i] = arn
	}

	// Remove unused remote targets
	for _, existingRemoteTarget := range existingRemoteTargets {
		found := false
		for _, arn := range usedARNs {
			if arn == existingRemoteTarget.Arn {
				found = true
				break
			}
		}
		if !found {
			if err := r.client.S3Admin.RemoveRemoteTarget(ctx, bucket, existingRemoteTarget.Arn); err != nil {
				return fmt.Errorf("error removing remote target %q: %w", existingRemoteTarget.Arn, err)
			}
		}
	}

	// Set replication config
	if err := r.client.S3Client.SetBucketReplication(ctx, bucket, rcfg); err != nil {
		return fmt.Errorf("error setting bucket replication: %w", err)
	}

	// Handle resync version
	var lastResyncID string
	if !plan.ResyncVersion.IsNull() && !plan.ResyncVersion.IsUnknown() {
		resyncVersion := plan.ResyncVersion.ValueInt64()
		if resyncVersion > 0 {
			tflog.Debug(ctx, "Triggering replication resync", map[string]interface{}{
				"bucket":  bucket,
				"version": resyncVersion,
			})

			rcfg, err := r.client.S3Client.GetBucketReplication(ctx, bucket)
			if err != nil {
				return fmt.Errorf("error reading replication config for resync: %w", err)
			}

			for _, rule := range rcfg.Rules {
				if rule.Destination.Bucket != "" {
					info, err := r.client.S3Client.ResetBucketReplicationOnTarget(ctx, bucket, 0, rule.Destination.Bucket)
					if err != nil {
						return fmt.Errorf("error triggering replication resync: %w", err)
					}
					if len(info.Targets) > 0 {
						lastResyncID = info.Targets[0].ResetID
					}
				}
			}
		}
	}

	// Store last_resync_id in state if it changed
	if lastResyncID != "" {
		plan.LastResyncID = types.StringValue(lastResyncID)
	}

	return nil
}

func (r *bucketReplicationResource) readReplication(ctx context.Context, state *bucketReplicationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	bucket := state.Bucket.ValueString()

	// Read bucket replication config
	rcfg, err := r.client.S3Client.GetBucketReplication(ctx, bucket)
	if err != nil {
		if isNoSuchBucketError(err) {
			state.ID = types.StringNull()
			state.Bucket = types.StringNull()
			return diags
		}
		diags.AddError(
			"Error reading bucket replication configuration",
			err.Error(),
		)
		return diags
	}

	// Build reverse index for rule priorities
	rulePriorityMap := map[int]int{}
	if state.Rules.IsNull() || state.Rules.IsUnknown() {
		state.Rules = types.ListNull(types.ObjectType{})
	} else {
		var rulesList []replicationRuleModel
		diags.Append(state.Rules.ElementsAs(ctx, &rulesList, false)...)
		if diags.HasError() {
			return diags
		}

		for idx, rule := range rulesList {
			priority := int(rule.Priority.ValueInt64())
			if priority == 0 {
				priority = -len(rulesList) + idx
			}
			rulePriorityMap[priority] = idx
		}
	}

	// Build rules from replication config
	rules := make([]replicationRuleModel, len(rcfg.Rules))
	ruleArnMap := map[string]int{}

	for idx, rule := range rcfg.Rules {
		var ruleIdx int
		var ok bool
		if ruleIdx, ok = rulePriorityMap[rule.Priority]; !ok {
			ruleIdx = idx
		}
		if _, ok = ruleArnMap[rule.Destination.Bucket]; ok {
			diags.AddError(
				"Error reading replication rules",
				fmt.Sprintf("conflict detected between two rules containing the same ARN: %q", rule.Destination.Bucket),
			)
			return diags
		}
		ruleArnMap[rule.Destination.Bucket] = ruleIdx

		// Build tags map
		tags := make(map[string]types.String)
		if len(rule.Filter.And.Tags) != 0 || rule.Filter.And.Prefix != "" {
			for _, tag := range rule.Filter.And.Tags {
				if tag.IsEmpty() {
					continue
				}
				tags[tag.Key] = types.StringValue(tag.Value)
			}
		} else if rule.Filter.Tag.Key != "" {
			tags[rule.Filter.Tag.Key] = types.StringValue(rule.Filter.Tag.Value)
		}

		tagsMap, tagsDiags := types.MapValueFrom(ctx, types.StringType, tags)
		diags.Append(tagsDiags...)
		if diags.HasError() {
			return diags
		}

		// Build rule model
		rules[ruleIdx] = replicationRuleModel{
			ID:                        types.StringValue(rule.ID),
			Arn:                       types.StringValue(rule.Destination.Bucket),
			Enabled:                   types.BoolValue(rule.Status == replication.Enabled),
			Prefix:                    types.StringValue(rule.Prefix()),
			DeleteReplication:         types.BoolValue(rule.DeleteReplication.Status == replication.Enabled),
			DeleteMarkerReplication:   types.BoolValue(rule.DeleteMarkerReplication.Status == replication.Enabled),
			ExistingObjectReplication: types.BoolValue(rule.ExistingObjectReplication.Status == replication.Enabled),
			MetadataSync:              types.BoolValue(rule.SourceSelectionCriteria.ReplicaModifications.Status == replication.Enabled),
			Tags:                      tagsMap,
		}

		// Set priority (only if explicitly set in config)
		if len(rules) > ruleIdx && int64(rule.Priority) == -rules[ruleIdx].Priority.ValueInt64() {
			rules[ruleIdx].Priority = types.Int64Null()
		} else {
			rules[ruleIdx].Priority = types.Int64Value(int64(rule.Priority))
		}

		// Initialize target
		rules[ruleIdx].Target = []replicationTargetModel{{
			StorageClass: types.StringValue(rule.Destination.StorageClass),
		}}
	}

	// Read remote targets
	existingRemoteTargets, err := r.client.S3Admin.ListRemoteTargets(ctx, bucket, "")
	if err != nil {
		diags.AddError(
			"Error reading replication remote target configuration",
			err.Error(),
		)
		return diags
	}

	if len(existingRemoteTargets) != len(rules) {
		diags.AddError(
			"Error reading replication configuration",
			fmt.Sprintf("inconsistent number of remote target and bucket replication rules (%d != %d)", len(existingRemoteTargets), len(rules)),
		)
		return diags
	}

	// Merge remote target info into rules
	for _, remoteTarget := range existingRemoteTargets {
		var ruleIdx int
		var ok bool
		if ruleIdx, ok = ruleArnMap[remoteTarget.Arn]; !ok {
			diags.AddError(
				"Error reading replication configuration",
				fmt.Sprintf("unable to find the remote target configuration for ARN %q", remoteTarget.Arn),
			)
			return diags
		}

		// Parse bucket path
		pathComponent := strings.Split(remoteTarget.TargetBucket, "/")
		targetBucket := pathComponent[len(pathComponent)-1]
		targetPath := strings.Join(pathComponent[:len(pathComponent)-1], "/")

		// Preserve secret key from state
		secretKey := types.StringNull()
		if ruleIdx < len(rules) && len(rules[ruleIdx].Target) > 0 {
			if !rules[ruleIdx].Target[0].SecretKey.IsNull() {
				secretKey = rules[ruleIdx].Target[0].SecretKey
			}
		}

		// Calculate bandwidth limit
		var bwUint64 uint64
		if remoteTarget.BandwidthLimit < 0 {
			bwUint64 = 0
		} else {
			bwUint64 = uint64(remoteTarget.BandwidthLimit)
		}

		rules[ruleIdx].Target[0] = replicationTargetModel{
			Bucket:            types.StringValue(targetBucket),
			Host:              types.StringValue(remoteTarget.Endpoint),
			Secure:            types.BoolValue(remoteTarget.Secure),
			PathStyle:         types.StringValue(remoteTarget.Path),
			Path:              types.StringValue(targetPath),
			Synchronous:       types.BoolValue(remoteTarget.ReplicationSync),
			DisableProxy:      types.BoolValue(remoteTarget.DisableProxy),
			HealthCheckPeriod: types.StringValue(shortDur(remoteTarget.HealthCheckDuration)),
			BandwidthLimit:    types.StringValue(humanize.Bytes(bwUint64)),
			Region:            types.StringValue(remoteTarget.Region),
			AccessKey:         types.StringValue(remoteTarget.Credentials.AccessKey),
			SecretKey:         secretKey,
		}
	}

	// Convert rules to list
	rulesList, rulesDiags := types.ListValueFrom(ctx, types.ObjectType{}, rules)
	diags.Append(rulesDiags...)
	if diags.HasError() {
		return diags
	}

	// Update state
	state.ID = types.StringValue(bucket)
	state.Rules = rulesList

	return diags
}

func expandReplicationRules(rules []replicationRuleModel) []S3MinioBucketReplicationRule {
	result := make([]S3MinioBucketReplicationRule, len(rules))

	for i, rule := range rules {
		result[i] = S3MinioBucketReplicationRule{
			Id:                        rule.ID.ValueString(),
			Arn:                       rule.Arn.ValueString(),
			Enabled:                   rule.Enabled.ValueBool(),
			Priority:                  int(rule.Priority.ValueInt64()),
			Prefix:                    rule.Prefix.ValueString(),
			Tags:                      make(map[string]string),
			DeleteReplication:         rule.DeleteReplication.ValueBool(),
			DeleteMarkerReplication:   rule.DeleteMarkerReplication.ValueBool(),
			ExistingObjectReplication: rule.ExistingObjectReplication.ValueBool(),
			MetadataSync:              rule.MetadataSync.ValueBool(),
		}

		// Convert tags map
		if !rule.Tags.IsNull() && !rule.Tags.IsUnknown() {
			var tagsMap map[string]string
			diags := rule.Tags.ElementsAs(context.Background(), &tagsMap, false)
			if !diags.HasError() {
				result[i].Tags = tagsMap
			}
		}

		// Convert target
		if len(rule.Target) > 0 {
			target := rule.Target[0]
			result[i].Target = S3MinioBucketReplicationRuleTarget{
				Bucket:       target.Bucket.ValueString(),
				StorageClass: target.StorageClass.ValueString(),
				Host:         target.Host.ValueString(),
				Secure:       target.Secure.ValueBool(),
				Path:         target.Path.ValueString(),
				Region:       target.Region.ValueString(),
				AccessKey:    target.AccessKey.ValueString(),
				SecretKey:    target.SecretKey.ValueString(),
				Synchronous:  target.Synchronous.ValueBool(),
				DisableProxy: target.DisableProxy.ValueBool(),
			}

			// Parse path style
			switch strings.TrimSpace(strings.ToLower(target.PathStyle.ValueString())) {
			case "on":
				result[i].Target.PathStyle = S3PathStyleOn
			case "off":
				result[i].Target.PathStyle = S3PathStyleOff
			default:
				result[i].Target.PathStyle = S3PathStyleAuto
			}

			// Parse bandwidth limit
			if bw, ok, _ := ParseBandwidthLimit(map[string]any{
				"bandwidth_limit": target.BandwidthLimit.ValueString(),
			}); ok {
				if bw > uint64(math.MaxInt64) {
					result[i].Target.BandwidthLimit = math.MaxInt64
				} else {
					result[i].Target.BandwidthLimit = int64(bw)
				}
			}

			// Parse health check period
			if healthCheckPeriod, err := time.ParseDuration(target.HealthCheckPeriod.ValueString()); err == nil {
				result[i].Target.HealthCheckPeriod = healthCheckPeriod
			}
		}
	}

	return result
}
