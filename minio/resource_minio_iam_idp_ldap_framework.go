package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/minio/madmin-go/v3"
)

var (
	_ resource.Resource                = &iamIdpLdapResource{}
	_ resource.ResourceWithConfigure   = &iamIdpLdapResource{}
	_ resource.ResourceWithImportState = &iamIdpLdapResource{}
)

type iamIdpLdapResource struct {
	client *S3MinioClient
}

type iamIdpLdapResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	ServerAddr         types.String `tfsdk:"server_addr"`
	LookupBindDN       types.String `tfsdk:"lookup_bind_dn"`
	LookupBindPassword types.String `tfsdk:"lookup_bind_password"`
	UserDNSearchBaseDN types.String `tfsdk:"user_dn_search_base_dn"`
	UserDNSearchFilter types.String `tfsdk:"user_dn_search_filter"`
	GroupSearchBaseDN  types.String `tfsdk:"group_search_base_dn"`
	GroupSearchFilter  types.String `tfsdk:"group_search_filter"`
	TLSSkipVerify      types.Bool   `tfsdk:"tls_skip_verify"`
	ServerInsecure     types.Bool   `tfsdk:"server_insecure"`
	StartTLS           types.Bool   `tfsdk:"starttls"`
	Enable             types.Bool   `tfsdk:"enable"`
	RestartRequired    types.Bool   `tfsdk:"restart_required"`
}

func newIAMIdpLdapResource() resource.Resource {
	return &iamIdpLdapResource{}
}

func (r *iamIdpLdapResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_idp_ldap"
}

func (r *iamIdpLdapResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *S3MinioClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *iamIdpLdapResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an LDAP/Active Directory identity provider configuration for MinIO.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"server_addr": schema.StringAttribute{
				Required:    true,
				Description: "LDAP server address in host:port format (e.g., 'ldap.example.com:389').",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"lookup_bind_dn": schema.StringAttribute{
				Optional:    true,
				Description: "Distinguished name used to bind to the LDAP server for user/group lookups (e.g., 'cn=admin,dc=example,dc=com').",
			},
			"lookup_bind_password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Password for the lookup bind DN. MinIO does not return this value on read; Terraform keeps the value from your configuration.",
			},
			"user_dn_search_base_dn": schema.StringAttribute{
				Optional:    true,
				Description: "Base DN for searching user entries (e.g., 'ou=users,dc=example,dc=com').",
			},
			"user_dn_search_filter": schema.StringAttribute{
				Optional:    true,
				Description: "LDAP filter to locate a user by username (e.g., '(uid=%s)').",
			},
			"group_search_base_dn": schema.StringAttribute{
				Optional:    true,
				Description: "Base DN for searching group entries (e.g., 'ou=groups,dc=example,dc=com').",
			},
			"group_search_filter": schema.StringAttribute{
				Optional:    true,
				Description: "LDAP filter for group membership lookup (e.g., '(&(objectclass=groupOfNames)(member=%d))').",
			},
			"tls_skip_verify": schema.BoolAttribute{
				Optional:    true,
				Description: "Disable TLS certificate verification. Not recommended for production.",
			},
			"server_insecure": schema.BoolAttribute{
				Optional:    true,
				Description: "Allow plain (non-TLS) LDAP connections. Not recommended for production.",
			},
			"starttls": schema.BoolAttribute{
				Optional:    true,
				Description: "Use STARTTLS to upgrade a plain LDAP connection to TLS.",
			},
			"enable": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether this LDAP configuration is enabled.",
			},
			"restart_required": schema.BoolAttribute{
				Computed:    true,
				Description: "Indicates whether a MinIO server restart is required for the configuration to take effect.",
			},
		},
	}
}

func (r *iamIdpLdapResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan iamIdpLdapResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Creating LDAP IDP configuration")

	cfgData := r.buildIdpLdapCfgData(&plan)
	restart, err := r.client.S3Admin.AddOrUpdateIDPConfig(ctx, madmin.LDAPIDPCfg, madmin.Default, cfgData, false)
	if err != nil {
		resp.Diagnostics.AddError("Creating LDAP IDP configuration", fmt.Sprintf("Failed to create LDAP IDP: %s", err))
		return
	}

	plan.ID = types.StringValue("ldap")
	plan.RestartRequired = types.BoolValue(restart)

	tflog.Info(ctx, fmt.Sprintf("Created LDAP IDP configuration (restart_required=%v)", restart))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *iamIdpLdapResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state iamIdpLdapResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Reading LDAP IDP configuration")

	cfg, err := r.client.S3Admin.GetIDPConfig(ctx, madmin.LDAPIDPCfg, madmin.Default)
	if err != nil {
		if isIDPConfigNotFound(err) {
			tflog.Warn(ctx, "LDAP IDP configuration no longer exists, removing from state")
			state.ID = types.StringNull()
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
		resp.Diagnostics.AddError("Reading LDAP IDP configuration", fmt.Sprintf("Failed to read LDAP IDP: %s", err))
		return
	}

	cfgMap := idpCfgInfoToMap(cfg.Info)

	stringFields := []string{
		"server_addr",
		"lookup_bind_dn",
		"user_dn_search_base_dn",
		"user_dn_search_filter",
		"group_search_base_dn",
		"group_search_filter",
	}
	for _, field := range stringFields {
		if v, ok := cfgMap[field]; ok {
			state.SetValueForStringField(field, v)
		}
	}

	for _, field := range []string{"tls_skip_verify", "server_insecure", "starttls"} {
		val := false
		if v, ok := cfgMap[field]; ok {
			val = v == "on"
		}
		state.SetBoolValue(field, val)
	}

	enableVal := true
	if v, ok := cfgMap["enable"]; ok {
		enableVal = v == "on"
	}
	state.Enable = types.BoolValue(enableVal)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *iamIdpLdapResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan iamIdpLdapResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Updating LDAP IDP configuration")

	cfgData := r.buildIdpLdapCfgData(&plan)
	restart, err := r.client.S3Admin.AddOrUpdateIDPConfig(ctx, madmin.LDAPIDPCfg, madmin.Default, cfgData, true)
	if err != nil {
		resp.Diagnostics.AddError("Updating LDAP IDP configuration", fmt.Sprintf("Failed to update LDAP IDP: %s", err))
		return
	}

	plan.RestartRequired = types.BoolValue(restart)

	tflog.Info(ctx, fmt.Sprintf("Updated LDAP IDP configuration (restart_required=%v)", restart))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *iamIdpLdapResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state iamIdpLdapResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Deleting LDAP IDP configuration")

	_, err := r.client.S3Admin.DeleteIDPConfig(ctx, madmin.LDAPIDPCfg, madmin.Default)
	if err != nil {
		if isIDPConfigNotFound(err) {
			tflog.Warn(ctx, "LDAP IDP configuration already removed")
			return
		}
		resp.Diagnostics.AddError("Deleting LDAP IDP configuration", fmt.Sprintf("Failed to delete LDAP IDP: %s", err))
		return
	}

	state.ID = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *iamIdpLdapResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *iamIdpLdapResource) buildIdpLdapCfgData(model *iamIdpLdapResourceModel) string {
	var parts []string

	addStr := func(key, val string) {
		if val != "" {
			if strings.ContainsAny(val, " \t") {
				parts = append(parts, fmt.Sprintf("%s=%q", key, val))
			} else {
				parts = append(parts, fmt.Sprintf("%s=%s", key, val))
			}
		}
	}
	addBool := func(key string, val bool) {
		if val {
			parts = append(parts, key+"=on")
		} else {
			parts = append(parts, key+"=off")
		}
	}

	addStr("server_addr", model.ServerAddr.ValueString())
	addStr("lookup_bind_dn", model.LookupBindDN.ValueString())
	addStr("lookup_bind_password", model.LookupBindPassword.ValueString())
	addStr("user_dn_search_base_dn", model.UserDNSearchBaseDN.ValueString())
	addStr("user_dn_search_filter", model.UserDNSearchFilter.ValueString())
	addStr("group_search_base_dn", model.GroupSearchBaseDN.ValueString())
	addStr("group_search_filter", model.GroupSearchFilter.ValueString())
	addBool("tls_skip_verify", model.TLSSkipVerify.ValueBool())
	addBool("server_insecure", model.ServerInsecure.ValueBool())
	addBool("starttls", model.StartTLS.ValueBool())
	addBool("enable", model.Enable.ValueBool())

	return strings.Join(parts, " ")
}

func (m *iamIdpLdapResourceModel) SetValueForStringField(fieldName, value string) {
	switch fieldName {
	case "server_addr":
		m.ServerAddr = types.StringValue(value)
	case "lookup_bind_dn":
		m.LookupBindDN = types.StringValue(value)
	case "user_dn_search_base_dn":
		m.UserDNSearchBaseDN = types.StringValue(value)
	case "user_dn_search_filter":
		m.UserDNSearchFilter = types.StringValue(value)
	case "group_search_base_dn":
		m.GroupSearchBaseDN = types.StringValue(value)
	case "group_search_filter":
		m.GroupSearchFilter = types.StringValue(value)
	}
}

func (m *iamIdpLdapResourceModel) SetBoolValue(fieldName string, value bool) {
	switch fieldName {
	case "tls_skip_verify":
		m.TLSSkipVerify = types.BoolValue(value)
	case "server_insecure":
		m.ServerInsecure = types.BoolValue(value)
	case "starttls":
		m.StartTLS = types.BoolValue(value)
	}
}

func idpCfgInfoToMap(info []madmin.IDPCfgInfo) map[string]string {
	m := make(map[string]string, len(info))
	for _, item := range info {
		if item.IsCfg {
			m[item.Key] = item.Value
		}
	}
	return m
}
