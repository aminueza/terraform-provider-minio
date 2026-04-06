package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceIAMUser() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieves information about a specific IAM user by name.",
		ReadContext: dataSourceIAMUserRead,
		Schema: map[string]*schema.Schema{
			"name":   {Type: schema.TypeString, Required: true},
			"status": {Type: schema.TypeString, Computed: true},
			"tags":   {Type: schema.TypeMap, Computed: true, Elem: &schema.Schema{Type: schema.TypeString}},
		},
	}
}

func dataSourceIAMUserRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	d.SetId(d.Get("name").(string))
	return minioReadUser(ctx, d, meta)
}
