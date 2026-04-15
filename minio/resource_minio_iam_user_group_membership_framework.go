package minio

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/madmin-go/v3"
)

var (
	_ resource.Resource                = &iamUserGroupMembershipResource{}
	_ resource.ResourceWithConfigure   = &iamUserGroupMembershipResource{}
	_ resource.ResourceWithImportState = &iamUserGroupMembershipResource{}
)

type iamUserGroupMembershipResource struct {
	client *S3MinioClient
}

type iamUserGroupMembershipResourceModel struct {
	ID     types.String `tfsdk:"id"`
	User   types.String `tfsdk:"user"`
	Groups types.Set    `tfsdk:"groups"`
}

func newIAMUserGroupMembershipResource() resource.Resource {
	return &iamUserGroupMembershipResource{}
}

func (r *iamUserGroupMembershipResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_user_group_membership"
}

func (r *iamUserGroupMembershipResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages IAM user group membership. This resource attaches a user to multiple groups and ensures the user is removed from any groups not specified in the configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "User name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user": schema.StringAttribute{
				Required:    true,
				Description: "The name of the IAM user.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"groups": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "A list of IAM groups to add the user to.",
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplaceIf(func(ctx context.Context, sp planmodifier.SetRequest, rrifr *setplanmodifier.RequiresReplaceIfFuncResponse) {
						rrifr.RequiresReplace = true
					}, "User name change requires resource recreation", "Changing the user name requires the resource to be recreated."),
				},
			},
		},
	}
}

func (r *iamUserGroupMembershipResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iamUserGroupMembershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data iamUserGroupMembershipResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.syncUserGroups(ctx, &data, false); err != nil {
		resp.Diagnostics.AddError(
			"Error creating user group membership",
			err.Error(),
		)
		return
	}

	data.ID = data.User

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamUserGroupMembershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data iamUserGroupMembershipResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userInfo, err := r.client.S3Admin.GetUserInfo(ctx, data.User.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading user info",
			err.Error(),
		)
		return
	}

	var groups []string
	resp.Diagnostics.Append(data.Groups.ElementsAs(ctx, &groups, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	desired := make(map[string]struct{})
	for _, g := range groups {
		desired[g] = struct{}{}
	}

	current := make([]string, 0)
	for _, g := range userInfo.MemberOf {
		if _, ok := desired[g]; ok {
			current = append(current, g)
		}
	}

	sort.Strings(current)

	groupSet, diags := types.SetValueFrom(ctx, types.StringType, current)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Groups = groupSet

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamUserGroupMembershipResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data iamUserGroupMembershipResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.syncUserGroups(ctx, &data, true); err != nil {
		resp.Diagnostics.AddError(
			"Error updating user group membership",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamUserGroupMembershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data iamUserGroupMembershipResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var groups []string
	resp.Diagnostics.Append(data.Groups.ElementsAs(ctx, &groups, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, grp := range groups {
		if err := r.client.S3Admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
			Group:    grp,
			Members:  []string{data.User.ValueString()},
			IsRemove: true,
		}); err != nil {
			resp.Diagnostics.AddError(
				"Error removing user from group",
				fmt.Sprintf("removing user %s from group %s: %v", data.User.ValueString(), grp, err),
			)
			return
		}
	}
}

func (r *iamUserGroupMembershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)

	user := parts[0]
	var groups []string

	if len(parts) == 2 {
		groupsStr := parts[1]
		if groupsStr != "" {
			groupList := strings.Split(groupsStr, ",")
			for _, g := range groupList {
				if g := strings.TrimSpace(g); g != "" {
					groups = append(groups, g)
				}
			}
		}
	}

	groupSet, diags := types.SetValueFrom(ctx, types.StringType, groups)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.State.SetAttribute(ctx, path.Root("id"), user)
	resp.State.SetAttribute(ctx, path.Root("user"), user)
	resp.State.SetAttribute(ctx, path.Root("groups"), groupSet)
}

func (r *iamUserGroupMembershipResource) syncUserGroups(ctx context.Context, data *iamUserGroupMembershipResourceModel, isUpdate bool) error {
	var desiredGroups []string
	diags := data.Groups.ElementsAs(ctx, &desiredGroups, false)
	if diags.HasError() {
		return fmt.Errorf("reading groups: %v", diags)
	}

	desired := make(map[string]struct{})
	for _, g := range desiredGroups {
		desired[g] = struct{}{}
	}

	userInfo, err := r.client.S3Admin.GetUserInfo(ctx, data.User.ValueString())
	if err != nil {
		return fmt.Errorf("reading user info: %w", err)
	}

	current := make(map[string]struct{})
	for _, g := range userInfo.MemberOf {
		current[g] = struct{}{}
	}

	for grp := range desired {
		if _, ok := current[grp]; !ok {
			if err := r.client.S3Admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
				Group:    grp,
				Members:  []string{data.User.ValueString()},
				IsRemove: false,
			}); err != nil {
				return fmt.Errorf("adding user to group %s: %w", grp, err)
			}
		}
	}

	for grp := range current {
		if _, ok := desired[grp]; !ok {
			if err := r.client.S3Admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
				Group:    grp,
				Members:  []string{data.User.ValueString()},
				IsRemove: true,
			}); err != nil {
				return fmt.Errorf("removing user from group %s: %w", grp, err)
			}
		}
	}

	if isUpdate {
		var updatedGroups []string
		userInfo, err := r.client.S3Admin.GetUserInfo(ctx, data.User.ValueString())
		if err != nil {
			return fmt.Errorf("reading updated user info: %w", err)
		}

		for _, g := range userInfo.MemberOf {
			if _, ok := desired[g]; ok {
				updatedGroups = append(updatedGroups, g)
			}
		}

		sort.Strings(updatedGroups)

		groupSet, diags := types.SetValueFrom(ctx, types.StringType, updatedGroups)
		if diags.HasError() {
			return fmt.Errorf("converting groups to set: %v", diags)
		}

		data.Groups = groupSet
	}

	return nil
}
