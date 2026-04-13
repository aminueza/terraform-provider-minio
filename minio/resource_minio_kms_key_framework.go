package minio

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &kmsKeyResource{}
	_ resource.ResourceWithConfigure   = &kmsKeyResource{}
	_ resource.ResourceWithImportState = &kmsKeyResource{}
)

type kmsKeyResource struct {
	client *S3MinioClient
}

type kmsKeyResourceModel struct {
	ID    types.String `tfsdk:"id"`
	KeyID types.String `tfsdk:"key_id"`
}

func newKMSKeyResource() resource.Resource {
	return &kmsKeyResource{}
}

func (r *kmsKeyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kms_key"
}

func (r *kmsKeyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a KMS key for MinIO. KMS keys are used for encryption and decryption operations.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "KMS key ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key_id": schema.StringAttribute{
				Required:    true,
				Description: "KMS key ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *kmsKeyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *kmsKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data kmsKeyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	keyID := data.KeyID.ValueString()

	if err := r.client.S3Admin.CreateKey(ctx, keyID); err != nil {
		resp.Diagnostics.AddError(
			"Error creating KMS key",
			err.Error(),
		)
		return
	}

	data.ID = types.StringValue(keyID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *kmsKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data kmsKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	status, err := r.client.S3Admin.GetKeyStatus(ctx, data.KeyID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}

	if status.EncryptionErr != "" {
		resp.Diagnostics.AddError(
			"KMS key has encryption error",
			status.EncryptionErr,
		)
		return
	}

	if status.DecryptionErr != "" {
		resp.Diagnostics.AddError(
			"KMS key has decryption error",
			status.DecryptionErr,
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *kmsKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data kmsKeyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *kmsKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data kmsKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.S3Admin.DeleteKey(ctx, data.KeyID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting KMS key",
			err.Error(),
		)
		return
	}
}

func (r *kmsKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("key_id"), req, resp)
	resp.State.SetAttribute(ctx, path.Root("id"), req.ID)
}
