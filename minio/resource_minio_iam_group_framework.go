package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/madmin-go/v3"
)

var (
	_ resource.Resource                = &iamGroupResource{}
	_ resource.ResourceWithConfigure   = &iamGroupResource{}
	_ resource.ResourceWithImportState = &iamGroupResource{}
)

type iamGroupResource struct {
	client *S3MinioClient
}

type iamGroupResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	ForceDestroy types.Bool   `tfsdk:"force_destroy"`
	GroupName    types.String `tfsdk:"group_name"`
	DisableGroup types.Bool   `tfsdk:"disable_group"`
}

func (r *iamGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_group"
}

func (r *iamGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an IAM group in MinIO.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Group name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the group",
				Required:    true,
			},
			"force_destroy": schema.BoolAttribute{
				Description: "Delete group even if it has non-Terraform-managed members",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"group_name": schema.StringAttribute{
				Description: "The name of the group.",
				Computed:    true,
			},
			"disable_group": schema.BoolAttribute{
				Description: "Disable group",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *iamGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iamGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data iamGroupResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupAddRemove := madmin.GroupAddRemove{
		Group:    data.Name.ValueString(),
		Members:  []string{},
		IsRemove: false,
	}

	err := r.client.S3Admin.UpdateGroupMembers(ctx, groupAddRemove)
	if err != nil {
		resp.Diagnostics.AddError("Creating group failed", err.Error())
		return
	}

	resp.Diagnostics.Append(r.setStatus(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = data.Name
	data.GroupName = data.Name

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data iamGroupResourceModel

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

func (r *iamGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data iamGroupResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setStatus(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ForceDestroy.ValueBool() {
		resp.Diagnostics.Append(r.delete(ctx, &data)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data iamGroupResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.delete(ctx, &data)...)
}

func (r *iamGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *iamGroupResource) read(ctx context.Context, data *iamGroupResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	output, err := r.client.S3Admin.GetGroupDescription(ctx, data.Name.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "group does not exist") {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError("Error reading IAM Group", err.Error())
		return diags
	}

	data.GroupName = types.StringValue(output.Name)
	data.DisableGroup = types.BoolValue(output.Status == string(madmin.GroupDisabled))

	if data.ID.IsNull() {
		data.ID = data.Name
	}

	return diags
}

func (r *iamGroupResource) setStatus(ctx context.Context, data *iamGroupResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	var groupStatus madmin.GroupStatus
	if data.DisableGroup.ValueBool() {
		groupStatus = madmin.GroupDisabled
	} else {
		groupStatus = madmin.GroupEnabled
	}

	err := r.client.S3Admin.SetGroupStatus(ctx, data.Name.ValueString(), groupStatus)
	if err != nil {
		diags.AddError("Error updating group status", err.Error())
		return diags
	}

	return diags
}

func (r *iamGroupResource) delete(ctx context.Context, data *iamGroupResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	groupDesc, err := r.client.S3Admin.GetGroupDescription(ctx, data.Name.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			return diags
		}
		diags.AddError("Error reading IAM Group for deletion", err.Error())
		return diags
	}

	if groupDesc.Name == "" {
		return diags
	}

	if len(groupDesc.Policy) == 0 {
		_, _ = r.client.S3Admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{
			Policies: []string{"readonly"},
			Group:    data.Name.ValueString(),
		})
	}

	var members []string
	if data.ForceDestroy.ValueBool() {
		members = groupDesc.Members
	}

	err = r.client.S3Admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
		Group:    data.Name.ValueString(),
		Members:  members,
		IsRemove: true,
	})
	if err != nil {
		diags.AddError("Error deleting IAM Group", err.Error())
		return diags
	}

	return diags
}

func newIAMGroupResource() resource.Resource {
	return &iamGroupResource{}
}
