package minio

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

func dataSourceIAMServiceAccounts() *schema.Resource {
	return &schema.Resource{
		Description: "Lists service accounts (access keys) for a specific IAM user.",
		ReadContext:        dataSourceIAMServiceAccountsRead,
		Schema: map[string]*schema.Schema{
			"user": {Type: schema.TypeString, Required: true, Description: "IAM user name to list service accounts for."},
			"service_accounts": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"access_key": {Type: schema.TypeString, Computed: true},
					},
				},
			},
		},
	}
}

func dataSourceIAMServiceAccountsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	user := d.Get("user").(string)
	resp, err := admin.ListServiceAccounts(ctx, user)
	if err != nil {
		return diag.FromErr(err)
	}

	var out []map[string]interface{}
	for _, sa := range resp.Accounts {
		out = append(out, map[string]interface{}{
			"access_key": sa.AccessKey,
		})
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	_ = d.Set("service_accounts", out)
	return nil
}
