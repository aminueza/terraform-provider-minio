package minio

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

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
	"github.com/minio/madmin-go/v3"
)

func processServiceAccountPolicy(policy string) []byte {
	if len(policy) == 0 {
		emptyPolicy := "{\n\"Version\": \"\",\n\"Statement\": null\n}"
		return []byte(emptyPolicy)
	}
	return []byte(policy)
}

func parseUserFromParentUser(parentUser string) string {
	user := parentUser

	for _, ldapSection := range strings.Split(parentUser, ",") {
		splitSection := strings.Split(ldapSection, "=")
		if len(splitSection) == 2 && strings.ToLower(strings.TrimSpace(splitSection[0])) == "cn" {
			return strings.TrimSpace(splitSection[1])
		}
	}

	return user
}

// Ensure provider defined types fully satisfy framework interfaces
var (
	_ resource.Resource                = &serviceAccountResource{}
	_ resource.ResourceWithConfigure   = &serviceAccountResource{}
	_ resource.ResourceWithImportState = &serviceAccountResource{}
)

// serviceAccountResource defines the resource implementation
type serviceAccountResource struct {
	client *S3MinioClient
}

// serviceAccountResourceModel describes the resource data model
type serviceAccountResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	TargetUser         types.String `tfsdk:"target_user"`
	DisableUser        types.Bool   `tfsdk:"disable_user"`
	UpdateSecret       types.Bool   `tfsdk:"update_secret"`
	Status             types.String `tfsdk:"status"`
	SecretKey          types.String `tfsdk:"secret_key"`
	SecretKeyWO        types.String `tfsdk:"secret_key_wo"`
	SecretKeyWOVersion types.Int64  `tfsdk:"secret_key_wo_version"`
	AccessKey          types.String `tfsdk:"access_key"`
	Policy             types.String `tfsdk:"policy"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Expiration         types.String `tfsdk:"expiration"`
}

func (r *serviceAccountResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_service_account"
}

func (r *serviceAccountResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a MinIO Service Account resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Access key of the service account.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target_user": schema.StringAttribute{
				Description: "User the service account will be created for.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"disable_user": schema.BoolAttribute{
				Description: "Disable service account.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"update_secret": schema.BoolAttribute{
				Description: "Rotate secret key.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"status": schema.StringAttribute{
				Description: "Status of the service account (on/off).",
				Computed:    true,
			},
			"secret_key": schema.StringAttribute{
				Description: "Secret key of service account.",
				Computed:    true,
				Sensitive:   true,
			},
			"secret_key_wo": schema.StringAttribute{
				Description: "Write-only secret key of service account.",
				Optional:    true,
				Sensitive:   true,
			},
			"secret_key_wo_version": schema.Int64Attribute{
				Description: "Version identifier for secret_key_wo. Change this value to trigger secret key rotation when using secret_key_wo.",
				Optional:    true,
			},
			"access_key": schema.StringAttribute{
				Description: "Access key of service account.",
				Computed:    true,
			},
			"policy": schema.StringAttribute{
				Description: "Policy of service account as encoded JSON string.",
				Optional:    true,
			},
			"name": schema.StringAttribute{
				Description: "Name of service account (32 bytes max), can't be cleared once set.",
				Optional:    true,
			},
			"description": schema.StringAttribute{
				Description: "Description of service account (256 bytes max), can't be cleared once set.",
				Optional:    true,
			},
			"expiration": schema.StringAttribute{
				Description: "Expiration of service account in RFC3339 format. Must be between NOW+15min & NOW+365d. If not set, the service account will not expire.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z?$`),
						"must be in RFC3339 format",
					),
				},
			},
		},
	}
}

func (r *serviceAccountResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *serviceAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data serviceAccountResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	targetUser := data.TargetUser.ValueString()
	policy := data.Policy.ValueString()
	secretWO := data.SecretKeyWO.ValueString()
	hasSecretWO := !data.SecretKeyWO.IsNull() && !data.SecretKeyWO.IsUnknown()

	// Parse expiration if provided
	var expirationPtr *time.Time
	if !data.Expiration.IsNull() && !data.Expiration.IsUnknown() {
		expiration, err := time.Parse(time.RFC3339, data.Expiration.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to parse expiration",
				data.Expiration.ValueString()+": "+err.Error(),
			)
			return
		}
		expirationPtr = &expiration
	}

	// Create service account
	addReq := madmin.AddServiceAccountReq{
		Policy:      processServiceAccountPolicy(policy),
		TargetUser:  targetUser,
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Expiration:  expirationPtr,
	}

	serviceAccount, err := r.client.S3Admin.AddServiceAccount(ctx, addReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating service account",
			"Could not create service account for "+targetUser+": "+err.Error(),
		)
		return
	}

	accessKey := serviceAccount.AccessKey
	secretKey := serviceAccount.SecretKey

	// Set ID
	data.ID = types.StringValue(accessKey)
	data.AccessKey = types.StringValue(accessKey)

	// Handle secret key
	if hasSecretWO {
		err = r.client.S3Admin.UpdateServiceAccount(ctx, accessKey, madmin.UpdateServiceAccountReq{NewSecretKey: secretWO})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating service account secret",
				"Could not update secret: "+err.Error(),
			)
			return
		}
		data.SecretKey = types.StringValue("")
	} else {
		data.SecretKey = types.StringValue(secretKey)
	}

	// Disable if requested
	if data.DisableUser.ValueBool() {
		err = r.client.S3Admin.UpdateServiceAccount(ctx, accessKey, madmin.UpdateServiceAccountReq{NewStatus: "off"})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error disabling service account",
				"Could not disable service account: "+err.Error(),
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

func (r *serviceAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data serviceAccountResourceModel

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

func (r *serviceAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data serviceAccountResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Determine desired status
	wantedStatus := "on"
	if data.DisableUser.ValueBool() {
		wantedStatus = "off"
	}

	// Get current service account info
	serviceAccountInfo, err := r.client.S3Admin.InfoServiceAccount(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading service account",
			"Could not get service account info: "+err.Error(),
		)
		return
	}

	// Update status if changed
	if serviceAccountInfo.AccountStatus != wantedStatus {
		err = r.client.S3Admin.UpdateServiceAccount(ctx, data.ID.ValueString(), madmin.UpdateServiceAccountReq{
			NewStatus: wantedStatus,
			NewPolicy: processServiceAccountPolicy(data.Policy.ValueString()),
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating service account status",
				"Could not update status: "+err.Error(),
			)
			return
		}
	}

	// Handle secret updates
	wantedSecret := data.SecretKey.ValueString()
	hasSecretWO := !data.SecretKeyWO.IsNull() && !data.SecretKeyWO.IsUnknown()
	if hasSecretWO {
		wantedSecret = data.SecretKeyWO.ValueString()
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

	// Check for secret_key_wo_version change
	hasSecretWOVersion := !data.SecretKeyWOVersion.IsNull() && !data.SecretKeyWOVersion.IsUnknown()
	hasSecretWOChange := hasSecretWOVersion

	if hasSecretWOChange && !hasSecretWO {
		resp.Diagnostics.AddError(
			"Error updating secret",
			"secret_key_wo must be provided when secret_key_wo_version changes",
		)
		return
	}

	// Update secret if changed
	if (!data.SecretKey.IsNull() && !data.SecretKey.IsUnknown()) || hasSecretWOChange || data.SecretKey.ValueString() != wantedSecret {
		err = r.client.S3Admin.UpdateServiceAccount(ctx, data.ID.ValueString(), madmin.UpdateServiceAccountReq{
			NewSecretKey: wantedSecret,
			NewPolicy:    processServiceAccountPolicy(data.Policy.ValueString()),
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating service account secret",
				"Could not update secret: "+err.Error(),
			)
			return
		}

		if hasSecretWO {
			data.SecretKey = types.StringValue("")
		} else {
			data.SecretKey = types.StringValue(wantedSecret)
		}
	}

	// Update policy if changed
	if !data.Policy.IsNull() && !data.Policy.IsUnknown() {
		err = r.client.S3Admin.UpdateServiceAccount(ctx, data.ID.ValueString(), madmin.UpdateServiceAccountReq{
			NewPolicy: processServiceAccountPolicy(data.Policy.ValueString()),
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating service account policy",
				"Could not update policy: "+err.Error(),
			)
			return
		}
	}

	// Update name if changed
	if !data.Name.IsNull() && !data.Name.IsUnknown() {
		if data.Name.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Error updating service account name",
				"MinIO does not support removing service account names",
			)
			return
		}
		err = r.client.S3Admin.UpdateServiceAccount(ctx, data.ID.ValueString(), madmin.UpdateServiceAccountReq{
			NewName:   data.Name.ValueString(),
			NewPolicy: processServiceAccountPolicy(data.Policy.ValueString()),
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating service account name",
				"Could not update name: "+err.Error(),
			)
			return
		}
	}

	// Update description if changed
	if !data.Description.IsNull() && !data.Description.IsUnknown() {
		if data.Description.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Error updating service account description",
				"MinIO does not support removing service account descriptions",
			)
			return
		}
		err = r.client.S3Admin.UpdateServiceAccount(ctx, data.ID.ValueString(), madmin.UpdateServiceAccountReq{
			NewDescription: data.Description.ValueString(),
			NewPolicy:      processServiceAccountPolicy(data.Policy.ValueString()),
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating service account description",
				"Could not update description: "+err.Error(),
			)
			return
		}
	}

	// Update expiration if changed
	if !data.Expiration.IsNull() && !data.Expiration.IsUnknown() {
		var expirationPtr *time.Time
		if !data.Expiration.IsNull() && !data.Expiration.IsUnknown() {
			expiration, err := time.Parse(time.RFC3339, data.Expiration.ValueString())
			if err != nil {
				resp.Diagnostics.AddError(
					"Error parsing service account expiration",
					data.Expiration.ValueString()+": "+err.Error(),
				)
				return
			}
			expirationPtr = &expiration
		}
		err = r.client.S3Admin.UpdateServiceAccount(ctx, data.ID.ValueString(), madmin.UpdateServiceAccountReq{
			NewExpiration: expirationPtr,
			NewPolicy:     processServiceAccountPolicy(data.Policy.ValueString()),
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating service account expiration",
				"Could not update expiration: "+err.Error(),
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

func (r *serviceAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data serviceAccountResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete service account
	err := r.client.S3Admin.DeleteServiceAccount(ctx, data.ID.ValueString())
	if err != nil {
		// Check if it still exists (idempotency)
		serviceAccountList, listErr := r.client.S3Admin.ListServiceAccounts(ctx, data.TargetUser.ValueString())
		if listErr != nil {
			resp.Diagnostics.AddError(
				"Error listing service accounts",
				listErr.Error(),
			)
			return
		}

		for _, account := range serviceAccountList.Accounts {
			if account.AccessKey == data.ID.ValueString() {
				resp.Diagnostics.AddError(
					"Error deleting service account",
					"Service account "+data.ID.ValueString()+" not deleted",
				)
				return
			}
		}
		// Not found, consider it deleted
	}

	// Clear ID
	data.ID = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *serviceAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *serviceAccountResource) read(ctx context.Context, data *serviceAccountResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	output, err := r.client.S3Admin.InfoServiceAccount(ctx, data.ID.ValueString())
	if err != nil {
		// Check for not found error
		if err.Error() == "The specified service account is not found (Specified service account does not exist)" {
			data.ID = types.StringNull()
			return diags
		}
		diags.AddError(
			"Error reading service account",
			"Could not read service account: "+err.Error(),
		)
		return diags
	}

	// Set status
	data.Status = types.StringValue(output.AccountStatus)
	data.DisableUser = types.BoolValue(output.AccountStatus == "off")

	// Set target user (parse from parent user if LDAP)
	targetUser := parseUserFromParentUser(output.ParentUser)
	data.TargetUser = types.StringValue(targetUser)

	// Set policy if not implied
	if !output.ImpliedPolicy {
		data.Policy = types.StringValue(output.Policy)
	}

	// Set name and description
	data.Name = types.StringValue(output.Name)
	data.Description = types.StringValue(output.Description)

	// Set expiration
	var expiration string
	if output.Expiration != nil {
		expiration = output.Expiration.Format(time.RFC3339)
	}
	data.Expiration = types.StringValue(expiration)

	return diags
}

// newServiceAccountResource creates a new service account resource instance
func newServiceAccountResource() resource.Resource {
	return &serviceAccountResource{}
}
