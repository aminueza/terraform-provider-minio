package minio

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceIAMPolicy() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieves an existing IAM policy by name.",
		Read:        dataSourceIAMPolicyRead,
		Schema: map[string]*schema.Schema{
			"name":   {Type: schema.TypeString, Required: true},
			"policy": {Type: schema.TypeString, Computed: true},
		},
	}
}

func dataSourceIAMPolicyRead(d *schema.ResourceData, meta interface{}) error {
	admin := meta.(*S3MinioClient).S3Admin

	name := d.Get("name").(string)
	info, err := admin.InfoCannedPolicyV2(context.Background(), name)
	if err != nil {
		return err
	}

	d.SetId(name)

	policyJSON, err := json.Marshal(json.RawMessage(info.Policy))
	if err != nil {
		return err
	}
	_ = d.Set("policy", string(policyJSON))

	return nil
}
