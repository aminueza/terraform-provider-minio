package minio

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/minio/madmin-go/v3"
)

// Ensure provider defined types fully satisfy framework interfaces
var (
	_ resource.Resource                = &iamUserResource{}
	_ resource.ResourceWithConfigure   = &iamUserResource{}
	_ resource.ResourceWithImportState = &iamUserResource{}
)

// secretWONullModifier sets secret_wo to null in plan when state has null
type secretWONullModifier struct{}

func (m secretWONullModifier) Description(ctx context.Context) string {
	return "Set secret_wo to null in plan when state has null"
}

func (m secretWONullModifier) MarkdownDescription(ctx context.Context) string {
	return "Set secret_wo to null in plan when state has null"
}

func (m secretWONullModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// If state is null (after first apply), set plan to null to avoid diff
	if req.StateValue.IsNull() {
		resp.PlanValue = types.StringNull()
	}
}

// iamUserResource defines the resource implementation
type iamUserResource struct {
	client *S3MinioClient
}

// iamUserResourceModel describes the resource data model
type iamUserResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	ForceDestroy    types.Bool   `tfsdk:"force_destroy"`
	DisableUser     types.Bool   `tfsdk:"disable_user"`
	UpdateSecret    types.Bool   `tfsdk:"update_secret"`
	Status          types.String `tfsdk:"status"`
	Secret          types.String `tfsdk:"secret"`
	SecretWO        types.String `tfsdk:"secret_wo"`
	SecretWOVersion types.Int64  `tfsdk:"secret_wo_version"`
	Tags            types.Map    `tfsdk:"tags"`
}

func (r *iamUserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_user"
}

func (r *iamUserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a MinIO IAM User resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the user.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the user.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						StaticUserNamePattern,
						"only alphanumeric characters, hyphens, underscores, commas, periods, @ symbols, plus and equals signs allowed or a valid LDAP Distinguished Name (DN)",
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"force_destroy": schema.BoolAttribute{
				Description: "Delete user even if it has non-Terraform-managed IAM access keys.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"disable_user": schema.BoolAttribute{
				Description: "Disable user.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"update_secret": schema.BoolAttribute{
				Description: "Rotate Minio User Secret Key.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"status": schema.StringAttribute{
				Description: "Status of the IAM user.",
				Computed:    true,
			},
			"secret": schema.StringAttribute{
				Description: "Secret key for the IAM user.",
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
			},
			"secret_wo": schema.StringAttribute{
				Description: "Write-only secret key for the IAM user.",
				Optional:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					secretWONullModifier{},
				},
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("secret_wo_version")),
				},
			},
			"secret_wo_version": schema.Int64Attribute{
				Description: "Version identifier for secret_wo. Change this value to trigger rotation when using secret_wo.",
				Optional:    true,
				Validators:  []validator.Int64{
					// Note: Int64AtLeast(1) validator would go here if available
				},
			},
			"tags": schema.MapAttribute{
				Description: "A map of tags to assign to the user.",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *iamUserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iamUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data iamUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	accessKey := data.Name.ValueString()
	secretKey := data.Secret.ValueString()

	// Handle secret_wo if provided
	if !data.SecretWO.IsNull() && !data.SecretWO.IsUnknown() {
		secretKey = data.SecretWO.ValueString()
	}

	// Generate secret if not provided
	if secretKey == "" {
		var err error
		if secretKey, err = generateSecretAccessKey(); err != nil {
			resp.Diagnostics.AddError(
				"Error creating user",
				"Could not generate secret key: "+err.Error(),
			)
			return
		}
	}

	// Create user
	err := r.client.S3Admin.AddUser(ctx, accessKey, secretKey)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating user",
			"Could not create user "+accessKey+": "+err.Error(),
		)
		return
	}

	// Set ID
	data.ID = types.StringValue(accessKey)

	// Set secret in state
	if !data.SecretWO.IsNull() && !data.SecretWO.IsUnknown() {
		data.Secret = types.StringValue("")
	} else {
		data.Secret = types.StringValue(secretKey)
	}

	// Clear write-only secret before setting state
	data.SecretWO = types.StringNull()

	// Disable user if requested
	if data.DisableUser.ValueBool() {
		err = r.client.S3Admin.SetUserStatus(ctx, accessKey, madmin.AccountDisabled)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error disabling user",
				"Could not disable user "+accessKey+": "+err.Error(),
			)
			return
		}
	}

	// Read final state
	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data iamUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If ID is null after read (user was deleted externally), don't set state
	// This allows the framework to handle external deletion correctly
	if data.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data iamUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Determine desired status
	wantedStatus := madmin.AccountEnabled
	if data.DisableUser.ValueBool() {
		wantedStatus = madmin.AccountDisabled
	}

	// Check if status changed
	userInfo, err := r.client.S3Admin.GetUserInfo(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading user",
			"Could not get user info: "+err.Error(),
		)
		return
	}

	if userInfo.Status != wantedStatus {
		err = r.client.S3Admin.SetUserStatus(ctx, data.ID.ValueString(), wantedStatus)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating user status",
				"Could not update user status: "+err.Error(),
			)
			return
		}
	}

	// Handle secret updates
	wantedSecret := data.Secret.ValueString()
	if !data.SecretWO.IsNull() && !data.SecretWO.IsUnknown() {
		wantedSecret = data.SecretWO.ValueString()
	}

	// Check if secret should be rotated
	if data.UpdateSecret.ValueBool() {
		if secretKey, err := generateSecretAccessKey(); err != nil {
			resp.Diagnostics.AddError(
				"Error rotating secret",
				"Could not generate new secret: "+err.Error(),
			)
			return
		} else {
			wantedSecret = secretKey
		}
	}

	// Check for secret_wo_version change
	hasSecretWOVersion := !data.SecretWOVersion.IsNull() && !data.SecretWOVersion.IsUnknown()
	hasSecretWOChange := !data.SecretWOVersion.Equal(basetypes.NewInt64Null()) && hasSecretWOVersion

	if hasSecretWOChange && data.SecretWO.IsNull() && data.SecretWO.IsUnknown() {
		resp.Diagnostics.AddError(
			"Error updating secret",
			"secret_wo must be provided when secret_wo_version changes",
		)
		return
	}

	// Update secret if changed
	if data.SecretChanged() || hasSecretWOChange || data.Secret.ValueString() != wantedSecret {
		err = r.client.S3Admin.SetUser(ctx, data.ID.ValueString(), wantedSecret, wantedStatus)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating user secret",
				"Could not update user secret: "+err.Error(),
			)
			return
		}

		if !data.SecretWO.IsNull() && !data.SecretWO.IsUnknown() {
			data.Secret = types.StringValue("")
		} else {
			data.Secret = types.StringValue(wantedSecret)
		}
	}

	// Clear write-only secret before setting state
	data.SecretWO = types.StringNull()

	// Read final state
	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data iamUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Remove user from groups first
	userInfo, err := r.client.S3Admin.GetUserInfo(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading user before deletion",
			"Could not get user info: "+err.Error(),
		)
		return
	}

	// Remove from all groups
	for _, groupMemberOf := range userInfo.MemberOf {
		groupAddRemove := madmin.GroupAddRemove{
			Group:    groupMemberOf,
			Members:  []string{data.ID.ValueString()},
			IsRemove: true,
		}

		err = r.client.S3Admin.UpdateGroupMembers(ctx, groupAddRemove)
		if err != nil {
			if !data.ForceDestroy.ValueBool() {
				resp.Diagnostics.AddError(
					"Error removing group memberships",
					"Could not remove user from group "+groupMemberOf+": "+err.Error(),
				)
				return
			}
		}
	}

	// Delete user
	err = r.client.S3Admin.RemoveUser(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting user",
			"Could not delete user "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Clear ID
	data.ID = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *iamUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

func (r *iamUserResource) read(ctx context.Context, data *iamUserResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	output, err := r.client.S3Admin.GetUserInfo(ctx, data.ID.ValueString())
	if err != nil {
		var errResp madmin.ErrorResponse
		if errors.As(err, &errResp) {
			if errResp.Code == "XMinioAdminNoSuchUser" {
				// User doesn't exist, remove from state
				data.ID = types.StringNull()
				return diags
			}
		}

		diags.AddError(
			"Error reading user",
			"Could not get user info: "+err.Error(),
		)
		return diags
	}

	data.Status = types.StringValue(string(output.Status))
	// Clear write-only attributes
	data.SecretWO = types.StringNull()

	return diags
}

// SecretChanged checks if the secret field has changed
func (m *iamUserResourceModel) SecretChanged() bool {
	// This is a simplified check - in practice, you'd use the plan/old/new values
	return !m.Secret.IsNull() && !m.Secret.IsUnknown()
}

// newIAMUserResource creates a new IAM user resource instance
func newIAMUserResource() resource.Resource {
	return &iamUserResource{}
}
