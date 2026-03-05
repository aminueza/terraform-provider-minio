package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioLicenseInfo() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieves MinIO license information.",
		Read:        dataSourceMinioLicenseInfoRead,
		Schema: map[string]*schema.Schema{
			"license_id":   {Type: schema.TypeString, Computed: true},
			"organization": {Type: schema.TypeString, Computed: true},
			"plan":         {Type: schema.TypeString, Computed: true},
			"issued_at":    {Type: schema.TypeString, Computed: true},
			"expires_at":   {Type: schema.TypeString, Computed: true},
			"trial":        {Type: schema.TypeBool, Computed: true},
		},
	}
}

func dataSourceMinioLicenseInfoRead(d *schema.ResourceData, meta interface{}) error {
	admin := meta.(*S3MinioClient).S3Admin

	info, err := admin.GetLicenseInfo(context.Background())
	if err != nil {
		d.SetId("unlicensed")
		_ = d.Set("plan", "")
		return nil
	}

	d.SetId(info.ID)
	_ = d.Set("license_id", info.ID)
	_ = d.Set("organization", info.Organization)
	_ = d.Set("plan", info.Plan)
	_ = d.Set("issued_at", info.IssuedAt.String())
	_ = d.Set("expires_at", info.ExpiresAt.String())
	_ = d.Set("trial", info.Trial)

	return nil
}
