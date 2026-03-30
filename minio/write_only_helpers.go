package minio

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func getWriteOnlyStringAt(d *schema.ResourceData, path cty.Path, fieldName string) (string, bool, error) {
	val, diags := d.GetRawConfigAt(path)
	if diags.HasError() {
		return "", false, fmt.Errorf("error retrieving write-only argument %s", fieldName)
	}

	if !val.IsKnown() || val.IsNull() {
		return "", false, nil
	}

	if !val.Type().Equals(cty.String) {
		return "", false, fmt.Errorf("error retrieving write-only argument %s: retrieved config value is not a string", fieldName)
	}

	trimmed := strings.TrimSpace(val.AsString())
	if trimmed == "" {
		return "", false, nil
	}

	return trimmed, true, nil
}

func getRawConfigStringAttr(rawConfig cty.Value, attrName string, fieldName string) (string, bool, error) {
	attrVal := rawConfig.GetAttr(attrName)
	if !attrVal.IsKnown() || attrVal.IsNull() {
		return "", false, nil
	}

	if !attrVal.Type().Equals(cty.String) {
		return "", false, fmt.Errorf("error retrieving write-only argument %s: retrieved config value is not a string", fieldName)
	}

	trimmed := strings.TrimSpace(attrVal.AsString())
	if trimmed == "" {
		return "", false, nil
	}

	return trimmed, true, nil
}
