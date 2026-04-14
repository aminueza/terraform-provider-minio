package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
)

var (
	_ resource.Resource                = &iamGroupPolicyResource{}
	_ resource.ResourceWithConfigure   = &iamGroupPolicyResource{}
	_ resource.ResourceWithImportState = &iamGroupPolicyResource{}
)

type iamGroupPolicyResource struct {
	client *S3MinioClient
}

type iamGroupPolicyResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Policy     types.String `tfsdk:"policy"`
	Name       types.String `tfsdk:"name"`
	NamePrefix types.String `tfsdk:"name_prefix"`
	Group      types.String `tfsdk:"group"`
}

func newIAMGroupPolicyResource() resource.Resource {
	return &iamGroupPolicyResource{}
}

func (r *iamGroupPolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_group_policy"
}

func (r *iamGroupPolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an IAM policy attached to a group. This resource creates a standalone policy that can be attached to a group.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the group policy (format: <group-name>:<policy-name>).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"policy": schema.StringAttribute{
				Required:    true,
				Description: "Policy JSON string.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Name of the policy. If omitted, Terraform will assign a random, unique name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name_prefix": schema.StringAttribute{
				Optional:    true,
				Description: "Prefix to the generated policy name. Do not use with `name`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group": schema.StringAttribute{
				Required:    true,
				Description: "Name of group the policy belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *iamGroupPolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iamGroupPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data iamGroupPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	if name == "" {
		if data.NamePrefix.ValueString() != "" {
			name = id.PrefixedUniqueId(data.NamePrefix.ValueString())
		} else {
			name = id.UniqueId()
		}
	}

	policy := data.Policy.ValueString()

	if err := r.client.S3Admin.AddCannedPolicy(ctx, name, []byte(policy)); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create group policy",
			err.Error(),
		)
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s:%s", data.Group.ValueString(), name))
	data.Name = types.StringValue(name)
	// Don't set Policy here - let Read handle it with normalization

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamGroupPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data iamGroupPolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupName, policyName, err := parseIAMGroupPolicyID(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing group policy ID",
			err.Error(),
		)
		return
	}

	info, err := r.client.S3Admin.InfoCannedPolicyV2(ctx, policyName)
	if info == nil || err != nil {
		resp.State.RemoveResource(ctx)
		return
	}

	data.Name = types.StringValue(policyName)
	data.Group = types.StringValue(groupName)

	rawPolicy := strings.TrimSpace(string(info.Policy))
	normalizedPolicy, err := NormalizeAndCompareJSONPolicies(data.Policy.ValueString(), rawPolicy)
	if err != nil {
		normalizedPolicy = rawPolicy
	}
	data.Policy = types.StringValue(normalizedPolicy)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamGroupPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data iamGroupPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, policyName, err := parseIAMGroupPolicyID(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing group policy ID",
			err.Error(),
		)
		return
	}

	policy := data.Policy.ValueString()

	if err := r.client.S3Admin.AddCannedPolicy(ctx, policyName, []byte(policy)); err != nil {
		resp.Diagnostics.AddError(
			"Unable to update group policy",
			err.Error(),
		)
		return
	}

	// Re-read to get normalized policy
	info, err := r.client.S3Admin.InfoCannedPolicyV2(ctx, policyName)
	if err != nil {
		resp.Diagnostics.AddError("Reading updated policy", err.Error())
		return
	}

	actualPolicy := strings.TrimSpace(string(info.Policy))
	normalizedPolicy, err := NormalizeAndCompareJSONPolicies(data.Policy.ValueString(), actualPolicy)
	if err != nil {
		resp.Diagnostics.AddError("Normalizing policy JSON", err.Error())
		return
	}
	data.Policy = types.StringValue(normalizedPolicy)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamGroupPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data iamGroupPolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, policyName, err := parseIAMGroupPolicyID(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing group policy ID",
			err.Error(),
		)
		return
	}

	info, _ := r.client.S3Admin.InfoCannedPolicyV2(ctx, policyName)
	if info == nil {
		return
	}

	if err := r.client.S3Admin.RemoveCannedPolicy(ctx, policyName); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete group policy",
			err.Error(),
		)
		return
	}
}

func (r *iamGroupPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func parseIAMGroupPolicyID(id string) (groupName, policyName string, err error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		err = fmt.Errorf("group_policy id must be of the form <group-name>:<policy-name>, got %s", id)
		return
	}

	groupName = parts[0]
	policyName = parts[1]
	return
}
