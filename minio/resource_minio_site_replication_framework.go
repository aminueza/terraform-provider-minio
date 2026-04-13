package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
	client *madmin.AdminClient
}

type siteReplicationResourceModel struct {
	Name               types.String               `tfsdk:"name"`
	Sites              []siteReplicationSiteModel `tfsdk:"site"`
	ReplicateILMExpiry types.Bool                 `tfsdk:"replicate_ilm_expiry"`
	Enabled            types.Bool                 `tfsdk:"enabled"`
}

type siteReplicationSiteModel struct {
	Name               types.String `tfsdk:"name"`
	Endpoint           types.String `tfsdk:"endpoint"`
	AccessKey          types.String `tfsdk:"access_key"`
	SecretKey          types.String `tfsdk:"secret_key"`
	SecretKeyWO        types.String `tfsdk:"secret_key_wo"`
	SecretKeyWOVersion types.Int64  `tfsdk:"secret_key_wo_version"`
}

type siteDiff struct {
	toAdd    []madmin.PeerSite
	toRemove []string
}

func newSiteReplicationResource() resource.Resource {
	return &siteReplicationResource{}
}

func (r *siteReplicationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_replication"
}

func (r *siteReplicationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Expected *S3MinioClient")
		return
	}
	r.client = client.S3Admin
}

func (r *siteReplicationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages MinIO site replication configuration. This resource allows you to configure multi-site replication between MinIO clusters.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "Name of the site replication configuration",
			},
			"site": schema.ListNestedAttribute{
				Required: true,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(2),
				},
				Description: "List of sites to replicate between (minimum 2). Access_key and secret_key are stored in state but not returned by the MinIO API during read operations for security reasons.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Unique name for this site",
						},
						"endpoint": schema.StringAttribute{
							Required:    true,
							Description: "MinIO server endpoint URL",
						},
						"access_key": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Access key for the site. Stored in Terraform state but not returned by the MinIO API for security reasons.",
						},
						"secret_key": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Sensitive:   true,
							Description: "Secret key for the site. Stored in Terraform state but not returned by the MinIO API for security reasons.",
						},
						"secret_key_wo": schema.StringAttribute{
							Optional:    true,
							Sensitive:   true,
							WriteOnly:   true,
							Description: "Write-only secret key for the site.",
						},
						"secret_key_wo_version": schema.Int64Attribute{
							Optional: true,
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
							},
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
							Description: "Version identifier for write-only secret key. Change this value to trigger updates when using secret_key_wo.",
						},
					},
				},
			},
			"replicate_ilm_expiry": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Replicate ILM expiration rules across sites.",
			},
			"enabled": schema.BoolAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
				Description: "Whether site replication is enabled",
			},
		},
	}
}

func (r *siteReplicationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan siteReplicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sites, diags := r.expandSitesWithWriteOnly(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Creating site replication: %s with %d sites", plan.Name.ValueString(), len(sites)))

	opts := madmin.SRAddOptions{
		ReplicateILMExpiry: plan.ReplicateILMExpiry.ValueBool(),
	}
	status, err := r.client.SiteReplicationAdd(ctx, sites, opts)
	if err != nil {
		resp.Diagnostics.AddError("Creating site replication", err.Error())
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Site replication created: %+v", status))

	plan.Enabled = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *siteReplicationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state siteReplicationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	info, err := r.client.SiteReplicationInfo(ctx)
	if err != nil {
		if r.isSiteReplicationError(err) {
			tflog.Warn(ctx, fmt.Sprintf("Site replication not configured or disabled, removing from state: %v", err))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading site replication", err.Error())
		return
	}

	if !info.Enabled {
		tflog.Warn(ctx, "Site replication not enabled, removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	state.Enabled = types.BoolValue(info.Enabled)
	state.Sites = r.flattenSitesWithState(ctx, &state, info.Sites)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *siteReplicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan siteReplicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state siteReplicationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	oldSites := r.expandSites(ctx, state.Sites)
	newSites, diags := r.expandSitesWithWriteOnly(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diff := r.calculateSiteDiff(ctx, oldSites, newSites)

	if len(diff.toRemove) > 0 {
		tflog.Info(ctx, fmt.Sprintf("Removing %d sites from replication: %v", len(diff.toRemove), diff.toRemove))
		_, err := r.client.SiteReplicationRemove(ctx, madmin.SRRemoveReq{
			SiteNames: diff.toRemove,
		})
		if err != nil {
			resp.Diagnostics.AddError("Removing sites from replication", err.Error())
			return
		}
	}

	if len(diff.toAdd) > 0 {
		tflog.Info(ctx, fmt.Sprintf("Adding %d sites to replication", len(diff.toAdd)))
		_, err := r.client.SiteReplicationAdd(ctx, diff.toAdd, madmin.SRAddOptions{})
		if err != nil {
			resp.Diagnostics.AddError("Adding sites to replication", err.Error())
			return
		}
	}

	plan.Enabled = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *siteReplicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state siteReplicationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sites := r.expandSites(ctx, state.Sites)
	siteNames := make([]string, len(sites))
	for i, site := range sites {
		siteNames[i] = site.Name
	}

	tflog.Info(ctx, fmt.Sprintf("Deleting site replication with %d sites: %v", len(siteNames), siteNames))

	_, err := r.client.SiteReplicationRemove(ctx, madmin.SRRemoveReq{
		SiteNames: siteNames,
		RemoveAll: true,
	})
	if err != nil {
		if r.isSiteReplicationError(err) {
			tflog.Info(ctx, "Site replication already removed or not configured")
			return
		}
		resp.Diagnostics.AddError("Deleting site replication", err.Error())
		return
	}
}

func (r *siteReplicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

func (r *siteReplicationResource) isSiteReplicationError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "site replication not configured") ||
		strings.Contains(errStr, "Global deployment ID mismatch") ||
		strings.Contains(errStr, "Unable to fetch server info")
}

func (r *siteReplicationResource) expandSites(ctx context.Context, sites []siteReplicationSiteModel) []madmin.PeerSite {
	result := make([]madmin.PeerSite, len(sites))
	for i, s := range sites {
		result[i] = madmin.PeerSite{
			Name:      s.Name.ValueString(),
			Endpoint:  s.Endpoint.ValueString(),
			AccessKey: s.AccessKey.ValueString(),
			SecretKey: s.SecretKey.ValueString(),
		}
	}
	return result
}

func (r *siteReplicationResource) expandSitesWithWriteOnly(ctx context.Context, plan *siteReplicationResourceModel) ([]madmin.PeerSite, diag.Diagnostics) {
	var diags diag.Diagnostics
	result := make([]madmin.PeerSite, len(plan.Sites))

	for i, s := range plan.Sites {
		secretKey := s.SecretKey.ValueString()

		if !s.SecretKeyWO.IsNull() && !s.SecretKeyWO.IsUnknown() {
			secretKey = s.SecretKeyWO.ValueString()
		}

		if !s.SecretKeyWOVersion.IsNull() && !s.SecretKeyWOVersion.IsUnknown() && s.SecretKeyWOVersion.ValueInt64() > 0 {
			if s.SecretKeyWO.IsNull() || s.SecretKeyWO.IsUnknown() {
				diags.AddError(
					fmt.Sprintf("Site %d requires secret_key_wo when secret_key_wo_version is set", i),
					"secret_key_wo must be provided",
				)
				continue
			}
		}

		result[i] = madmin.PeerSite{
			Name:      s.Name.ValueString(),
			Endpoint:  s.Endpoint.ValueString(),
			AccessKey: s.AccessKey.ValueString(),
			SecretKey: secretKey,
		}
	}

	return result, diags
}

func (r *siteReplicationResource) flattenSitesWithState(ctx context.Context, state *siteReplicationResourceModel, sites []madmin.PeerInfo) []siteReplicationSiteModel {
	result := make([]siteReplicationSiteModel, len(sites))

	for i, site := range sites {
		siteModel := siteReplicationSiteModel{
			Name:     types.StringValue(site.Name),
			Endpoint: types.StringValue(site.Endpoint),
		}

		if i < len(state.Sites) {
			if state.Sites[i].SecretKeyWOVersion.ValueInt64() == 0 {
				siteModel.SecretKey = state.Sites[i].SecretKey
			}
			siteModel.SecretKeyWOVersion = state.Sites[i].SecretKeyWOVersion
			if !state.Sites[i].AccessKey.IsNull() {
				siteModel.AccessKey = state.Sites[i].AccessKey
			}
		}

		result[i] = siteModel
	}
	return result
}

func (r *siteReplicationResource) calculateSiteDiff(ctx context.Context, oldSites, newSites []madmin.PeerSite) *siteDiff {
	diff := &siteDiff{
		toAdd:    make([]madmin.PeerSite, 0),
		toRemove: make([]string, 0),
	}

	oldSiteMap := make(map[string]madmin.PeerSite)
	newSiteMap := make(map[string]madmin.PeerSite)

	for _, site := range oldSites {
		oldSiteMap[site.Name] = site
	}
	for _, site := range newSites {
		newSiteMap[site.Name] = site
	}

	for siteName := range oldSiteMap {
		if _, exists := newSiteMap[siteName]; !exists {
			diff.toRemove = append(diff.toRemove, siteName)
		}
	}

	for siteName, newSite := range newSiteMap {
		if oldSite, exists := oldSiteMap[siteName]; !exists {
			diff.toAdd = append(diff.toAdd, newSite)
		} else if r.sitesDiffer(ctx, oldSite, newSite) {
			diff.toRemove = append(diff.toRemove, siteName)
			diff.toAdd = append(diff.toAdd, newSite)
		}
	}

	return diff
}

func (r *siteReplicationResource) sitesDiffer(ctx context.Context, oldSite, newSite madmin.PeerSite) bool {
	return oldSite.Endpoint != newSite.Endpoint ||
		oldSite.AccessKey != newSite.AccessKey ||
		oldSite.SecretKey != newSite.SecretKey
}
