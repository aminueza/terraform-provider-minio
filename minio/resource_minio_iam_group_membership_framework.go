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
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/minio/madmin-go/v3"
)

var (
	_ resource.Resource                = &iamGroupMembershipResource{}
	_ resource.ResourceWithConfigure   = &iamGroupMembershipResource{}
	_ resource.ResourceWithImportState = &iamGroupMembershipResource{}
)

type iamGroupMembershipResource struct {
	client *S3MinioClient
}

type iamGroupMembershipResourceModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Users types.Set    `tfsdk:"users"`
	Group types.String `tfsdk:"group"`
}

func (r *iamGroupMembershipResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_group_membership"
}

func (r *iamGroupMembershipResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an IAM group membership in MinIO. Adds users to a group.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of group membership",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"users": schema.SetAttribute{
				Description: "Add user or list of users such as a group membership",
				Required:    true,
				ElementType: types.StringType,
			},
			"group": schema.StringAttribute{
				Description: "Group name to add users",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *iamGroupMembershipResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iamGroupMembershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data iamGroupMembershipResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	users := r.usersToStringSlice(data.Users)

	groupAddRemove := madmin.GroupAddRemove{
		Group:    data.Group.ValueString(),
		Members:  users,
		IsRemove: false,
	}

	err := r.client.S3Admin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		resp.Diagnostics.AddError("Error adding user(s) to group", err.Error())
		return
	}

	data.ID = data.Name

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamGroupMembershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data iamGroupMembershipResourceModel

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

func (r *iamGroupMembershipResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data iamGroupMembershipResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state iamGroupMembershipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.Users.Equal(state.Users) {
		oldUsersSet := state.Users
		newUsersSet := data.Users

		if oldUsersSet.IsNull() {
			oldUsersSet = types.SetNull(types.StringType)
		}
		if newUsersSet.IsNull() {
			newUsersSet = types.SetNull(types.StringType)
		}

		oldUsersSlice := r.usersToStringSlice(oldUsersSet)
		newUsersSlice := r.usersToStringSlice(newUsersSet)

		usersToRemoveMap := make(map[string]bool)
		for _, u := range oldUsersSlice {
			usersToRemoveMap[u] = true
		}
		for _, u := range newUsersSlice {
			delete(usersToRemoveMap, u)
		}

		usersToAddMap := make(map[string]bool)
		for _, u := range newUsersSlice {
			usersToAddMap[u] = true
		}
		for _, u := range oldUsersSlice {
			delete(usersToAddMap, u)
		}

		var usersToRemove, usersToAdd []string
		for u := range usersToRemoveMap {
			usersToRemove = append(usersToRemove, u)
		}
		for u := range usersToAddMap {
			usersToAdd = append(usersToAdd, u)
		}

		if len(usersToAdd) > 0 {
			if err := r.userToAdd(ctx, &data, usersToAdd); err != nil {
				resp.Diagnostics.AddError("Adding users to group", err.Error())
				return
			}
		}

		if len(usersToRemove) > 0 {
			if err := r.userToRemove(ctx, &data, usersToRemove); err != nil {
				if !strings.Contains(err.Error(), "does not exist") {
					resp.Diagnostics.AddError("Removing users from group", err.Error())
					return
				}
			}
		}
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamGroupMembershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data iamGroupMembershipResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	users := r.usersToStringSlice(data.Users)

	groupAddRemove := madmin.GroupAddRemove{
		Group:    data.Group.ValueString(),
		Members:  users,
		IsRemove: true,
	}

	err := r.client.S3Admin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		if !strings.Contains(err.Error(), "does not exist") {
			resp.Diagnostics.AddError("Error deleting user(s) from group", err.Error())
			return
		}
	}
}

func (r *iamGroupMembershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *iamGroupMembershipResource) read(ctx context.Context, data *iamGroupMembershipResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	groupDesc, err := r.client.S3Admin.GetGroupDescription(ctx, data.Group.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError("Error reading IAM Group", err.Error())
		return diags
	}

	if groupDesc.Name == "" {
		data.ID = types.StringNull()
		return diags
	}

	users, userDiags := types.SetValueFrom(ctx, types.StringType, groupDesc.Members)
	diags.Append(userDiags...)
	if diags.HasError() {
		return diags
	}

	data.Users = users

	if data.ID.IsNull() {
		data.ID = data.Name
	}

	return diags
}

func (r *iamGroupMembershipResource) userToAdd(ctx context.Context, data *iamGroupMembershipResourceModel, usersToAdd []string) error {
	groupDesc, err := r.client.S3Admin.GetGroupDescription(ctx, data.Group.ValueString())
	if err != nil {
		return fmt.Errorf("error getting group description: %w", err)
	}

	var users []string
	users = append(users, groupDesc.Members...)
	users = append(users, usersToAdd...)

	groupAddRemove := madmin.GroupAddRemove{
		Group:    data.Group.ValueString(),
		Members:  users,
		IsRemove: false,
	}

	err = r.client.S3Admin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		return fmt.Errorf("error updating user(s) to group %s: %w", data.Group.ValueString(), err)
	}

	return nil
}

func (r *iamGroupMembershipResource) userToRemove(ctx context.Context, data *iamGroupMembershipResourceModel, usersToRemove []string) error {
	groupAddRemove := madmin.GroupAddRemove{
		Group:    data.Group.ValueString(),
		Members:  usersToRemove,
		IsRemove: true,
	}

	_, err := r.client.S3Admin.GetGroupDescription(ctx, data.Group.ValueString())
	if err != nil {
		return fmt.Errorf("error getting group description: %w", err)
	}

	err = r.client.S3Admin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		return fmt.Errorf("error updating user(s) to group %s: %w", data.Group.ValueString(), err)
	}

	return nil
}

func (r *iamGroupMembershipResource) usersToStringSlice(usersSet types.Set) []string {
	if usersSet.IsNull() || usersSet.IsUnknown() {
		return []string{}
	}

	var users []string
	for _, user := range usersSet.Elements() {
		if str, ok := user.(basetypes.StringValue); ok {
			users = append(users, str.ValueString())
		}
	}

	return users
}

func newIAMGroupMembershipResource() resource.Resource {
	return &iamGroupMembershipResource{}
}
