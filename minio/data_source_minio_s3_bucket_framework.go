package minio

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &minioS3BucketDataSource{}

// minioS3BucketDataSource defines the data source implementation
type minioS3BucketDataSource struct {
	client *S3MinioClient
}

// minioS3BucketDataSourceModel describes the data source data model
type minioS3BucketDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Bucket            types.String `tfsdk:"bucket"`
	Region            types.String `tfsdk:"region"`
	VersioningEnabled types.Bool   `tfsdk:"versioning_enabled"`
	ObjectLockEnabled types.Bool   `tfsdk:"object_lock_enabled"`
	Policy            types.String `tfsdk:"policy"`
}

func newS3BucketDataSource() datasource.DataSource {
	return &minioS3BucketDataSource{}
}

// Metadata returns the data source type name
func (d *minioS3BucketDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket"
}

// Schema returns the data source schema
func (d *minioS3BucketDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads properties of an existing S3 bucket including versioning, region, and object lock status.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Bucket name (same as bucket field)",
				Computed:    true,
			},
			"bucket": schema.StringAttribute{
				Description: "Name of the bucket",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "Region of the bucket",
				Computed:    true,
			},
			"versioning_enabled": schema.BoolAttribute{
				Description: "Whether versioning is enabled for the bucket",
				Computed:    true,
			},
			"object_lock_enabled": schema.BoolAttribute{
				Description: "Whether object locking is enabled for the bucket",
				Computed:    true,
			},
			"policy": schema.StringAttribute{
				Description: "Bucket policy in JSON format",
				Computed:    true,
			},
		},
	}
}

// Configure sets up the data source
func (d *minioS3BucketDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*S3MinioClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *S3MinioClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

// Read reads the data source
func (d *minioS3BucketDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data minioS3BucketDataSourceModel

	// Read provider data
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bucket := data.Bucket.ValueString()

	// Set ID and bucket
	data.ID = types.StringValue(bucket)
	data.Bucket = types.StringValue(bucket)

	// Get region
	region, err := d.client.S3Client.GetBucketLocation(ctx, bucket)
	if err != nil {
		resp.Diagnostics.AddError("Error reading bucket region", err.Error())
		return
	}
	data.Region = types.StringValue(region)

	// Get versioning
	versioning, err := d.client.S3Client.GetBucketVersioning(ctx, bucket)
	if err == nil {
		data.VersioningEnabled = types.BoolValue(versioning.Enabled())
	} else {
		data.VersioningEnabled = types.BoolValue(false)
	}

	// Get object lock config
	lockConfig, _, _, _, err := d.client.S3Client.GetObjectLockConfig(ctx, bucket)
	if err == nil {
		data.ObjectLockEnabled = types.BoolValue(lockConfig == "Enabled")
	} else {
		data.ObjectLockEnabled = types.BoolValue(false)
	}

	// Get bucket policy
	policy, err := d.client.S3Client.GetBucketPolicy(ctx, bucket)
	if err == nil {
		data.Policy = types.StringValue(policy)
	} else {
		data.Policy = types.StringNull()
	}

	// Save data
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
