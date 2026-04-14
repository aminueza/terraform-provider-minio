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
	_ resource.Resource                = &iamLDAPUserPolicyAttachmentResource{}
	_ resource.ResourceWithConfigure   = &iamLDAPUserPolicyAttachmentResource{}
	_ resource.ResourceWithImportState = &iamLDAPUserPolicyAttachmentResource{}
)

type iamLDAPUserPolicyAttachmentResource struct {
	client *S3MinioClient
}

type iamLDAPUserPolicyAttachmentResourceModel struct {
	ID         types.String `tfsdk:"id"`
	PolicyName types.String `tfsdk:"policy_name"`
	UserDN     types.String `tfsdk:"user_dn"`
}

func (r *iamLDAPUserPolicyAttachmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_ldap_user_policy_attachment"
}

func (r *iamLDAPUserPolicyAttachmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Attaches LDAP user to a policy. Can be used against both built-in and user-defined policies.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"policy_name": schema.StringAttribute{
				Description: "Name of policy to attach to user",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_dn": schema.StringAttribute{
				Description: "The DN of user to attach policy to",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *iamLDAPUserPolicyAttachmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iamLDAPUserPolicyAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data iamLDAPUserPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policies, err := r.readLDAPUserPolicies(ctx, data.UserDN.ValueString())
	if err != nil {
		resp.Diagnostics.Append(err...)
		return
	}

	if !containsString(policies, data.PolicyName.ValueString()) {
		_, err := r.client.S3Admin.AttachPolicyLDAP(ctx, madmin.PolicyAssociationReq{
			Policies: []string{data.PolicyName.ValueString()},
			User:     data.UserDN.ValueString(),
		})
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Unable to attach user to policy '%s'", data.PolicyName.ValueString()), err.Error())
			return
		}
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.UserDN.ValueString(), data.PolicyName.ValueString()))

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamLDAPUserPolicyAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data iamLDAPUserPolicyAttachmentResourceModel

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

func (r *iamLDAPUserPolicyAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan iamLDAPUserPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Verify the resource still exists
	resp.Diagnostics.Append(r.read(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.ID.IsNull() {
		// Resource no longer exists, remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *iamLDAPUserPolicyAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data iamLDAPUserPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policies, err := r.readLDAPUserPolicies(ctx, data.UserDN.ValueString())
	if err != nil {
		resp.Diagnostics.Append(err...)
		return
	}

	if !containsString(policies, data.PolicyName.ValueString()) {
		return
	}

	_, detachErr := r.client.S3Admin.DetachPolicyLDAP(ctx, madmin.PolicyAssociationReq{
		Policies: []string{data.PolicyName.ValueString()},
		User:     data.UserDN.ValueString(),
	})
	if detachErr != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to detach policy '%s'", data.PolicyName.ValueString()), detachErr.Error())
		return
	}
}

func (r *iamLDAPUserPolicyAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID Format",
			fmt.Sprintf("Unexpected format of ID (%q), expected <user-name>/<policy-name>", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_dn"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("policy_name"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *iamLDAPUserPolicyAttachmentResource) read(ctx context.Context, data *iamLDAPUserPolicyAttachmentResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	// Query for policy entities with both policy and user specified
	per, err := r.client.S3Admin.GetLDAPPolicyEntities(ctx, madmin.PolicyEntitiesQuery{
		Policy: []string{data.PolicyName.ValueString()},
		Users:  []string{data.UserDN.ValueString()},
	})

	if err != nil {
		errMsg := err.Error()
		// Check for LDAP not configured error
		if strings.Contains(errMsg, "LDAP") || strings.Contains(errMsg, "not configured") || strings.Contains(errMsg, "there is no target") {
			// LDAP not configured - remove resource from state
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError(fmt.Sprintf("Failed to query for user policy '%s'", data.PolicyName.ValueString()), err.Error())
		return diags
	}

	if len(per.PolicyMappings) == 0 {
		// Policy not found for user - remove from state
		data.ID = types.StringNull()
		return diags
	}

	if data.ID.IsNull() {
		data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.UserDN.ValueString(), data.PolicyName.ValueString()))
	}

	return diags
}

func (r *iamLDAPUserPolicyAttachmentResource) readLDAPUserPolicies(ctx context.Context, userDN string) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	policyEntities, err := r.client.S3Admin.GetLDAPPolicyEntities(ctx, madmin.PolicyEntitiesQuery{
		Users: []string{userDN},
	})

	if err != nil {
		errMsg := err.Error()
		// Check for LDAP not configured error
		if strings.Contains(errMsg, "LDAP") || strings.Contains(errMsg, "not configured") || strings.Contains(errMsg, "there is no target") {
			// LDAP not configured - return empty slice, not error
			return []string{}, diags
		}
		diags.AddError("Failed to load LDAP user policies", err.Error())
		return nil, diags
	}

	if len(policyEntities.UserMappings) == 0 {
		// User has no policies - return empty slice
		return []string{}, diags
	}

	if len(policyEntities.UserMappings) > 1 {
		diags.AddError("Failed to load user policies",
			fmt.Sprintf("more than one user mapping returned for DN %s", userDN))
		return nil, diags
	}

	return policyEntities.UserMappings[0].Policies, diags
}

func newIAMLDAPUserPolicyAttachmentResource() resource.Resource {
	return &iamLDAPUserPolicyAttachmentResource{}
}
