package minio

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/madmin-go/v3"
)

func resourceMinioILMTier() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateILMTier,
		ReadContext:   minioReadILMTier,
		DeleteContext: minioDeleteILMTier,
		UpdateContext: minioUpdateILMTier,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Description: "Manages remote storage tiers for MinIO ILM (Information Lifecycle Management). Tiers allow transitioning objects to cheaper remote storage (S3, GCS, Azure, or another MinIO deployment) based on lifecycle rules.",
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Unique name for this tier (e.g., S3TIER, GCSTIER). Must be uppercase.",
			},
			"prefix": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Object name prefix to use on the remote tier bucket.",
			},
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Bucket name on the remote storage target.",
			},
			"type": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice([]string{"s3", "minio", "gcs", "azure"}, false),
				Description:  "Remote storage type: s3, minio, gcs, or azure.",
			},
			"endpoint": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Default:     "",
				Description: "Endpoint URL for the remote storage. Required for s3 and minio types.",
			},
			"region": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Region of the remote storage bucket.",
			},
			"force_new_credentials": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Force credential update even when the server returns REDACTED values.",
			},

			"minio_config": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: "Configuration for MinIO remote tier. Required when type is minio.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"access_key": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Access key for the remote MinIO instance.",
						},
						"secret_key": {
							Type:      schema.TypeString,
							Optional:  true,
							Sensitive: true,
							DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
								if d.Get("force_new_credentials").(bool) {
									return false
								}
								return old == "REDACTED"
							},
							Description: "Secret key for the remote MinIO instance.",
						},
					},
				},
			},
			"s3_config": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: "Configuration for S3 remote tier. Required when type is s3.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"access_key": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "AWS access key ID.",
						},
						"secret_key": {
							Type:      schema.TypeString,
							Optional:  true,
							Sensitive: true,
							DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
								if d.Get("force_new_credentials").(bool) {
									return false
								}
								return old == "REDACTED"
							},
							Description: "AWS secret access key.",
						},
						"storage_class": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "S3 storage class (e.g., STANDARD_IA, GLACIER, DEEP_ARCHIVE).",
						},
					},
				},
			},
			"azure_config": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: "Configuration for Azure Blob Storage remote tier. Required when type is azure.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"account_name": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Azure storage account name.",
						},
						"account_key": {
							Type:      schema.TypeString,
							Optional:  true,
							Sensitive: true,
							DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
								if d.Get("force_new_credentials").(bool) {
									return false
								}
								return old == "REDACTED"
							},
							Description: "Azure storage account key.",
						},
						"storage_class": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Azure storage tier (e.g., Hot, Cool, Archive).",
						},
					},
				},
			},
			"gcs_config": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: "Configuration for Google Cloud Storage remote tier. Required when type is gcs.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"credentials": {
							Type:      schema.TypeString,
							Optional:  true,
							Sensitive: true,
							DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
								if d.Get("force_new_credentials").(bool) {
									return false
								}
								return old == "REDACTED"
							},
							Description: "GCS service account credentials JSON.",
						},
						"storage_class": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "GCS storage class (e.g., NEARLINE, COLDLINE, ARCHIVE).",
						},
					},
				},
			},
		},
	}
}

func minioCreateILMTier(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var err error
	var tierConf *madmin.TierConfig
	c := meta.(*S3MinioClient).S3Admin
	name := d.Get("name").(string)
	d.SetId(name)
	switch d.Get("type").(string) {
	case madmin.S3.String():
		s3Config := d.Get("s3_config").([]interface{})[0].(map[string]interface{})
		var s3Options []madmin.S3Options
		if d.Get("prefix").(string) != "" {
			s3Options = append(s3Options, madmin.S3Prefix(d.Get("prefix").(string)))
		}
		if d.Get("region").(string) != "" {
			s3Options = append(s3Options, madmin.S3Region(d.Get("region").(string)))
		}
		if _, ok := s3Config["storage_class"]; ok {
			s3Options = append(s3Options, madmin.S3StorageClass(s3Config["storage_class"].(string)))
		}
		if d.Get("endpoint").(string) != "" {
			s3Options = append(s3Options, madmin.S3Endpoint(d.Get("endpoint").(string)))
		}
		tierConf, err = madmin.NewTierS3(
			name,
			s3Config["access_key"].(string),
			s3Config["secret_key"].(string),
			d.Get("bucket").(string),
			s3Options...,
		)
	case madmin.MinIO.String():
		minioConfig := d.Get("minio_config").([]interface{})[0].(map[string]interface{})
		var minioOptions []madmin.MinIOOptions
		if d.Get("prefix").(string) != "" {
			minioOptions = append(minioOptions, madmin.MinIOPrefix(d.Get("prefix").(string)))
		}
		if d.Get("region").(string) != "" {
			minioOptions = append(minioOptions, madmin.MinIORegion(d.Get("region").(string)))
		}

		tierConf, err = madmin.NewTierMinIO(
			name,
			d.Get("endpoint").(string),
			minioConfig["access_key"].(string),
			minioConfig["secret_key"].(string),
			d.Get("bucket").(string),
			minioOptions...,
		)
	case madmin.GCS.String():
		gcsConfigListRaw, ok := d.GetOk("gcs_config")
		if !ok {
			return NewResourceError("gcs_config is required when type is gcs", name, "missing gcs_config")
		}
		gcsConfigList := gcsConfigListRaw.([]interface{})
		if len(gcsConfigList) == 0 {
			return NewResourceError("gcs_config is required when type is gcs", name, "empty gcs_config")
		}
		gcsConfig := gcsConfigList[0].(map[string]interface{})
		var gcsOptions []madmin.GCSOptions
		if d.Get("prefix").(string) != "" {
			gcsOptions = append(gcsOptions, madmin.GCSPrefix(d.Get("prefix").(string)))
		}
		if d.Get("region").(string) != "" {
			gcsOptions = append(gcsOptions, madmin.GCSRegion(d.Get("region").(string)))
		}
		if _, ok := gcsConfig["storage_class"]; ok {
			gcsOptions = append(gcsOptions, madmin.GCSStorageClass(gcsConfig["storage_class"].(string)))
		}
		gcsCredentialsStr, _ := gcsConfig["credentials"].(string)
		tierConf, err = madmin.NewTierGCS(
			name,
			[]byte(gcsCredentialsStr),
			d.Get("bucket").(string),
			gcsOptions...,
		)
	case madmin.Azure.String():
		azureConfig := d.Get("azure_config").([]interface{})[0].(map[string]interface{})
		var azureOptions []madmin.AzureOptions
		if d.Get("endpoint").(string) != "" {
			azureOptions = append(azureOptions, madmin.AzureEndpoint(d.Get("endpoint").(string)))
		}
		if d.Get("prefix").(string) != "" {
			azureOptions = append(azureOptions, madmin.AzurePrefix(d.Get("prefix").(string)))
		}
		if d.Get("region").(string) != "" {
			azureOptions = append(azureOptions, madmin.AzureRegion(d.Get("region").(string)))
		}
		if _, ok := azureConfig["storage_class"]; ok {
			azureOptions = append(azureOptions, madmin.AzureStorageClass(azureConfig["storage_class"].(string)))
		}
		tierConf, err = madmin.NewTierAzure(name,
			azureConfig["account_name"].(string),
			azureConfig["account_key"].(string),
			d.Get("bucket").(string),
			azureOptions...,
		)
	}
	if err != nil {
		return NewResourceError("creating remote tier failed", name, err)
	}
	err = c.AddTier(ctx, tierConf)
	if err != nil {
		return NewResourceError("adding remote tier failed", name, err)
	}
	log.Printf("[DEBUG] Created Tier %s", name)
	return minioReadILMTier(ctx, d, meta)
}

func minioReadILMTier(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Admin
	name := d.Id()
	tier, err := getTier(c, ctx, name)
	if err != nil {
		return NewResourceError("reading remote tier failed", name, err)
	}
	if tier == nil {
		log.Printf("%s", NewResourceErrorStr("unable to find tier", name, err))
		d.SetId("")
		return nil
	}
	log.Printf("[DEBUG] Tier [%s] exists!", name)
	d.SetId(tier.Name)
	if err := d.Set("type", tier.Type.String()); err != nil {
		return NewResourceError("setting type", name, err)
	}
	if err := d.Set("prefix", tier.Prefix()); err != nil {
		return NewResourceError("setting prefix", name, err)
	}
	if err := d.Set("name", tier.Name); err != nil {
		return NewResourceError("setting name", name, err)
	}
	if err := d.Set("bucket", tier.Bucket()); err != nil {
		return NewResourceError("setting bucket", name, err)
	}
	if err := d.Set("endpoint", tier.Endpoint()); err != nil {
		return NewResourceError("setting endpoint", name, err)
	}
	if err := d.Set("region", tier.Region()); err != nil {
		return NewResourceError("setting region", name, err)
	}
	switch tier.Type {
	case madmin.MinIO:
		minioConfig := []map[string]string{{
			"access_key": tier.MinIO.AccessKey,
			"secret_key": tier.MinIO.SecretKey,
		}}
		if err := d.Set("minio_config", minioConfig); err != nil {
			return NewResourceError("setting minio_config", name, err)
		}
	case madmin.GCS:
		gcsConfig := []map[string]string{{
			"credentials":   tier.GCS.Creds,
			"storage_class": tier.GCS.StorageClass,
		}}
		if err := d.Set("gcs_config", gcsConfig); err != nil {
			return NewResourceError("setting gcs_config", name, err)
		}
	case madmin.Azure:
		azureConfig := []map[string]string{{
			"account_name":  tier.Azure.AccountName,
			"account_key":   tier.Azure.AccountKey,
			"storage_class": tier.Azure.StorageClass,
		}}
		if err := d.Set("azure_config", azureConfig); err != nil {
			return NewResourceError("setting azure_config", name, err)
		}
	case madmin.S3:
		s3Config := []map[string]string{{
			"access_key":    tier.S3.AccessKey,
			"secret_key":    tier.S3.SecretKey,
			"storage_class": tier.S3.StorageClass,
		}}
		if err := d.Set("s3_config", s3Config); err != nil {
			return NewResourceError("setting s3_config", name, err)
		}
	}

	return nil
}

func minioDeleteILMTier(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Admin
	err := c.RemoveTier(ctx, d.Get("name").(string))
	if err != nil {
		return NewResourceError("deleting remote tier failed", d.Id(), err)
	}
	return nil
}

func minioUpdateILMTier(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*S3MinioClient).S3Admin
	name := d.Get("name").(string)
	credentials := madmin.TierCreds{}
	switch d.Get("type").(string) {
	case madmin.MinIO.String():
		minioConfig := d.Get("minio_config").([]interface{})[0].(map[string]interface{})
		credentials.AccessKey = minioConfig["access_key"].(string)
		credentials.SecretKey = minioConfig["secret_key"].(string)
	case madmin.GCS.String():
		gcsConfigListRaw, ok := d.GetOk("gcs_config")
		if !ok {
			return NewResourceError("gcs_config is required when type is gcs", name, "missing gcs_config")
		}
		gcsConfigList := gcsConfigListRaw.([]interface{})
		if len(gcsConfigList) == 0 {
			return NewResourceError("gcs_config is required when type is gcs", name, "empty gcs_config")
		}
		gcsConfig := gcsConfigList[0].(map[string]interface{})
		credentials.CredsJSON = []byte(gcsConfig["credentials"].(string))
	case madmin.Azure.String():
		azureConfig := d.Get("azure_config").([]interface{})[0].(map[string]interface{})
		credentials.SecretKey = azureConfig["account_key"].(string)
	case madmin.S3.String():
		minioConfig := d.Get("s3_config").([]interface{})[0].(map[string]interface{})
		credentials.AccessKey = minioConfig["access_key"].(string)
		credentials.SecretKey = minioConfig["secret_key"].(string)
	}
	if d.HasChanges("minio_config", "gcs_config", "azure_config", "s3_config") {
		err := c.EditTier(ctx, name, credentials)
		if err != nil {
			return NewResourceError("error updating ILM tier %s: %s", d.Id(), err)
		}
	}
	return minioReadILMTier(ctx, d, meta)
}

func getTier(client *madmin.AdminClient, ctx context.Context, name string) (*madmin.TierConfig, error) {
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
