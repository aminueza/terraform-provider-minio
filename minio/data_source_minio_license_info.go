package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioLicenseInfo() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieves MinIO license information.",
		ReadContext: dataSourceMinioLicenseInfoRead,
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

func dataSourceMinioLicenseInfoRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	tflog.Debug(ctx, "Reading license info")

	info, err := admin.GetLicenseInfo(ctx)
	if err != nil {
		// Servers without a license subsystem (community MinIO) report the
		// unlicensed state instead of failing the read.
		d.SetId("unlicensed")
		if err := d.Set("plan", ""); err != nil {
			return NewResourceError("setting plan", d.Id(), err)
		}
		return nil
	}

	d.SetId(info.ID)
	if err := d.Set("license_id", info.ID); err != nil {
		return NewResourceError("setting license_id", d.Id(), err)
	}
	if err := d.Set("organization", info.Organization); err != nil {
		return NewResourceError("setting organization", d.Id(), err)
	}
	if err := d.Set("plan", info.Plan); err != nil {
		return NewResourceError("setting plan", d.Id(), err)
	}
	if err := d.Set("issued_at", info.IssuedAt.String()); err != nil {
		return NewResourceError("setting issued_at", d.Id(), err)
	}
	if err := d.Set("expires_at", info.ExpiresAt.String()); err != nil {
		return NewResourceError("setting expires_at", d.Id(), err)
	}
	if err := d.Set("trial", info.Trial); err != nil {
		return NewResourceError("setting trial", d.Id(), err)
	}

	return nil
}
