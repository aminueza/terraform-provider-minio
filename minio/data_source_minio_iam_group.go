package minio

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceIAMGroup() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieves information about a specific IAM group by name.",
		ReadContext: dataSourceIAMGroupRead,
		Schema: map[string]*schema.Schema{
			"name":   {Type: schema.TypeString, Required: true},
			"status": {Type: schema.TypeString, Computed: true},
			"policy": {Type: schema.TypeString, Computed: true},
			"members": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func dataSourceIAMGroupRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	name := d.Get("name").(string)
	desc, err := admin.GetGroupDescription(ctx, name)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(name)
	_ = d.Set("status", strings.ToLower(desc.Status))
	_ = d.Set("policy", desc.Policy)
	_ = d.Set("members", desc.Members)

	return nil
}
