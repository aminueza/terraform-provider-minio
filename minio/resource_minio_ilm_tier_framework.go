package minio

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/minio/madmin-go/v3"
)

var (
	_ resource.Resource                = &ilmTierResource{}
	_ resource.ResourceWithConfigure   = &ilmTierResource{}
	_ resource.ResourceWithImportState = &ilmTierResource{}
)

type ilmTierResource struct {
	client *S3MinioClient
}

type ilmTierResourceModel struct {
	ID                  types.String         `tfsdk:"id"`
	Name                types.String         `tfsdk:"name"`
	Prefix              types.String         `tfsdk:"prefix"`
	Bucket              types.String         `tfsdk:"bucket"`
	Type                types.String         `tfsdk:"type"`
	Endpoint            types.String         `tfsdk:"endpoint"`
	Region              types.String         `tfsdk:"region"`
	ForceNewCredentials types.Bool           `tfsdk:"force_new_credentials"`
	MinioConfig         []ilmTierMinioConfig `tfsdk:"minio_config"`
	S3Config            []ilmTierS3Config    `tfsdk:"s3_config"`
	AzureConfig         []ilmTierAzureConfig `tfsdk:"azure_config"`
	GCSConfig           []ilmTierGCSConfig   `tfsdk:"gcs_config"`
}

type ilmTierMinioConfig struct {
	AccessKey types.String `tfsdk:"access_key"`
	SecretKey types.String `tfsdk:"secret_key"`
}

type ilmTierS3Config struct {
	AccessKey    types.String `tfsdk:"access_key"`
	SecretKey    types.String `tfsdk:"secret_key"`
	StorageClass types.String `tfsdk:"storage_class"`
}

type ilmTierAzureConfig struct {
	AccountName  types.String `tfsdk:"account_name"`
	AccountKey   types.String `tfsdk:"account_key"`
	StorageClass types.String `tfsdk:"storage_class"`
}

type ilmTierGCSConfig struct {
	Credentials  types.String `tfsdk:"credentials"`
	StorageClass types.String `tfsdk:"storage_class"`
}

func newILMTierResource() resource.Resource {
	return &ilmTierResource{}
}

func (r *ilmTierResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ilm_tier"
}

func (r *ilmTierResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages remote storage tiers for MinIO ILM (Information Lifecycle Management). Tiers allow transitioning objects to cheaper remote storage (S3, GCS, Azure, or another MinIO deployment) based on lifecycle rules.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Tier name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique name for this tier (e.g., S3TIER, GCSTIER). Must be uppercase.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`^[A-Z0-9_-]+$`), "must be uppercase alphanumeric, hyphens, or underscores"),
				},
			},
			"prefix": schema.StringAttribute{
				Optional:    true,
				Description: "Object name prefix to use on the remote tier bucket.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bucket": schema.StringAttribute{
				Required:    true,
				Description: "Bucket name on the remote storage target.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required:    true,
				Description: "Remote storage type: s3, minio, gcs, or azure.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("s3", "minio", "gcs", "azure"),
				},
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Endpoint URL for the remote storage. Required for s3 and minio types.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				Optional:    true,
				Description: "Region of the remote storage bucket.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"force_new_credentials": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Force credential update even when the server returns REDACTED values.",
				Default:     booldefault.StaticBool(false),
			},
			"minio_config": schema.ListAttribute{
				Optional:    true,
				Description: "Configuration for MinIO remote tier. Required when type is minio.",
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"access_key": types.StringType,
						"secret_key": types.StringType,
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"s3_config": schema.ListAttribute{
				Optional:    true,
				Description: "Configuration for S3 remote tier. Required when type is s3.",
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"access_key":    types.StringType,
						"secret_key":    types.StringType,
						"storage_class": types.StringType,
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"azure_config": schema.ListAttribute{
				Optional:    true,
				Description: "Configuration for Azure Blob Storage remote tier. Required when type is azure.",
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"account_name":  types.StringType,
						"account_key":   types.StringType,
						"storage_class": types.StringType,
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"gcs_config": schema.ListAttribute{
				Optional:    true,
				Description: "Configuration for Google Cloud Storage remote tier. Required when type is gcs.",
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"credentials":   types.StringType,
						"storage_class": types.StringType,
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *ilmTierResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ilmTierResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ilmTierResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.createILMTier(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error creating ILM tier",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ilmTierResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ilmTierResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readILMTier(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error reading ILM tier",
			err.Error(),
		)
		return
	}

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ilmTierResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ilmTierResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.updateILMTier(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error updating ILM tier",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ilmTierResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ilmTierResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.deleteILMTier(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting ILM tier",
			err.Error(),
		)
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *ilmTierResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *ilmTierResource) createILMTier(ctx context.Context, data *ilmTierResourceModel) error {
	c := r.client.S3Admin

	var tierConf *madmin.TierConfig
	var err error

	switch data.Type.ValueString() {
	case "s3":
		tierConf, err = r.createS3Tier(data)
	case "minio":
		tierConf, err = r.createMinioTier(data)
	case "gcs":
		tierConf, err = r.createGCSTier(data)
	case "azure":
		tierConf, err = r.createAzureTier(data)
	default:
		return fmt.Errorf("unsupported tier type: %s", data.Type.ValueString())
	}

	if err != nil {
		return fmt.Errorf("creating tier configuration: %w", err)
	}

	if err := c.AddTier(ctx, tierConf); err != nil {
		return fmt.Errorf("adding remote tier: %w", err)
	}

	data.ID = data.Name

	return r.readILMTier(ctx, data)
}

func (r *ilmTierResource) createS3Tier(data *ilmTierResourceModel) (*madmin.TierConfig, error) {
	if len(data.S3Config) == 0 {
		return nil, fmt.Errorf("s3_config is required when type is s3")
	}

	s3Cfg := data.S3Config[0]
	var s3Options []madmin.S3Options

	if !data.Prefix.IsNull() && data.Prefix.ValueString() != "" {
		s3Options = append(s3Options, madmin.S3Prefix(data.Prefix.ValueString()))
	}
	if !data.Region.IsNull() && data.Region.ValueString() != "" {
		s3Options = append(s3Options, madmin.S3Region(data.Region.ValueString()))
	}
	if !s3Cfg.StorageClass.IsNull() && s3Cfg.StorageClass.ValueString() != "" {
		s3Options = append(s3Options, madmin.S3StorageClass(s3Cfg.StorageClass.ValueString()))
	}
	if !data.Endpoint.IsNull() && data.Endpoint.ValueString() != "" {
		s3Options = append(s3Options, madmin.S3Endpoint(data.Endpoint.ValueString()))
	}

	return madmin.NewTierS3(
		data.Name.ValueString(),
		s3Cfg.AccessKey.ValueString(),
		s3Cfg.SecretKey.ValueString(),
		data.Bucket.ValueString(),
		s3Options...,
	)
}

func (r *ilmTierResource) createMinioTier(data *ilmTierResourceModel) (*madmin.TierConfig, error) {
	if len(data.MinioConfig) == 0 {
		return nil, fmt.Errorf("minio_config is required when type is minio")
	}

	minioCfg := data.MinioConfig[0]
	var minioOptions []madmin.MinIOOptions

	if !data.Prefix.IsNull() && data.Prefix.ValueString() != "" {
		minioOptions = append(minioOptions, madmin.MinIOPrefix(data.Prefix.ValueString()))
	}
	if !data.Region.IsNull() && data.Region.ValueString() != "" {
		minioOptions = append(minioOptions, madmin.MinIORegion(data.Region.ValueString()))
	}

	return madmin.NewTierMinIO(
		data.Name.ValueString(),
		data.Endpoint.ValueString(),
		minioCfg.AccessKey.ValueString(),
		minioCfg.SecretKey.ValueString(),
		data.Bucket.ValueString(),
		minioOptions...,
	)
}

func (r *ilmTierResource) createGCSTier(data *ilmTierResourceModel) (*madmin.TierConfig, error) {
	if len(data.GCSConfig) == 0 {
		return nil, fmt.Errorf("gcs_config is required when type is gcs")
	}

	gcsCfg := data.GCSConfig[0]
	var gcsOptions []madmin.GCSOptions

	if !data.Prefix.IsNull() && data.Prefix.ValueString() != "" {
		gcsOptions = append(gcsOptions, madmin.GCSPrefix(data.Prefix.ValueString()))
	}
	if !data.Region.IsNull() && data.Region.ValueString() != "" {
		gcsOptions = append(gcsOptions, madmin.GCSRegion(data.Region.ValueString()))
	}
	if !gcsCfg.StorageClass.IsNull() && gcsCfg.StorageClass.ValueString() != "" {
		gcsOptions = append(gcsOptions, madmin.GCSStorageClass(gcsCfg.StorageClass.ValueString()))
	}

	return madmin.NewTierGCS(
		data.Name.ValueString(),
		[]byte(gcsCfg.Credentials.ValueString()),
		data.Bucket.ValueString(),
		gcsOptions...,
	)
}

func (r *ilmTierResource) createAzureTier(data *ilmTierResourceModel) (*madmin.TierConfig, error) {
	if len(data.AzureConfig) == 0 {
		return nil, fmt.Errorf("azure_config is required when type is azure")
	}

	azureCfg := data.AzureConfig[0]
	var azureOptions []madmin.AzureOptions

	if !data.Endpoint.IsNull() && data.Endpoint.ValueString() != "" {
		azureOptions = append(azureOptions, madmin.AzureEndpoint(data.Endpoint.ValueString()))
	}
	if !data.Prefix.IsNull() && data.Prefix.ValueString() != "" {
		azureOptions = append(azureOptions, madmin.AzurePrefix(data.Prefix.ValueString()))
	}
	if !data.Region.IsNull() && data.Region.ValueString() != "" {
		azureOptions = append(azureOptions, madmin.AzureRegion(data.Region.ValueString()))
	}
	if !azureCfg.StorageClass.IsNull() && azureCfg.StorageClass.ValueString() != "" {
		azureOptions = append(azureOptions, madmin.AzureStorageClass(azureCfg.StorageClass.ValueString()))
	}

	return madmin.NewTierAzure(
		data.Name.ValueString(),
		azureCfg.AccountName.ValueString(),
		azureCfg.AccountKey.ValueString(),
		data.Bucket.ValueString(),
		azureOptions...,
	)
}

func (r *ilmTierResource) readILMTier(ctx context.Context, data *ilmTierResourceModel) error {
	c := r.client.S3Admin
	name := data.ID.ValueString()

	tier, err := r.getTier(c, ctx, name)
	if err != nil {
		return fmt.Errorf("reading tier: %w", err)
	}

	if tier == nil {
		data.ID = types.StringNull()
		return nil
	}

	data.ID = types.StringValue(tier.Name)
	data.Name = types.StringValue(tier.Name)
	data.Type = types.StringValue(tier.Type.String())

	prefix := tier.Prefix()
	if prefix != "" {
		data.Prefix = types.StringValue(prefix)
	} else {
		data.Prefix = types.StringNull()
	}

	data.Bucket = types.StringValue(tier.Bucket())

	endpoint := tier.Endpoint()
	if endpoint != "" {
		data.Endpoint = types.StringValue(endpoint)
	} else {
		data.Endpoint = types.StringNull()
	}

	region := tier.Region()
	if region != "" {
		data.Region = types.StringValue(region)
	} else {
		data.Region = types.StringNull()
	}

	switch tier.Type {
	case madmin.MinIO:
		data.MinioConfig = []ilmTierMinioConfig{{
			AccessKey: types.StringValue(tier.MinIO.AccessKey),
			SecretKey: types.StringValue(tier.MinIO.SecretKey),
		}}
	case madmin.GCS:
		data.GCSConfig = []ilmTierGCSConfig{{
			Credentials:  types.StringValue(tier.GCS.Creds),
			StorageClass: getStringOrNull(tier.GCS.StorageClass),
		}}
	case madmin.Azure:
		data.AzureConfig = []ilmTierAzureConfig{{
			AccountName:  types.StringValue(tier.Azure.AccountName),
			AccountKey:   types.StringValue(tier.Azure.AccountKey),
			StorageClass: getStringOrNull(tier.Azure.StorageClass),
		}}
	case madmin.S3:
		data.S3Config = []ilmTierS3Config{{
			AccessKey:    types.StringValue(tier.S3.AccessKey),
			SecretKey:    types.StringValue(tier.S3.SecretKey),
			StorageClass: getStringOrNull(tier.S3.StorageClass),
		}}
	}

	return nil
}

func (r *ilmTierResource) updateILMTier(ctx context.Context, data *ilmTierResourceModel) error {
	c := r.client.S3Admin

	credentials := madmin.TierCreds{}

	switch data.Type.ValueString() {
	case "minio":
		if len(data.MinioConfig) > 0 {
			credentials.AccessKey = data.MinioConfig[0].AccessKey.ValueString()
			credentials.SecretKey = data.MinioConfig[0].SecretKey.ValueString()
		}
	case "gcs":
		if len(data.GCSConfig) > 0 {
			credentials.CredsJSON = []byte(data.GCSConfig[0].Credentials.ValueString())
		}
	case "azure":
		if len(data.AzureConfig) > 0 {
			credentials.SecretKey = data.AzureConfig[0].AccountKey.ValueString()
		}
	case "s3":
		if len(data.S3Config) > 0 {
			credentials.AccessKey = data.S3Config[0].AccessKey.ValueString()
			credentials.SecretKey = data.S3Config[0].SecretKey.ValueString()
		}
	}

	if err := c.EditTier(ctx, data.Name.ValueString(), credentials); err != nil {
		return fmt.Errorf("updating tier: %w", err)
	}

	return r.readILMTier(ctx, data)
}

func (r *ilmTierResource) deleteILMTier(ctx context.Context, data *ilmTierResourceModel) error {
	c := r.client.S3Admin

	if err := c.RemoveTier(ctx, data.Name.ValueString()); err != nil {
		errMsg := err.Error()
		if contains(errMsg, "not found") || contains(errMsg, "does not exist") {
			return nil
		}
		return fmt.Errorf("deleting tier: %w", err)
	}

	return nil
}

func (r *ilmTierResource) getTier(client *madmin.AdminClient, ctx context.Context, name string) (*madmin.TierConfig, error) {
	tiers, err := client.ListTiers(ctx)
	if err != nil {
		return nil, err
	}
	for _, tier := range tiers {
		if tier.Name == name {
			return tier, nil
		}
	}
	return nil, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
