package minio

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/madmin-go/v3"
)

// policySemanticEqualityModifier suppresses plan changes when the policy JSON is semantically equivalent
type policySemanticEqualityModifier struct{}

func (m policySemanticEqualityModifier) Description(ctx context.Context) string {
	return "Suppresses plan changes when policy JSON is semantically equivalent"
}

func (m policySemanticEqualityModifier) MarkdownDescription(ctx context.Context) string {
	return "Suppresses plan changes when policy JSON is semantically equivalent"
}

func (m policySemanticEqualityModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// If the policy is null or unknown, let it pass through
	if req.PlanValue.IsUnknown() || req.PlanValue.IsNull() {
		return
	}

	// If the state is null or unknown, let it pass through
	if req.StateValue.IsUnknown() || req.StateValue.IsNull() {
		return
	}

	planPolicy := req.PlanValue.ValueString()
	statePolicy := req.StateValue.ValueString()

	// Use AWS policy equivalence checker which handles array reordering
	equivalent, err := awspolicy.PoliciesAreEquivalent(planPolicy, statePolicy)
	if err == nil && equivalent {
		resp.PlanValue = req.StateValue
	}
}

// Ensure provider defined types fully satisfy framework interfaces
var (
	_ resource.Resource                = &iamPolicyResource{}
	_ resource.ResourceWithConfigure   = &iamPolicyResource{}
	_ resource.ResourceWithImportState = &iamPolicyResource{}
)

// iamPolicyResource defines the resource implementation
type iamPolicyResource struct {
	client *S3MinioClient
}

// iamPolicyResourceModel describes the resource data model
type iamPolicyResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	NamePrefix types.String `tfsdk:"name_prefix"`
	Policy     types.String `tfsdk:"policy"`
}

func (r *iamPolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_policy"
}

func (r *iamPolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a MinIO IAM Policy resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the policy.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the policy. Conflicts with name_prefix.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("name_prefix")),
					stringvalidator.LengthAtMost(128),
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[\w+=,.@:/-]*$`),
						"must match [\\w+=,.@:/-]",
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name_prefix": schema.StringAttribute{
				Description: "Prefix to the generated policy name. Do not use with name.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("name")),
					stringvalidator.LengthAtMost(128),
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[\w+=,.@:/-]*$`),
						"must match [\\w+=,.@:/-]",
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"policy": schema.StringAttribute{
				Description: "Policy JSON string.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					policySemanticEqualityModifier{},
				},
			},
		},
	}
}

func (r *iamPolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iamPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data iamPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate policy JSON
	if err := validatePolicyJSON(data.Policy.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Invalid policy JSON",
			err.Error(),
		)
		return
	}

	// Determine policy name
	var name string
	if !data.Name.IsNull() && !data.Name.IsUnknown() {
		name = data.Name.ValueString()
	} else if !data.NamePrefix.IsNull() && !data.NamePrefix.IsUnknown() {
		// Use simple prefix-based naming (framework version of PrefixedUniqueId)
		name = data.NamePrefix.ValueString() + "-" + generateUniqueID()
	} else {
		// Generate unique name
		name = generateUniqueID()
	}

	// Create policy
	err := r.client.S3Admin.AddCannedPolicy(ctx, name, []byte(data.Policy.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating policy",
			"Could not create policy "+name+": "+err.Error(),
		)
		return
	}

	// Set ID and name
	data.ID = types.StringValue(name)
	data.Name = types.StringValue(name)

	// Don't call read() to preserve user's formatting - MinIO API reformats JSON
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data iamPolicyResourceModel

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

func (r *iamPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data iamPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate policy JSON
	if err := validatePolicyJSON(data.Policy.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Invalid policy JSON",
			err.Error(),
		)
		return
	}

	// Update policy
	err := r.client.S3Admin.AddCannedPolicy(ctx, data.ID.ValueString(), []byte(data.Policy.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating policy",
			"Could not update policy "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Don't call read() to preserve user's formatting - MinIO API reformats JSON
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data iamPolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete policy
	err := r.client.S3Admin.RemoveCannedPolicy(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting policy",
			"Could not delete policy "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Clear ID
	data.ID = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *iamPolicyResource) read(ctx context.Context, data *iamPolicyResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	info, err := r.client.S3Admin.InfoCannedPolicyV2(ctx, data.ID.ValueString())
	if err != nil {
		var errResp madmin.ErrorResponse
		if errors.As(err, &errResp) {
			if errResp.Code == "XMinioAdminNoSuchPolicy" {
				// Policy doesn't exist, remove from state
				data.ID = types.StringNull()
				return diags
			}
		}

		diags.AddError(
			"Error reading policy",
			"Could not read policy: "+err.Error(),
		)
		return diags
	}

	actualPolicyText := strings.TrimSpace(string(info.Policy))
	existingPolicy := ""
	if !data.Policy.IsNull() && !data.Policy.IsUnknown() {
		existingPolicy = data.Policy.ValueString()
	}

	// Always preserve user's formatting if it exists - MinIO API may reorder arrays
	// but the policy is semantically equivalent
	if existingPolicy != "" {
		data.Policy = types.StringValue(existingPolicy)
		data.Name = types.StringValue(data.ID.ValueString())
		return diags
	}

	data.Policy = types.StringValue(actualPolicyText)
	data.Name = types.StringValue(data.ID.ValueString())

	return diags
}

// validatePolicyJSON validates that a string is valid JSON policy
func validatePolicyJSON(policy string) error {
	if len(policy) < 1 {
		return fmt.Errorf("policy contains an invalid JSON policy")
	}
	if policy[:1] != "{" {
		return fmt.Errorf("policy contains an invalid JSON policy")
	}
	if _, err := structure.NormalizeJsonString(policy); err != nil {
		return fmt.Errorf("policy contains an invalid JSON: %s", err)
	}
	return nil
}

// generateUniqueID generates a unique ID for policy names
func generateUniqueID() string {
	// Simple unique ID generation (in production, use proper UUID or similar)
	return fmt.Sprintf("policy-%d", time.Now().UnixNano())
}

// newIAMPolicyResource creates a new IAM policy resource instance
func newIAMPolicyResource() resource.Resource {
	return &iamPolicyResource{}
}
