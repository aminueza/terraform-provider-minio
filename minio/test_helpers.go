package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/minio/madmin-go/v3"
	minio "github.com/minio/minio-go/v7/pkg/lifecycle"
)

// Helper functions for tests

func auditWebhookConfigKey(name string) string {
	return fmt.Sprintf("audit_webhook:%s", name)
}

func buildAuditWebhookCfgData(config *S3MinioAuditWebhook) string {
	var parts []string

	addParam := func(key, val string) {
		if val != "" {
			if strings.ContainsAny(val, " \t") {
				parts = append(parts, fmt.Sprintf("%s=%q", key, val))
			} else {
				parts = append(parts, fmt.Sprintf("%s=%s", key, val))
			}
		}
	}

	addParam("endpoint", config.Endpoint)
	addParam("auth_token", config.AuthToken)
	addParam("client_cert", config.ClientCert)
	addParam("client_key", config.ClientKey)

	if config.QueueSize > 0 {
		parts = append(parts, fmt.Sprintf("queue_size=%d", config.QueueSize))
	}

	if config.BatchSize > 0 {
		parts = append(parts, fmt.Sprintf("batch_size=%d", config.BatchSize))
	}

	if config.Enable {
		parts = append(parts, "enable=on")
	} else {
		parts = append(parts, "enable=off")
	}

	return strings.Join(parts, " ")
}

func buildIdpLdapCfgData(config *S3MinioIdpLdap) string {
	var parts []string

	addStr := func(key, val string) {
		if val != "" {
			if strings.ContainsAny(val, " \t") {
				parts = append(parts, fmt.Sprintf("%s=%q", key, val))
			} else {
				parts = append(parts, fmt.Sprintf("%s=%s", key, val))
			}
		}
	}
	addBool := func(key string, val bool) {
		if val {
			parts = append(parts, key+"=on")
		} else {
			parts = append(parts, key+"=off")
		}
	}

	addStr("server_addr", config.ServerAddr)
	addStr("lookup_bind_dn", config.LookupBindDN)
	addStr("lookup_bind_password", config.LookupBindPassword)
	addStr("user_dn_search_base_dn", config.UserDNSearchBaseDN)
	addStr("user_dn_search_filter", config.UserDNSearchFilter)
	addStr("group_search_base_dn", config.GroupSearchBaseDN)
	addStr("group_search_filter", config.GroupSearchFilter)
	addBool("tls_skip_verify", config.TLSSkipVerify)
	addBool("server_insecure", config.ServerInsecure)
	addBool("starttls", config.StartTLS)
	addBool("enable", config.Enable)

	return strings.Join(parts, " ")
}

func getTier(client *madmin.AdminClient, ctx context.Context, name string) (*madmin.TierConfig, error) {
	tiers, err := client.ListTiers(ctx)
	if err != nil {
		return nil, err
	}
	for _, tier := range tiers {
		if tier.Name == name {
			return tier, nil
		}
	}
	return nil, nil
}

func createLifecycleRule(ruleData map[string]interface{}) (minio.Rule, error) {
	id, ok := getStringValue(ruleData, "id")
	if !ok {
		return minio.Rule{}, fmt.Errorf("rule id is required")
	}

	status, ok := getStringValue(ruleData, "status")
	if !ok {
		status = "Enabled"
	}

	if err := validateILMRuleConflicts(ruleData); err != nil {
		return minio.Rule{}, err
	}

	// Build filter
	var filter minio.Filter
	prefix, _ := getStringValue(ruleData, "filter")
	rawTags, _ := ruleData["tags"].(map[string]interface{})
	if len(rawTags) > 0 {
		filter.And.Prefix = prefix
		for k, v := range rawTags {
			filter.And.Tags = append(filter.And.Tags, minio.Tag{Key: k, Value: fmt.Sprintf("%v", v)})
		}
	} else {
		filter.Prefix = prefix
	}
	if filter.IsNull() {
		filter.ObjectSizeGreaterThan = emptyFilterSentinel
	}

	// Build expiration
	var expiration minio.Expiration
	if expStr, ok := getStringValue(ruleData, "expiration"); ok && expStr != "" {
		var days int
		if _, err := fmt.Sscanf(expStr, "%dd", &days); err == nil {
			expiration = minio.Expiration{Days: minio.ExpirationDays(days)}
		}
	}

	return minio.Rule{
		ID:         id,
		Status:     status,
		RuleFilter: filter,
		Expiration: expiration,
	}, nil
}

func getStringValue(data map[string]interface{}, key string) (string, bool) {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

func validateILMRuleConflicts(ruleData map[string]interface{}) error {
	hasTransition := false
	hasNoncurrentTransition := false

	if _, exists := ruleData["transition"]; exists && len(ruleData["transition"].([]interface{})) > 0 {
		hasTransition = true
	}
	if _, exists := ruleData["noncurrent_transition"]; exists && len(ruleData["noncurrent_transition"].([]interface{})) > 0 {
		hasNoncurrentTransition = true
	}

	if hasTransition && hasNoncurrentTransition {
		return fmt.Errorf("transition and noncurrent_transition cannot be specified together")
	}

	return nil
}

func marshalPolicy(policy interface{}) (string, error) {
	if policy == nil {
		return "{}", nil
	}
	if p, ok := policy.([]byte); ok {
		return string(p), nil
	}
	if p, ok := policy.(string); ok {
		return p, nil
	}
	b, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseBucketAndKeyFromID(id string) (string, string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func validateS3BucketName(value string) error {
	if (len(value) < 3) || (len(value) > 63) {
		return fmt.Errorf("%q must contain from 3 to 63 characters", value)
	}
	if !regexp.MustCompile(`^[0-9a-z-.]+$`).MatchString(value) {
		return fmt.Errorf("only lowercase alphanumeric characters and hyphens allowed in %q", value)
	}
	if regexp.MustCompile(`^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$`).MatchString(value) {
		return fmt.Errorf("%q must not be formatted as an IP address", value)
	}
	if strings.HasPrefix(value, `.`) {
		return fmt.Errorf("%q cannot start with a period", value)
	}
	if strings.HasSuffix(value, `.`) {
		return fmt.Errorf("%q cannot end with a period", value)
	}
	if strings.Contains(value, `..`) {
		return fmt.Errorf("%q can be only one period between labels", value)
	}

	return nil
}
