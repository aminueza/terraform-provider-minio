package minio

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/madmin-go/v3"
	"log"
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
		Description: "`minio_ilm_tier` handles remote tiers",
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"prefix": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"bucket": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"type": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice([]string{"s3", "minio", "gcs", "azure"}, false),
			},
			"endpoint": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "",
			},
			"region": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"force_new_credentials": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"minio_config": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"access_key": {
							Type:     schema.TypeString,
							Optional: true,
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
						},
					},
				},
			},
			"s3_config": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"access_key": {
							Type:     schema.TypeString,
							Optional: true,
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
						},
						"storage_class": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
						},
					},
				},
			},
			"azure_config": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"container": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
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
						},
					},
				},
			},
			"gcs_config": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
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
		tierConf, err = madmin.NewTierS3(
			name,
			d.Get("access_key").(string),
			d.Get("secret_key").(string),
			d.Get("bucket").(string),
		)
	case madmin.MinIO.String():
		minioConfig := d.Get("minio_config").([]interface{})[0].(map[string]interface{})
		tierConf, err = madmin.NewTierMinIO(
			name,
			d.Get("endpoint").(string),
			minioConfig["access_key"].(string),
			minioConfig["secret_key"].(string),
			d.Get("bucket").(string),
		)
	case madmin.GCS.String():
		tierConf, err = madmin.NewTierGCS(
			name,
			d.Get("credentials").([]byte),
			d.Get("bucket").(string),
		)
	case madmin.Azure.String():
		tierConf, err = madmin.NewTierAzure(name,
			d.Get("account_name").(string),
			d.Get("account_key").(string),
			d.Get("bucket").(string),
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
	name := d.Get("name").(string)
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
	if err := d.Set("type", tier.Type.String()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("prefix", tier.Prefix()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("name", tier.Name); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("bucket", tier.Bucket()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("endpoint", tier.Endpoint()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("region", tier.Region()); err != nil {
		return diag.FromErr(err)
	}
	switch tier.Type {
	case madmin.MinIO:
		minioConfig := []map[string]string{{
			"access_key": tier.MinIO.AccessKey,
			"secret_key": tier.MinIO.SecretKey,
		}}
		if err := d.Set("minio_config", minioConfig); err != nil {
			return diag.FromErr(err)
		}
	case madmin.GCS:
		gcsConfig := []map[string]string{{
			"credentials": tier.GCS.Creds,
		}}
		if err := d.Set("gcs_config", gcsConfig); err != nil {
			return diag.FromErr(err)
		}
	case madmin.Azure:
		azureConfig := []map[string]string{{
			"container":   tier.Azure.AccountName,
			"account_key": tier.Azure.AccountKey,
		}}
		if err := d.Set("azure_config", azureConfig); err != nil {
			return diag.FromErr(err)
		}
	case madmin.S3:
		s3Config := []map[string]string{{
			"access_key":    tier.S3.AccessKey,
			"secret_key":    tier.S3.SecretKey,
			"storage_class": tier.S3.StorageClass,
		}}
		if err := d.Set("s3_config", s3Config); err != nil {
			return diag.FromErr(err)
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
		gcsConfig := d.Get("gcs_config").([]interface{})[0].(map[string]interface{})
		credentials.CredsJSON = gcsConfig["credentials"].([]byte)
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
