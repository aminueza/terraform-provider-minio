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
	_ resource.Resource                = &iamGroupUserAttachmentResource{}
	_ resource.ResourceWithConfigure   = &iamGroupUserAttachmentResource{}
	_ resource.ResourceWithImportState = &iamGroupUserAttachmentResource{}
)

type iamGroupUserAttachmentResource struct {
	client *S3MinioClient
}

type iamGroupUserAttachmentResourceModel struct {
	ID        types.String `tfsdk:"id"`
	UserName  types.String `tfsdk:"user_name"`
	GroupName types.String `tfsdk:"group_name"`
}

func (r *iamGroupUserAttachmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_group_user_attachment"
}

func (r *iamGroupUserAttachmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an IAM group user attachment in MinIO. Adds a user to a group.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_name": schema.StringAttribute{
				Description: "Name of user to add to group",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_name": schema.StringAttribute{
				Description: "Name of group to add user to",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *iamGroupUserAttachmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iamGroupUserAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data iamGroupUserAttachmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupDesc, err := r.client.S3Admin.GetGroupDescription(ctx, data.GroupName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Getting group description", err.Error())
		return
	}

	var members []string
	members = append(members, groupDesc.Members...)
	members = append(members, data.UserName.ValueString())

	err = r.client.S3Admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
		Group:    data.GroupName.ValueString(),
		Members:  members,
		IsRemove: false,
	})
	if err != nil {
		resp.Diagnostics.AddError("Adding user to group", err.Error())
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.GroupName.ValueString(), data.UserName.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamGroupUserAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data iamGroupUserAttachmentResourceModel

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

func (r *iamGroupUserAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data iamGroupUserAttachmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamGroupUserAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data iamGroupUserAttachmentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupDesc, err := r.client.S3Admin.GetGroupDescription(ctx, data.GroupName.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			return
		}
		resp.Diagnostics.AddError("Getting group description", err.Error())
		return
	}

	var members []string
	for _, member := range groupDesc.Members {
		if member != data.UserName.ValueString() {
			members = append(members, member)
		}
	}

	err = r.client.S3Admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
		Group:    data.GroupName.ValueString(),
		Members:  members,
		IsRemove: false,
	})
	if err != nil {
		if !strings.Contains(err.Error(), "does not exist") {
			resp.Diagnostics.AddError("Removing user from group", err.Error())
			return
		}
	}
}

func (r *iamGroupUserAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID Format",
			fmt.Sprintf("Unexpected format of ID (%q), expected <group-name>/<user-name>", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_name"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_name"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *iamGroupUserAttachmentResource) read(ctx context.Context, data *iamGroupUserAttachmentResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	groupDesc, err := r.client.S3Admin.GetGroupDescription(ctx, data.GroupName.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError("Getting group description", err.Error())
		return diags
	}

	found := false
	for _, member := range groupDesc.Members {
		if member == data.UserName.ValueString() {
			found = true
			break
		}
	}

	if !found {
		data.ID = types.StringNull()
		return diags
	}

	if data.ID.IsNull() {
		data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.GroupName.ValueString(), data.UserName.ValueString()))
	}

	return diags
}

func newIAMGroupUserAttachmentResource() resource.Resource {
	return &iamGroupUserAttachmentResource{}
}
