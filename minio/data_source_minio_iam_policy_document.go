package minio

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/minio/minio-go/v7/pkg/set"
)

var dataSourceMinioIAMPolicyDocumentReplacer = strings.NewReplacer("&{", "${")

func dataSourceMinioIAMPolicyDocument() *schema.Resource {
	stringSet := &schema.Schema{
		Type:     schema.TypeSet,
		Optional: true,
		Elem: &schema.Schema{
			Type: schema.TypeString,
		},
	}

	return &schema.Resource{
		Description: "Generates an IAM policy document in JSON format for use with IAM policies.",
		Read:        dataSourceMinioIAMPolicyDocumentRead,

		Schema: map[string]*schema.Schema{
			"override_json": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"policy_id": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"source_json": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"statement": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"sid": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"effect": {
							Type:         schema.TypeString,
							Optional:     true,
							Default:      "Allow",
							ValidateFunc: validation.StringInSlice([]string{"Allow", "Deny"}, false),
						},
						"actions":       stringSet,
						"resources":     stringSet,
						"not_resources": stringSet,
						"principal": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"not_principal": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"condition": {
							Type:     schema.TypeSet,
							Optional: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"test": {
										Type:     schema.TypeString,
										Required: true,
									},
									"variable": {
										Type:     schema.TypeString,
										Required: true,
									},
									"values": {
										Type:     schema.TypeSet,
										Required: true,
										Elem: &schema.Schema{
											Type: schema.TypeString,
										},
									},
								},
							},
						},
					},
				},
			},
			"version": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "2012-10-17",
				ValidateFunc: validation.StringInSlice([]string{
					"2008-10-17",
					"2012-10-17",
				}, false),
			},
			"json": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceMinioIAMPolicyDocumentRead(d *schema.ResourceData, meta interface{}) error {
	mergedDoc := &IAMPolicyDoc{}

	if sourceJSON, hasSourceJSON := d.GetOk("source_json"); hasSourceJSON {
		if err := json.Unmarshal([]byte(sourceJSON.(string)), mergedDoc); err != nil {
			return err
		}
	}

	doc := &IAMPolicyDoc{
		Version: d.Get("version").(string),
	}

	if policyID, hasPolicyID := d.GetOk("policy_id"); hasPolicyID {
		doc.ID = policyID.(string)
	}

	if cfgStmts, hasCfgStmts := d.GetOk("statement"); hasCfgStmts {
		var cfgStmtIntf = cfgStmts.([]interface{})
		stmts := make([]*IAMPolicyStatement, len(cfgStmtIntf))
		sidMap := make(map[string]struct{})

		for i, stmtI := range cfgStmtIntf {
			cfgStmt := stmtI.(map[string]interface{})
			stmt := &IAMPolicyStatement{
				Effect: cfgStmt["effect"].(string),
			}

			if sid, ok := cfgStmt["sid"]; ok {
				if _, ok := sidMap[sid.(string)]; ok {
					return fmt.Errorf("found duplicate sid (%s), either remove the sid or ensure the sid is unique across all statements", sid.(string))
				}
				stmt.Sid = sid.(string)
				if len(stmt.Sid) > 0 {
					sidMap[stmt.Sid] = struct{}{}
				}
			}

			if actions := cfgStmt["actions"].(*schema.Set).List(); len(actions) > 0 {
				stmt.Actions = minioDecodePolicyStringList(actions)
			}

			if resources := cfgStmt["resources"].(*schema.Set).List(); len(resources) > 0 {
				var err error
				stmt.Resources, err = dataSourceMinioIAMPolicyDocumentReplaceVarsInList(
					minioDecodePolicyStringList(resources), doc.Version,
				)
				if err != nil {
					return fmt.Errorf("error reading resources: %s", err)
				}
			}

			if notResources := cfgStmt["not_resources"].(*schema.Set).List(); len(notResources) > 0 {
				var err error
				stmt.NotResources, err = dataSourceMinioIAMPolicyDocumentReplaceVarsInList(
					minioDecodePolicyStringList(notResources), doc.Version,
				)
				if err != nil {
					return fmt.Errorf("error reading not_resources: %s", err)
				}
			}

			resourcesSet := stmt.Resources != nil
			notResourcesSet := stmt.NotResources != nil

			if resourcesSet {
				switch v := stmt.Resources.(type) {
				case string:
					resourcesSet = v != ""
				case []string:
					resourcesSet = len(v) > 0
				default:
					resourcesSet = false
				}
			}

			if notResourcesSet {
				switch v := stmt.NotResources.(type) {
				case string:
					notResourcesSet = v != ""
				case []string:
					notResourcesSet = len(v) > 0
				default:
					notResourcesSet = false
				}
			}

			if resourcesSet && notResourcesSet {
				return fmt.Errorf("cannot set both resources and not_resources in the same statement")
			}

			principalString := cfgStmt["principal"].(string)
			notPrincipalString := ""

			if v, ok := cfgStmt["not_principal"]; ok && v != nil {
				notPrincipalString = v.(string)
			}

			if principalString != "" && notPrincipalString != "" {
				return fmt.Errorf("cannot set both principal and not_principal in the same statement")
			}

			if principalString != "" {
				stmt.Principal = principalString
			}

			if notPrincipalString != "" {
				stmt.NotPrincipal = notPrincipalString
			}

			if conditions := cfgStmt["condition"].(*schema.Set).List(); len(conditions) > 0 {
				var err error
				stmt.Conditions, err = dataSourceMinioIAMPolicyDocumentMakeConditions(conditions, doc.Version)
				if err != nil {
					return fmt.Errorf("error reading condition: %s", err)
				}
			}

			stmts[i] = stmt
		}

		doc.Statements = stmts

	}

	mergedDoc.merge(doc)

	if overrideJSON, hasOverrideJSON := d.GetOk("override_json"); hasOverrideJSON {
		overrideDoc := &IAMPolicyDoc{}
		if err := json.Unmarshal([]byte(overrideJSON.(string)), overrideDoc); err != nil {
			return err
		}

		mergedDoc.merge(overrideDoc)
	}

	jsonDoc, err := json.MarshalIndent(mergedDoc, "", "  ")
	if err != nil {
		return err
	}
	jsonString := string(jsonDoc)

	_ = d.Set("json", jsonString)
	d.SetId(strconv.Itoa(HashcodeString(jsonString)))

	return nil
}

func dataSourceMinioIAMPolicyDocumentReplaceVarsInList(in interface{}, version string) (interface{}, error) {
	switch v := in.(type) {
	case string:
		if version == "2008-10-17" && strings.Contains(v, "&{") {
			return nil, fmt.Errorf("found &{ sequence in (%s), which is not supported in document version 2008-10-17", v)
		}
		return dataSourceMinioIAMPolicyDocumentReplacer.Replace(v), nil
	case []string:
		out := make([]string, len(v))
		for i, item := range v {
			if version == "2008-10-17" && strings.Contains(item, "&{") {
				return nil, fmt.Errorf("found &{ sequence in (%s), which is not supported in document version 2008-10-17", item)
			}
			out[i] = dataSourceMinioIAMPolicyDocumentReplacer.Replace(item)
		}
		return out, nil
	default:
		panic("dataSourceAwsIamPolicyDocumentReplaceVarsInList: input not string nor []string")
	}
}

func dataSourceMinioIAMPolicyDocumentMakeConditions(in []interface{}, version string) (interface{}, error) {
	out := make(ConditionMap, len(in))
	for _, itemI := range in {
		item := itemI.(map[string]interface{})
		condKeyMap := make(ConditionKeyMap)
		condMap := make(ConditionMap)
		values, err := dataSourceMinioIAMPolicyDocumentReplaceVarsInList(
			minioDecodePolicyStringList(
				item["values"].(*schema.Set).List(),
			), version,
		)
	switch v := values.(type) {
	case string:
		condKeyMap.Add(item["variable"].(string), set.CreateStringSet(v))
	case []string:
		for _, itemV := range v {
			condKeyMap.Add(item["variable"].(string), set.CreateStringSet(itemV))
		}
	}
		condMap.Add(item["test"].(string), condKeyMap)
		if err != nil {
			return nil, fmt.Errorf("error reading values: %s", err)
		}
		out = condMap
	}
	return out, nil
}
