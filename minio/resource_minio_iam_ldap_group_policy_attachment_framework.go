package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/madmin-go/v3"
)

var (
	_ resource.Resource                = &iamLDAPGroupPolicyAttachmentResource{}
	_ resource.ResourceWithConfigure   = &iamLDAPGroupPolicyAttachmentResource{}
	_ resource.ResourceWithImportState = &iamLDAPGroupPolicyAttachmentResource{}
)

type iamLDAPGroupPolicyAttachmentResource struct {
	client *S3MinioClient
}

type iamLDAPGroupPolicyAttachmentResourceModel struct {
	ID         types.String `tfsdk:"id"`
	PolicyName types.String `tfsdk:"policy_name"`
	GroupDN    types.String `tfsdk:"group_dn"`
}

func (r *iamLDAPGroupPolicyAttachmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_ldap_group_policy_attachment"
}

func (r *iamLDAPGroupPolicyAttachmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Attaches LDAP group to a policy. Can be used against both built-in and user-defined policies.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"policy_name": schema.StringAttribute{
				Description: "Name of policy to attach to group",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_dn": schema.StringAttribute{
				Description: "The distinguished name (DN) of group to attach policy to",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *iamLDAPGroupPolicyAttachmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iamLDAPGroupPolicyAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data iamLDAPGroupPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policies, err := r.readLDAPGroupPolicies(ctx, data.GroupDN.ValueString())
	if err != nil {
		resp.Diagnostics.Append(err...)
		return
	}

	if !containsString(policies, data.PolicyName.ValueString()) {
		_, err := r.client.S3Admin.AttachPolicyLDAP(ctx, madmin.PolicyAssociationReq{
			Policies: []string{data.PolicyName.ValueString()},
			Group:    data.GroupDN.ValueString(),
		})
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Unable to attach group to policy '%s'", data.PolicyName.ValueString()), err.Error())
			return
		}
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.GroupDN.ValueString(), data.PolicyName.ValueString()))

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamLDAPGroupPolicyAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data iamLDAPGroupPolicyAttachmentResourceModel

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

func (r *iamLDAPGroupPolicyAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data iamLDAPGroupPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamLDAPGroupPolicyAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data iamLDAPGroupPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policies, err := r.readLDAPGroupPolicies(ctx, data.GroupDN.ValueString())
	if err != nil {
		resp.Diagnostics.Append(err...)
		return
	}

	if !containsString(policies, data.PolicyName.ValueString()) {
		return
	}

	_, detachErr := r.client.S3Admin.DetachPolicyLDAP(ctx, madmin.PolicyAssociationReq{
		Policies: []string{data.PolicyName.ValueString()},
		Group:    data.GroupDN.ValueString(),
	})
	if detachErr != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to detach policy '%s'", data.PolicyName.ValueString()), detachErr.Error())
		return
	}
}

func (r *iamLDAPGroupPolicyAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID Format",
			fmt.Sprintf("Unexpected format of ID (%q), expected <group-dn>/<policy-name>", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_dn"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("policy_name"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *iamLDAPGroupPolicyAttachmentResource) read(ctx context.Context, data *iamLDAPGroupPolicyAttachmentResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	per, err := r.client.S3Admin.GetLDAPPolicyEntities(ctx, madmin.PolicyEntitiesQuery{
		Policy: []string{data.PolicyName.ValueString()},
		Groups: []string{data.GroupDN.ValueString()},
	})

	if err != nil {
		diags.AddError(fmt.Sprintf("Failed to query for group policy '%s'", data.PolicyName.ValueString()), err.Error())
		return diags
	}

	if len(per.PolicyMappings) == 0 {
		data.ID = types.StringNull()
		return diags
	}

	if data.ID.IsNull() {
		data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.GroupDN.ValueString(), data.PolicyName.ValueString()))
	}

	return diags
}

func (r *iamLDAPGroupPolicyAttachmentResource) readLDAPGroupPolicies(ctx context.Context, groupDN string) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	policyEntities, err := r.client.S3Admin.GetLDAPPolicyEntities(ctx, madmin.PolicyEntitiesQuery{
		Groups: []string{groupDN},
	})

	if err != nil {
		diags.AddError("Failed to load group info", err.Error())
		return nil, diags
	}

	if len(policyEntities.GroupMappings) == 0 {
		return nil, nil
	}

	if len(policyEntities.GroupMappings) > 1 {
		diags.AddError("Failed to load group info", "more than one group returned when getting LDAP policies for single group")
		return nil, diags
	}

	return policyEntities.GroupMappings[0].Policies, diags
}

func newIAMLDAPGroupPolicyAttachmentResource() resource.Resource {
	return &iamLDAPGroupPolicyAttachmentResource{}
}
