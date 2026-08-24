package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

func dataSourceIAMPolicy() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieves an existing IAM policy by name.",
		ReadContext:        dataSourceIAMPolicyRead,
		Schema: map[string]*schema.Schema{
			"name":   {Type: schema.TypeString, Required: true},
			"policy": {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceIAMPolicyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	name := d.Get("name").(string)
	info, err := admin.InfoCannedPolicy(ctx, name)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(name)

	_ = d.Set("policy", string(info.Policy))

	return nil
}
