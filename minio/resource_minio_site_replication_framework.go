package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwp "github.com/hashicorp/terraform-plugin-framework/path"
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
	_ resource.Resource                = &siteReplicationResource{}
	_ resource.ResourceWithConfigure   = &siteReplicationResource{}
	_ resource.ResourceWithImportState = &siteReplicationResource{}
)

type siteReplicationResource struct {
	client *S3MinioClient
}

type siteReplicationResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Sites              types.List   `tfsdk:"site"`
	ReplicateILMExpiry types.Bool   `tfsdk:"replicate_ilm_expiry"`
	Enabled            types.Bool   `tfsdk:"enabled"`
}

type siteModel struct {
	Name               types.String `tfsdk:"name"`
	Endpoint           types.String `tfsdk:"endpoint"`
	AccessKey          types.String `tfsdk:"access_key"`
	SecretKey          types.String `tfsdk:"secret_key"`
	SecretKeyWO        types.String `tfsdk:"secret_key_wo"`
	SecretKeyWOVersion types.Int64  `tfsdk:"secret_key_wo_version"`
}

var siteObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":                  types.StringType,
		"endpoint":              types.StringType,
		"access_key":            types.StringType,
		"secret_key":            types.StringType,
		"secret_key_wo":         types.StringType,
		"secret_key_wo_version": types.Int64Type,
	},
}

func newSiteReplicationResource() resource.Resource {
	return &siteReplicationResource{}
}

func (r *siteReplicationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_replication"
}

func (r *siteReplicationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages MinIO site replication configuration for multi-site object storage synchronization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Site replication configuration name (used as resource ID)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the site replication configuration",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"site": schema.ListAttribute{
				Description: "List of sites to replicate between (minimum 2). Access_key and secret_key are stored in state but not returned by the MinIO API during read operations for security reasons.",
				Required:    true,
				ElementType: siteObjectType,
			},
			"replicate_ilm_expiry": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Replicate ILM expiration rules across sites.",
			},
			"enabled": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether site replication is enabled",
			},
		},
	}
}

func (r *siteReplicationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *siteReplicationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan siteReplicationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating site replication configuration", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	if err := r.createSiteReplication(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error creating site replication configuration",
			err.Error(),
		)
		return
	}

	if diags := r.readSiteReplication(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *siteReplicationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state siteReplicationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading site replication configuration", map[string]interface{}{
		"name": state.Name.ValueString(),
	})

	if diags := r.readSiteReplication(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *siteReplicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state siteReplicationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting site replication configuration", map[string]interface{}{
		"name": state.Name.ValueString(),
	})

	if err := r.deleteSiteReplication(ctx, &state); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting site replication configuration",
			err.Error(),
		)
		return
	}
}

func (r *siteReplicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, fwp.Root("name"), req, resp)
}

func isSiteReplicationError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "site replication not configured") ||
		strings.Contains(errStr, "Global deployment ID mismatch") ||
		strings.Contains(errStr, "Unable to fetch server info")
}

func (r *siteReplicationResource) readSiteReplication(ctx context.Context, model *siteReplicationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	client := r.client.S3Admin

	info, err := client.SiteReplicationInfo(ctx)
	if err != nil {
		if isSiteReplicationError(err) {
			tflog.Warn(ctx, "Site replication not configured or disabled, removing from state")
			model.ID = types.StringNull()
			model.Name = types.StringNull()
			model.Sites = types.ListNull(siteObjectType)
			model.ReplicateILMExpiry = types.BoolNull()
			model.Enabled = types.BoolNull()
			return diags
		}
		diags.AddError("Reading site replication configuration", fmt.Sprintf("Failed to read site replication config: %s", err))
		return diags
	}

	if !info.Enabled {
		tflog.Warn(ctx, "Site replication not enabled, removing from state")
		model.ID = types.StringNull()
		model.Name = types.StringNull()
		model.Sites = types.ListNull(siteObjectType)
		model.ReplicateILMExpiry = types.BoolNull()
		model.Enabled = types.BoolNull()
		return diags
	}

	model.Enabled = types.BoolValue(info.Enabled)

	sites, d := r.flattenSitesWithState(ctx, model, info.Sites)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	model.Sites = sites

	if model.Name.IsNull() {
		model.Name = types.StringValue(model.ID.ValueString())
	}

	return diags
}

func (r *siteReplicationResource) flattenSitesWithState(ctx context.Context, model *siteReplicationResourceModel, sites []madmin.PeerInfo) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	result := make([]attr.Value, len(sites))

	var existingSites []siteModel
	if !model.Sites.IsNull() && !model.Sites.IsUnknown() {
		diags.Append(model.Sites.ElementsAs(ctx, &existingSites, false)...)
		if diags.HasError() {
			return types.ListNull(siteObjectType), diags
		}
	}

	for i, site := range sites {
		siteMap := map[string]attr.Value{
			"name":                  types.StringValue(site.Name),
			"endpoint":              types.StringValue(site.Endpoint),
			"access_key":            types.StringNull(),
			"secret_key":            types.StringNull(),
			"secret_key_wo":         types.StringNull(),
			"secret_key_wo_version": types.Int64Null(),
		}

		if i < len(existingSites) {
			existingSite := existingSites[i]
			if !existingSite.AccessKey.IsNull() && existingSite.AccessKey.ValueString() != "" {
				siteMap["access_key"] = existingSite.AccessKey
			}
			if !existingSite.SecretKey.IsNull() && existingSite.SecretKey.ValueString() != "" {
				siteMap["secret_key"] = existingSite.SecretKey
			}
			if !existingSite.SecretKeyWO.IsNull() && existingSite.SecretKeyWO.ValueString() != "" {
				siteMap["secret_key_wo"] = existingSite.SecretKeyWO
			}
			if !existingSite.SecretKeyWOVersion.IsNull() {
				siteMap["secret_key_wo_version"] = existingSite.SecretKeyWOVersion
			}
		}

		obj, d := types.ObjectValue(siteObjectType.AttrTypes, siteMap)
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(siteObjectType), diags
		}
		result[i] = obj
	}

	return types.ListValue(siteObjectType, result)
}

func (r *siteReplicationResource) createSiteReplication(ctx context.Context, model *siteReplicationResourceModel) error {
	client := r.client.S3Admin
	name := model.Name.ValueString()

	sites, err := r.expandSites(ctx, model)
	if err != nil {
		return fmt.Errorf("expanding site replication sites: %w", err)
	}

	tflog.Debug(ctx, "Creating site replication", map[string]interface{}{
		"name":  name,
		"sites": len(sites),
	})

	opts := madmin.SRAddOptions{
		ReplicateILMExpiry: model.ReplicateILMExpiry.ValueBool(),
	}

	_, err = client.SiteReplicationAdd(ctx, sites, opts)
	if err != nil {
		return fmt.Errorf("creating site replication: %w", err)
	}

	model.ID = types.StringValue(name)

	return nil
}

func (r *siteReplicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan siteReplicationResourceModel
	var state siteReplicationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating site replication configuration", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	if err := r.updateSiteReplication(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError(
			"Error updating site replication configuration",
			err.Error(),
		)
		return
	}

	if diags := r.readSiteReplication(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *siteReplicationResource) updateSiteReplication(ctx context.Context, plan *siteReplicationResourceModel, state *siteReplicationResourceModel) error {
	client := r.client.S3Admin

	info, err := client.SiteReplicationInfo(ctx)
	if err != nil {
		if isSiteReplicationError(err) {
			return fmt.Errorf("site replication not configured: %w", err)
		}
		return fmt.Errorf("reading site replication for update: %w", err)
	}

	if plan.ReplicateILMExpiry.ValueBool() != state.ReplicateILMExpiry.ValueBool() {
		editOpts := madmin.SREditOptions{
			EnableILMExpiryReplication:  plan.ReplicateILMExpiry.ValueBool(),
			DisableILMExpiryReplication: !plan.ReplicateILMExpiry.ValueBool(),
		}

		if len(info.Sites) > 0 {
			peer := madmin.PeerInfo{
				Endpoint:     info.Sites[0].Endpoint,
				Name:         info.Sites[0].Name,
				DeploymentID: info.Sites[0].DeploymentID,
			}

			_, err := client.SiteReplicationEdit(ctx, peer, editOpts)
			if err != nil {
				return fmt.Errorf("updating ILM expiry replication: %w", err)
			}
		}
	}

	var oldSites, newSites []siteModel
	diags := state.Sites.ElementsAs(ctx, &oldSites, false)
	if diags.HasError() {
		return fmt.Errorf("parsing old sites: %v", diags)
	}

	diags = plan.Sites.ElementsAs(ctx, &newSites, false)
	if diags.HasError() {
		return fmt.Errorf("parsing new sites: %v", diags)
	}

	diff := r.calculateSiteDiff(oldSites, newSites)

	if len(diff.toRemove) > 0 {
		tflog.Debug(ctx, "Removing sites from replication", map[string]interface{}{
			"count": len(diff.toRemove),
			"sites": diff.toRemove,
		})

		_, err := client.SiteReplicationRemove(ctx, madmin.SRRemoveReq{
			SiteNames: diff.toRemove,
		})
		if err != nil {
			return fmt.Errorf("removing sites from replication: %w", err)
		}
	}

	if len(diff.toAdd) > 0 {
		tflog.Debug(ctx, "Adding sites to replication", map[string]interface{}{
			"count": len(diff.toAdd),
		})

		opts := madmin.SRAddOptions{}
		_, err := client.SiteReplicationAdd(ctx, diff.toAdd, opts)
		if err != nil {
			return fmt.Errorf("adding sites to replication: %w", err)
		}
	}

	return nil
}

type siteDiff struct {
	toAdd    []madmin.PeerSite
	toRemove []string
}

func (r *siteReplicationResource) calculateSiteDiff(oldSites, newSites []siteModel) *siteDiff {
	diff := &siteDiff{
		toAdd:    make([]madmin.PeerSite, 0),
		toRemove: make([]string, 0),
	}

	oldSiteMap := make(map[string]siteModel)
	newSiteMap := make(map[string]siteModel)

	for _, site := range oldSites {
		oldSiteMap[site.Name.ValueString()] = site
	}
	for _, site := range newSites {
		newSiteMap[site.Name.ValueString()] = site
	}

	for siteName := range oldSiteMap {
		if _, exists := newSiteMap[siteName]; !exists {
			diff.toRemove = append(diff.toRemove, siteName)
		}
	}

	for siteName, newSite := range newSiteMap {
		if oldSite, exists := oldSiteMap[siteName]; !exists {
			diff.toAdd = append(diff.toAdd, r.siteModelToPeerSite(newSite))
		} else if r.sitesDiffer(oldSite, newSite) {
			diff.toRemove = append(diff.toRemove, siteName)
			diff.toAdd = append(diff.toAdd, r.siteModelToPeerSite(newSite))
		}
	}

	return diff
}

func (r *siteReplicationResource) sitesDiffer(oldSite, newSite siteModel) bool {
	return oldSite.Endpoint.ValueString() != newSite.Endpoint.ValueString() ||
		oldSite.AccessKey.ValueString() != newSite.AccessKey.ValueString() ||
		oldSite.SecretKey.ValueString() != newSite.SecretKey.ValueString()
}

func (r *siteReplicationResource) siteModelToPeerSite(site siteModel) madmin.PeerSite {
	return madmin.PeerSite{
		Name:      site.Name.ValueString(),
		Endpoint:  site.Endpoint.ValueString(),
		AccessKey: site.AccessKey.ValueString(),
		SecretKey: site.SecretKey.ValueString(),
	}
}

func (r *siteReplicationResource) expandSites(ctx context.Context, model *siteReplicationResourceModel) ([]madmin.PeerSite, error) {
	var sites []siteModel
	diags := model.Sites.ElementsAs(ctx, &sites, false)
	if diags.HasError() {
		return nil, fmt.Errorf("parsing sites: %v", diags)
	}

	result := make([]madmin.PeerSite, len(sites))
	for i, site := range sites {
		result[i] = madmin.PeerSite{
			Name:      site.Name.ValueString(),
			Endpoint:  site.Endpoint.ValueString(),
			AccessKey: site.AccessKey.ValueString(),
			SecretKey: site.SecretKey.ValueString(),
		}
	}

	return result, nil
}

func (r *siteReplicationResource) deleteSiteReplication(ctx context.Context, model *siteReplicationResourceModel) error {
	client := r.client.S3Admin

	var sites []siteModel
	diags := model.Sites.ElementsAs(ctx, &sites, false)
	if diags.HasError() {
		return fmt.Errorf("parsing sites: %v", diags)
	}

	siteNames := make([]string, len(sites))
	for i, site := range sites {
		siteNames[i] = site.Name.ValueString()
	}

	tflog.Debug(ctx, "Deleting site replication", map[string]interface{}{
		"sites": siteNames,
	})

	_, err := client.SiteReplicationRemove(ctx, madmin.SRRemoveReq{
		SiteNames: siteNames,
		RemoveAll: true,
	})
	if err != nil {
		if isSiteReplicationError(err) {
			tflog.Info(ctx, "Site replication already removed or not configured")
			return nil
		}
		return fmt.Errorf("deleting site replication: %w", err)
	}

	return nil
}
