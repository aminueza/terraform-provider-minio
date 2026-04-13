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
	_ resource.Resource                = &iamGroupPolicyAttachmentResource{}
	_ resource.ResourceWithConfigure   = &iamGroupPolicyAttachmentResource{}
	_ resource.ResourceWithImportState = &iamGroupPolicyAttachmentResource{}
)

type iamGroupPolicyAttachmentResource struct {
	client *S3MinioClient
}

type iamGroupPolicyAttachmentResourceModel struct {
	ID         types.String `tfsdk:"id"`
	PolicyName types.String `tfsdk:"policy_name"`
	GroupName  types.String `tfsdk:"group_name"`
}

func (r *iamGroupPolicyAttachmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_group_policy_attachment"
}

func (r *iamGroupPolicyAttachmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an IAM group policy attachment in MinIO. Attaches a policy to a group.",
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
			"group_name": schema.StringAttribute{
				Description: "Name of group to attach policy to",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *iamGroupPolicyAttachmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iamGroupPolicyAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data iamGroupPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policies, err := r.readGroupPolicies(ctx, data.GroupName.ValueString())
	if err != nil {
		resp.Diagnostics.Append(err...)
		return
	}

	if !containsString(policies, data.PolicyName.ValueString()) {
		policies = append(policies, data.PolicyName.ValueString())
		_, err := r.client.S3Admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{
			Policies: policies,
			Group:    data.GroupName.ValueString(),
		})
		if err != nil {
			resp.Diagnostics.AddError("Unable to attach group policy", err.Error())
			return
		}
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.GroupName.ValueString(), data.PolicyName.ValueString()))

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamGroupPolicyAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data iamGroupPolicyAttachmentResourceModel

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

func (r *iamGroupPolicyAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data iamGroupPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamGroupPolicyAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data iamGroupPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policies, err := r.readGroupPolicies(ctx, data.GroupName.ValueString())
	if err != nil {
		resp.Diagnostics.Append(err...)
		return
	}

	if !containsString(policies, data.PolicyName.ValueString()) {
		return
	}

	var detachErr error
	_, detachErr = r.client.S3Admin.DetachPolicy(ctx, madmin.PolicyAssociationReq{
		Policies: []string{data.PolicyName.ValueString()},
		Group:    data.GroupName.ValueString(),
	})
	if detachErr != nil {
		resp.Diagnostics.AddError("Unable to delete group policy", detachErr.Error())
		return
	}
}

func (r *iamGroupPolicyAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID Format",
			fmt.Sprintf("Unexpected format of ID (%q), expected <group-name>/<policy-name>", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_name"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("policy_name"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *iamGroupPolicyAttachmentResource) read(ctx context.Context, data *iamGroupPolicyAttachmentResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	policies, err := r.readGroupPolicies(ctx, data.GroupName.ValueString())
	if err != nil {
		diags.Append(err...)
		return diags
	}

	if !containsString(policies, data.PolicyName.ValueString()) {
		data.ID = types.StringNull()
		return diags
	}

	if data.ID.IsNull() {
		data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.GroupName.ValueString(), data.PolicyName.ValueString()))
	}

	return diags
}

func (r *iamGroupPolicyAttachmentResource) readGroupPolicies(ctx context.Context, groupName string) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	groupInfo, err := r.client.S3Admin.GetGroupDescription(ctx, groupName)
	if err != nil {
		diags.AddError("Failed to load group info", err.Error())
		return nil, diags
	}

	if groupInfo.Policy == "" {
		return nil, nil
	}

	return strings.Split(groupInfo.Policy, ","), diags
}

func newIAMGroupPolicyAttachmentResource() resource.Resource {
	return &iamGroupPolicyAttachmentResource{}
}
