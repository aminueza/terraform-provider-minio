package minio

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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
	_ resource.Resource                = &iamUserPolicyAttachmentResource{}
	_ resource.ResourceWithConfigure   = &iamUserPolicyAttachmentResource{}
	_ resource.ResourceWithImportState = &iamUserPolicyAttachmentResource{}

	ldapUserDNPattern = regexp.MustCompile(`^(?:((?:(?:CN|cn|OU|ou)=[^,]+,?)+),)+((?:(?:DC|dc)=[^,]+,?)+)$`)
)

type iamUserPolicyAttachmentResource struct {
	client *S3MinioClient
}

type iamUserPolicyAttachmentResourceModel struct {
	ID         types.String `tfsdk:"id"`
	PolicyName types.String `tfsdk:"policy_name"`
	UserName   types.String `tfsdk:"user_name"`
}

func (r *iamUserPolicyAttachmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_user_policy_attachment"
}

func (r *iamUserPolicyAttachmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an IAM user policy attachment in MinIO. Attaches a policy to a user.",
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
			"user_name": schema.StringAttribute{
				Description: "Name of user",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *iamUserPolicyAttachmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iamUserPolicyAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data iamUserPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policies, err := r.readUserPolicies(ctx, data.UserName.ValueString())
	if err != nil {
		resp.Diagnostics.Append(err...)
		return
	}

	if !containsString(policies, data.PolicyName.ValueString()) {
		policies = append(policies, data.PolicyName.ValueString())
		_, err := r.client.S3Admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{
			Policies: policies,
			User:     data.UserName.ValueString(),
		})
		if err != nil {
			resp.Diagnostics.AddError("Unable to set user policy", err.Error())
			return
		}
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.UserName.ValueString(), data.PolicyName.ValueString()))

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamUserPolicyAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data iamUserPolicyAttachmentResourceModel

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

func (r *iamUserPolicyAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data iamUserPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamUserPolicyAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data iamUserPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policies, err := r.readUserPolicies(ctx, data.UserName.ValueString())
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
		User:     data.UserName.ValueString(),
	})
	if detachErr != nil {
		resp.Diagnostics.AddError("Unable to delete user policy", detachErr.Error())
		return
	}
}

func (r *iamUserPolicyAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID Format",
			fmt.Sprintf("Unexpected format of ID (%q), expected <user-name>/<policy_name>", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_name"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("policy_name"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *iamUserPolicyAttachmentResource) read(ctx context.Context, data *iamUserPolicyAttachmentResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	policies, err := r.readUserPolicies(ctx, data.UserName.ValueString())
	if err != nil {
		diags.Append(err...)
		return diags
	}

	if !containsString(policies, data.PolicyName.ValueString()) {
		data.ID = types.StringNull()
		return diags
	}

	if data.ID.IsNull() {
		data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.UserName.ValueString(), data.PolicyName.ValueString()))
	}

	return diags
}

func (r *iamUserPolicyAttachmentResource) readUserPolicies(ctx context.Context, userName string) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	isLDAPUser := ldapUserDNPattern.MatchString(userName)

	userInfo, err := r.client.S3Admin.GetUserInfo(ctx, userName)
	if err != nil {
		var errResponse madmin.ErrorResponse
		if errors.As(err, &errResponse) {
			if strings.EqualFold(errResponse.Code, "XMinioAdminNoSuchUser") {
				return nil, nil
			}
		}
		if !isLDAPUser {
			diags.AddError("Failed to load user info", err.Error())
			return nil, diags
		}
	}

	if userInfo.PolicyName == "" {
		return nil, nil
	}

	return strings.Split(userInfo.PolicyName, ","), diags
}

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func newIAMUserPolicyAttachmentResource() resource.Resource {
	return &iamUserPolicyAttachmentResource{}
}
