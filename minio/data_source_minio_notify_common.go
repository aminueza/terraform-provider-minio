package minio

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceNotify(nrc notifyResourceConfig, resourceSchema map[string]*schema.Schema) *schema.Resource {
	dsSchema := make(map[string]*schema.Schema, len(resourceSchema))
	for k, v := range resourceSchema {
		cp := *v
		if k == "name" {
			cp.Required = true
			cp.Optional = false
			cp.Computed = false
			cp.ForceNew = false
		} else {
			cp.Required = false
			cp.Optional = false
			cp.Computed = true
			cp.ForceNew = false
			cp.Default = nil
			cp.ValidateFunc = nil
			cp.Sensitive = false
		}
		dsSchema[k] = &cp
	}

	return &schema.Resource{
		ReadContext: func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
			d.SetId(d.Get("name").(string))
			return notifyRead(nrc)(ctx, d, meta)
		},
		Schema: dsSchema,
	}
}
