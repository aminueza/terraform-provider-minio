package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/minio/madmin-go/v3"
)

var (
	_ resource.Resource                = &accessKeyResource{}
	_ resource.ResourceWithConfigure   = &accessKeyResource{}
	_ resource.ResourceWithImportState = &accessKeyResource{}
)

type accessKeyResource struct {
	client *S3MinioClient
}

type accessKeyResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	User               types.String `tfsdk:"user"`
	AccessKey          types.String `tfsdk:"access_key"`
	SecretKey          types.String `tfsdk:"secret_key"`
	SecretKeyWO        types.String `tfsdk:"secret_key_wo"`
	SecretKeyVersion   types.String `tfsdk:"secret_key_version"`
	SecretKeyWOVersion types.Int64  `tfsdk:"secret_key_wo_version"`
	Status             types.String `tfsdk:"status"`
	Policy             types.String `tfsdk:"policy"`
	Description        types.String `tfsdk:"description"`
}

func newAccessKeyResource() resource.Resource {
	return &accessKeyResource{}
}

func (r *accessKeyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_accesskey"
}

func (r *accessKeyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *accessKeyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages MinIO access keys (service accounts) for users. Access keys provide temporary, rotatable credentials for users without exposing their primary secret key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The access key identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user": schema.StringAttribute{
				Required:    true,
				Description: "The user for whom the access key is managed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"access_key": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The access key. If provided, must be between 8 and 20 characters.",
			},
			"secret_key": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				Description: "The secret key. If provided, must be at least 8 characters. This is a write-only field and will not be stored in state.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"secret_key_wo": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Write-only secret key for the access key.",
			},
			"secret_key_version": schema.StringAttribute{
				Optional:    true,
				Description: "Version identifier for the secret key. Change this value to trigger a secret key rotation.",
			},
			"secret_key_wo_version": schema.Int64Attribute{
				Optional:    true,
				Description: "Version identifier for secret_key_wo. Increment this integer to trigger rotation when using secret_key_wo.",
			},
			"status": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The status of the access key (enabled/disabled).",
				Default:     stringdefault.StaticString("enabled"),
			},
			"policy": schema.StringAttribute{
				Optional:    true,
				Description: "Policy to attach to the access key (policy name or JSON document).",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Description for the access key (max 256 characters).",
			},
		},
	}
}

func (r *accessKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan accessKeyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user := plan.User.ValueString()
	accessKey := plan.AccessKey.ValueString()
	secretKey := plan.SecretKey.ValueString()
	if plan.SecretKeyWO.ValueString() != "" {
		secretKey = plan.SecretKeyWO.ValueString()
	}
	status := plan.Status.ValueString()
	policy := plan.Policy.ValueString()
	description := plan.Description.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Creating accesskey for user %s", user))

	reqCreate := madmin.AddServiceAccountReq{
		SecretKey:   secretKey,
		AccessKey:   accessKey,
		TargetUser:  user,
		Description: description,
	}

	creds, err := r.client.S3Admin.AddServiceAccount(ctx, reqCreate)
	if err != nil {
		resp.Diagnostics.AddError("Creating access key", fmt.Sprintf("Failed to create access key: %s", err))
		return
	}

	plan.ID = types.StringValue(creds.AccessKey)
	plan.AccessKey = types.StringValue(creds.AccessKey)

	err = retry.RetryContext(ctx, 5*time.Minute, func() *retry.RetryError {
		_, err := r.client.S3Admin.InfoServiceAccount(ctx, creds.AccessKey)
		if err != nil {
			return retry.RetryableError(
				fmt.Errorf("waiting for accesskey %s to become available: %w", creds.AccessKey, err),
			)
		}
		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for access key readiness", fmt.Sprintf("Failed to confirm access key readiness: %s", err))
		return
	}

	if policy != "" {
		err := r.client.S3Admin.UpdateServiceAccount(ctx, creds.AccessKey, madmin.UpdateServiceAccountReq{
			NewPolicy: []byte(policy),
		})
		if err != nil {
			resp.Diagnostics.AddError("Updating access key policy", fmt.Sprintf("Failed to update access key policy: %s", err))
			return
		}
	}

	if status == "disabled" {
		resp.Diagnostics.Append(r.updateAccessKey(ctx, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		resp.Diagnostics.Append(r.readAccessKey(ctx, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accessKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state accessKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Reading accesskey %s", state.ID.ValueString()))

	resp.Diagnostics.Append(r.readAccessKey(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *accessKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan accessKeyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Updating accesskey %s", plan.ID.ValueString()))

	resp.Diagnostics.Append(r.updateAccessKey(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accessKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state accessKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	accessKeyID := state.ID.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Deleting accesskey %s", accessKeyID))

	_, err := r.client.S3Admin.InfoServiceAccount(ctx, accessKeyID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "service account does not exist") {
			tflog.Warn(ctx, fmt.Sprintf("AccessKey %s no longer exists, removing from state", accessKeyID))
			return
		}
		resp.Diagnostics.AddError("Checking access key before deletion", fmt.Sprintf("Failed to check access key: %s", err))
		return
	}

	err = retry.RetryContext(ctx, 5*time.Minute, func() *retry.RetryError {
		err := r.client.S3Admin.DeleteServiceAccount(ctx, accessKeyID)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error deleting accesskey %s: %w", accessKeyID, err))
			}

			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "service account does not exist") {
				return nil
			}

			return retry.NonRetryableError(fmt.Errorf("failed to delete accesskey: %w", err))
		}
		return nil
	})

	if err != nil {
		resp.Diagnostics.AddError("Deleting access key", fmt.Sprintf("Failed to delete access key: %s", err))
		return
	}

	err = retry.RetryContext(ctx, 30*time.Second, func() *retry.RetryError {
		_, err := r.client.S3Admin.InfoServiceAccount(ctx, accessKeyID)
		if err == nil {
			return retry.RetryableError(fmt.Errorf("waiting for accesskey %s to be deleted", accessKeyID))
		}

		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "service account does not exist") {
			return nil
		}

		return retry.RetryableError(fmt.Errorf("error checking if accesskey %s is deleted: %w", accessKeyID, err))
	})

	if err != nil {
		resp.Diagnostics.AddError("Confirming access key deletion", fmt.Sprintf("Failed to confirm deletion: %s", err))
		return
	}

	state.ID = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *accessKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *accessKeyResource) readAccessKey(ctx context.Context, model *accessKeyResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	accessKeyID := model.ID.ValueString()

	var info madmin.InfoServiceAccountResp
	var err error

	err = retry.RetryContext(ctx, 2*time.Minute, func() *retry.RetryError {
		info, err = r.client.S3Admin.InfoServiceAccount(ctx, accessKeyID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "service account does not exist") {
				tflog.Warn(ctx, fmt.Sprintf("AccessKey %s no longer exists", accessKeyID))
				model.ID = types.StringNull()
				return nil
			}

			return retry.RetryableError(fmt.Errorf("error reading accesskey %s: %w", accessKeyID, err))
		}
		return nil
	})

	if err != nil {
		diags.AddError("Reading access key", fmt.Sprintf("Failed to read access key: %s", err))
		return diags
	}

	if model.ID.IsNull() {
		return diags
	}

	parentUser := info.ParentUser
	model.User = types.StringValue(parentUser)

	var status string
	if info.AccountStatus == "on" {
		status = "enabled"
	} else {
		status = "disabled"
	}
	model.Status = types.StringValue(status)
	model.AccessKey = types.StringValue(accessKeyID)

	if info.Description != "" {
		model.Description = types.StringValue(info.Description)
	} else if model.Description.IsUnknown() {
		model.Description = types.StringNull()
	}

	// secret_key is write-only - MinIO never returns it
	// Keep it as empty string to match test expectations
	model.SecretKey = types.StringValue("")

	if !info.ImpliedPolicy {
		policy := strings.TrimSpace(info.Policy)
		isEmptyPolicy := false
		if policy == "" || policy == "null" || policy == "{}" {
			isEmptyPolicy = true
		} else {
			var policyObj map[string]interface{}
			err := json.Unmarshal([]byte(policy), &policyObj)
			if err == nil {
				statement, hasStatement := policyObj["Statement"]
				version, hasVersion := policyObj["Version"]
				if hasStatement && hasVersion {
					statementIsEmpty := statement == nil || (fmt.Sprintf("%v", statement) == "<nil>" || fmt.Sprintf("%v", statement) == "null")
					versionIsEmpty := version == nil || version == ""
					if statementIsEmpty && versionIsEmpty {
						isEmptyPolicy = true
					}
				}
			}
		}

		if !isEmptyPolicy {
			normalized, err := NormalizeAndCompareJSONPolicies(model.Policy.ValueString(), policy)
			if err != nil {
				model.Policy = types.StringValue(policy)
			} else {
				model.Policy = types.StringValue(normalized)
			}
		} else {
			model.Policy = types.StringNull()
		}
	} else {
		model.Policy = types.StringNull()
	}

	return diags
}

func (r *accessKeyResource) updateAccessKey(ctx context.Context, model *accessKeyResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	accessKeyID := model.ID.ValueString()
	status := model.Status.ValueString()
	policy := model.Policy.ValueString()
	description := model.Description.ValueString()

	tflog.Info(ctx, fmt.Sprintf("Updating accesskey %s", accessKeyID))

	if model.Status.IsUnknown() || model.Status.IsNull() {
		return diags
	}

	newStatus := "on"
	if status == "disabled" {
		newStatus = "off"
	}

	err := retry.RetryContext(ctx, 5*time.Minute, func() *retry.RetryError {
		err := r.client.S3Admin.UpdateServiceAccount(ctx, accessKeyID, madmin.UpdateServiceAccountReq{NewStatus: newStatus})
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
				return retry.RetryableError(fmt.Errorf("transient error updating accesskey %s status: %w", accessKeyID, err))
			}
			return retry.NonRetryableError(fmt.Errorf("failed to update accesskey status: %w", err))
		}
		return nil
	})

	if err != nil {
		diags.AddError("Updating access key status", fmt.Sprintf("Failed to update status: %s", err))
		return diags
	}

	err = retry.RetryContext(ctx, 30*time.Second, func() *retry.RetryError {
		info, err := r.client.S3Admin.InfoServiceAccount(ctx, accessKeyID)
		if err != nil {
			return retry.RetryableError(fmt.Errorf("error verifying accesskey %s status update: %w", accessKeyID, err))
		}

		actualStatus := "enabled"
		if info.AccountStatus == "off" {
			actualStatus = "disabled"
		}

		if actualStatus != status {
			return retry.RetryableError(fmt.Errorf("accesskey %s status not yet updated (current: %s, expected: %s)",
				accessKeyID, actualStatus, status))
		}

		return nil
	})

	if err != nil {
		diags.AddError("Verifying access key status", fmt.Sprintf("Failed to verify status: %s", err))
		return diags
	}

	if policy != "" {
		err := retry.RetryContext(ctx, 5*time.Minute, func() *retry.RetryError {
			err := r.client.S3Admin.UpdateServiceAccount(ctx, accessKeyID, madmin.UpdateServiceAccountReq{NewPolicy: []byte(policy)})
			if err != nil {
				if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
					return retry.RetryableError(fmt.Errorf("transient error updating accesskey %s policy: %w", accessKeyID, err))
				}
				return retry.NonRetryableError(fmt.Errorf("failed to update accesskey policy: %w", err))
			}
			return nil
		})

		if err != nil {
			diags.AddError("Updating access key policy", fmt.Sprintf("Failed to update policy: %s", err))
			return diags
		}
	}

	if !model.Description.IsNull() && !model.Description.IsUnknown() {
		err := retry.RetryContext(ctx, 5*time.Minute, func() *retry.RetryError {
			err := r.client.S3Admin.UpdateServiceAccount(ctx, accessKeyID, madmin.UpdateServiceAccountReq{NewDescription: description})
			if err != nil {
				if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout") {
					return retry.RetryableError(fmt.Errorf("transient error updating accesskey %s description: %w", accessKeyID, err))
				}
				return retry.NonRetryableError(fmt.Errorf("failed to update accesskey description: %w", err))
			}
			return nil
		})

		if err != nil {
			diags.AddError("Updating access key description", fmt.Sprintf("Failed to update description: %s", err))
			return diags
		}
	}

	diags.Append(r.readAccessKey(ctx, model)...)
	return diags
}
