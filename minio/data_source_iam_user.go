package minio

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	madmin "github.com/minio/madmin-go/v3"
)

// adminFromMeta extracts *madmin.AdminClient from provider meta without
// depending on internal types. It tries common patterns (method or field).
func adminFromMeta(meta interface{}) (*madmin.AdminClient, error) {
	if meta == nil {
		return nil, fmt.Errorf("provider meta is nil")
	}

	// Direct pointer
	if ac, ok := meta.(*madmin.AdminClient); ok && ac != nil {
		return ac, nil
	}

	// Method returning *madmin.AdminClient (Admin, AdminClient, GetAdmin)
	for _, m := range []string{"Admin", "AdminClient", "GetAdmin"} {
		mv := reflect.ValueOf(meta).MethodByName(m)
		if mv.IsValid() && mv.Type().NumIn() == 0 && mv.Type().NumOut() == 1 {
			if mv.Type().Out(0) == reflect.TypeOf((*madmin.AdminClient)(nil)) {
				out := mv.Call(nil)[0]
				if !out.IsNil() {
					return out.Interface().(*madmin.AdminClient), nil
				}
			}
		}
	}

	// Exported field of type *madmin.AdminClient
	rv := reflect.ValueOf(meta)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.IsValid() && rv.Kind() == reflect.Struct {
		for i := 0; i < rv.NumField(); i++ {
			fv := rv.Field(i)
			if fv.CanInterface() && fv.Type() == reflect.TypeOf((*madmin.AdminClient)(nil)) {
				val := fv.Interface().(*madmin.AdminClient)
				if val != nil {
					return val, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("cannot extract *madmin.AdminClient from provider meta")
}

// Data source: minio_iam_user — reads one existing user by name.
func dataSourceIAMUser() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceIAMUserRead,
		Schema: map[string]*schema.Schema{
			// Input
			"name": {Type: schema.TypeString, Required: true},

			// Outputs
			"status": {Type: schema.TypeString, Computed: true},

			// Placeholders for future enrichment (policies & groups).
			"policy_names": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"member_of_groups": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func dataSourceIAMUserRead(d *schema.ResourceData, meta interface{}) error {
	admin, err := adminFromMeta(meta)
	if err != nil {
		return err
	}

	name := d.Get("name").(string)
	info, err := admin.GetUserInfo(context.Background(), name)
	if err != nil {
		return err
	}

	d.SetId(name)
	_ = d.Set("status", strings.ToLower(string(info.Status)))
	_ = d.Set("policy_names", []string{})
	_ = d.Set("member_of_groups", []string{})

	return nil
}