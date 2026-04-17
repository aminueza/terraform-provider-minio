package minio

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	minio "github.com/minio/minio-go/v7"
)

// Ensure provider defined types fully satisfy framework interfaces
var (
	_ resource.Resource                = &bucketVersioningResource{}
	_ resource.ResourceWithConfigure   = &bucketVersioningResource{}
	_ resource.ResourceWithImportState = &bucketVersioningResource{}
)

// bucketVersioningResource defines the resource implementation
type bucketVersioningResource struct {
	client *S3MinioClient
}

// bucketVersioningResourceModel describes the resource data model
type bucketVersioningResourceModel struct {
	ID     types.String                        `tfsdk:"id"`
	Bucket types.String                        `tfsdk:"bucket"`
	Config *bucketVersioningConfigurationModel `tfsdk:"versioning_configuration"`
}

// bucketVersioningConfigurationModel describes the versioning configuration model
type bucketVersioningConfigurationModel struct {
	Status           types.String `tfsdk:"status"`
	ExcludedPrefixes types.List   `tfsdk:"excluded_prefixes"`
	ExcludeFolders   types.Bool   `tfsdk:"exclude_folders"`
}

func (r *bucketVersioningResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket_versioning"
}

func (r *bucketVersioningResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a MinIO S3 Bucket Versioning resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Bucket name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Description: "Name of the bucket.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"versioning_configuration": schema.SingleNestedAttribute{
				Description: "Versioning configuration for the bucket.",
				Required:    true,
				Attributes: map[string]schema.Attribute{
					"status": schema.StringAttribute{
						Description: "Versioning status: Enabled or Suspended.",
						Required:    true,
						Validators: []validator.String{
							stringvalidator.OneOf(minio.Enabled, minio.Suspended),
						},
					},
					"excluded_prefixes": schema.ListAttribute{
						Description: "List of object key prefixes to exclude from versioning.",
						Optional:    true,
						Computed:    true,
						ElementType: types.StringType,
						PlanModifiers: []planmodifier.List{
							listplanmodifier.UseStateForUnknown(),
						},
					},
					"exclude_folders": schema.BoolAttribute{
						Description: "Whether to exclude folders from versioning.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *bucketVersioningResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *bucketVersioningResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data bucketVersioningResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.putVersioning(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = data.Bucket

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketVersioningResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data bucketVersioningResourceModel

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

func (r *bucketVersioningResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data bucketVersioningResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.putVersioning(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.read(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketVersioningResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data bucketVersioningResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if already suspended
	if data.Config != nil && !data.Config.Status.IsNull() {
		if data.Config.Status.ValueString() == minio.Suspended {
			return
		}
	}

	// Suspend versioning
	err := r.client.S3Client.SuspendVersioning(ctx, data.Bucket.ValueString())
	if err != nil {
		if isNoSuchBucketError(err) {
			// Bucket doesn't exist, nothing to delete
			return
		}
		var minioErr minio.ErrorResponse
		if errors.As(err, &minioErr) && minioErr.Code == "InvalidBucketState" {
			return
		}
		resp.Diagnostics.AddError(
			"Error suspending bucket versioning",
			err.Error(),
		)
		return
	}

	// Clear ID
	data.ID = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *bucketVersioningResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("bucket"), req, resp)
}

func (r *bucketVersioningResource) putVersioning(ctx context.Context, data *bucketVersioningResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	if data.Config == nil {
		return diags
	}

	versioningConfig := convertFrameworkVersioningConfig(data.Config)

	// Wait for bucket to be ready
	timeout := 5 * time.Minute
	waitTimeout := timeout - 30*time.Second
	if waitTimeout < 30*time.Second {
		waitTimeout = 30 * time.Second
	}

	if err := waitForBucketReadyFramework(ctx, r.client.S3Client, data.Bucket.ValueString(), waitTimeout); err != nil {
		diags.AddError(
			"Error waiting for bucket to be ready",
			err.Error(),
		)
		return diags
	}

	// Retry SetBucketVersioning for transient NoSuchBucket errors
	err := retry.RetryContext(ctx, waitTimeout, func() *retry.RetryError {
		err := r.client.S3Client.SetBucketVersioning(
			ctx,
			data.Bucket.ValueString(),
			versioningConfig,
		)
		if err != nil {
			if isNoSuchBucketError(err) {
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		diags.AddError(
			"Error putting bucket versioning configuration",
			err.Error(),
		)
		return diags
	}

	// Handle policy preservation
	policyBefore, _ := r.client.S3Client.GetBucketPolicy(ctx, data.Bucket.ValueString())
	if strings.TrimSpace(policyBefore) != "" && strings.TrimSpace(policyBefore) != "{}" {
		time.Sleep(500 * time.Millisecond)
		policyAfter, _ := r.client.S3Client.GetBucketPolicy(ctx, data.Bucket.ValueString())
		if strings.TrimSpace(policyAfter) == "" || strings.TrimSpace(policyAfter) == "{}" {
			_ = r.client.S3Client.SetBucketPolicy(ctx, data.Bucket.ValueString(), policyBefore)
		}
	}

	return diags
}

func (r *bucketVersioningResource) read(ctx context.Context, data *bucketVersioningResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	// For new resources, wait for bucket to be ready
	if data.ID.IsNull() {
		timeout := 2 * time.Minute
		if err := waitForBucketReadyFramework(ctx, r.client.S3Client, data.Bucket.ValueString(), timeout); err != nil {
			if isNoSuchBucketError(err) {
				// Bucket doesn't exist, but keep ID for state consistency
				data.ID = data.Bucket
				return diags
			}
			diags.AddError(
				"Error waiting for bucket to be ready",
				err.Error(),
			)
			return diags
		}
	} else {
		// For existing resources, check if bucket exists
		exists, err := r.client.S3Client.BucketExists(ctx, data.Bucket.ValueString())
		if err != nil {
			diags.AddError(
				"Error checking bucket existence",
				err.Error(),
			)
			return diags
		}
		if !exists {
			// Bucket doesn't exist, but keep ID for state consistency
			data.ID = data.Bucket
			return diags
		}
	}

	// Get versioning configuration
	cfg, readErr := r.client.S3Client.GetBucketVersioning(ctx, data.Bucket.ValueString())
	if readErr != nil {
		diags.AddError(
			"Error loading bucket versioning",
			readErr.Error(),
		)
		return diags
	}

	versioningConfig := cfg

	if versioningConfig.Status == "" {
		retryTimeout := 5 * time.Second
		retryErr := retry.RetryContext(ctx, retryTimeout, func() *retry.RetryError {
			cfg, err := r.client.S3Client.GetBucketVersioning(ctx, data.Bucket.ValueString())
			if err != nil {
				if isNoSuchBucketError(err) && data.ID.IsNull() {
					return retry.RetryableError(err)
				}
				return retry.NonRetryableError(err)
			}
			if cfg.Status == "" {
				return retry.RetryableError(fmt.Errorf("versioning status not yet available for bucket %s", data.Bucket.ValueString()))
			}
			versioningConfig = cfg
			return nil
		})
		if retryErr != nil && data.ID.IsNull() {
			diags.AddError(
				"Error loading bucket versioning",
				retryErr.Error(),
			)
			return diags
		}
	}

	// Build configuration model
	var excludedPrefixes []string
	for _, prefix := range versioningConfig.ExcludedPrefixes {
		excludedPrefixes = append(excludedPrefixes, prefix.Prefix)
	}

	excludedPrefixesList, diagErr := types.ListValueFrom(ctx, types.StringType, excludedPrefixes)
	if diagErr.HasError() {
		diags.Append(diagErr...)
		return diags
	}

	// Ensure excluded_prefixes is empty list, not null, when server returns no prefixes
	if excludedPrefixesList.IsNull() || excludedPrefixesList.IsUnknown() {
		excludedPrefixesList, diagErr = types.ListValueFrom(ctx, types.StringType, []string{})
		if diagErr.HasError() {
			diags.Append(diagErr...)
			return diags
		}
	}

	newConfig := bucketVersioningConfigurationModel{
		Status:           types.StringValue(versioningConfig.Status),
		ExcludedPrefixes: excludedPrefixesList,
		ExcludeFolders:   types.BoolValue(versioningConfig.ExcludeFolders),
	}

	data.Config = &newConfig
	if data.ID.IsNull() {
		data.ID = data.Bucket
	}

	return diags
}

// convertFrameworkVersioningConfig converts framework config to minio config
func convertFrameworkVersioningConfig(c *bucketVersioningConfigurationModel) minio.BucketVersioningConfiguration {
	if c == nil {
		return minio.BucketVersioningConfiguration{}
	}
	conf := minio.BucketVersioningConfiguration{
		Status:         c.Status.ValueString(),
		ExcludeFolders: c.ExcludeFolders.ValueBool(),
	}

	var excludedPrefixes []string
	c.ExcludedPrefixes.ElementsAs(context.Background(), &excludedPrefixes, false)
	for _, prefix := range excludedPrefixes {
		conf.ExcludedPrefixes = append(conf.ExcludedPrefixes, minio.ExcludedPrefix{Prefix: prefix})
	}

	return conf
}

// newBucketVersioningResource creates a new bucket versioning resource instance
func newBucketVersioningResource() resource.Resource {
	return &bucketVersioningResource{}
}
