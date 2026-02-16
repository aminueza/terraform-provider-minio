package minio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

const anonymousAccessResourcePrefix = "anonymous::"

func encodeAnonymousAccessID(bucket string) string {
	return fmt.Sprintf("%s%s", anonymousAccessResourcePrefix, bucket)
}

func putAnonymousBucketPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}, bucket, policy string) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client

	// Wait for bucket to be ready for eventual consistency
	timeout := d.Timeout(schema.TimeoutCreate)
	if d.Id() != "" {
		timeout = d.Timeout(schema.TimeoutUpdate)
	}
	// Reserve time for the actual operation
	waitTimeout := timeout - 30*time.Second
	if waitTimeout < 30*time.Second {
		waitTimeout = 30 * time.Second
	}

	if err := waitForBucketReady(ctx, client, bucket, waitTimeout); err != nil {
		return NewResourceError("error waiting for bucket to be ready", bucket, err)
	}

	// Retry SetBucketPolicy for transient NoSuchBucket errors
	err := retry.RetryContext(ctx, waitTimeout, func() *retry.RetryError {
		err := client.SetBucketPolicy(ctx, bucket, policy)
		if err != nil {
			if isNoSuchBucketError(err) {
				log.Printf("[DEBUG] Bucket %q not yet available for policy, retrying...", bucket)
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return NewResourceError("error putting bucket policy", bucket, err)
	}

	return nil
}

func readAnonymousBucketPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}, bucket string) (string, diag.Diagnostics) {
	client := meta.(*S3MinioClient).S3Client
	timeout := d.Timeout(schema.TimeoutRead)

	if err := waitForBucketReady(ctx, client, bucket, timeout); err != nil {
		if isNoSuchBucketError(err) {
			log.Printf("[WARN] Bucket %s not found after waiting, removing anonymous policy resource from state", bucket)
			d.SetId("")
			return "", nil
		}
		return "", NewResourceError("error waiting for bucket to be ready", bucket, err)
	}

	actualPolicyText, err := client.GetBucketPolicy(ctx, bucket)
	if err != nil {
		if isNoSuchBucketError(err) {
			log.Printf("[WARN] Bucket %s no longer exists, removing anonymous policy resource from state", bucket)
			d.SetId("")
			return "", nil
		}
		return "", NewResourceError("failed to load bucket policy", bucket, err)
	}

	existingPolicy := ""
	if v, ok := d.GetOk("policy"); ok {
		existingPolicy = v.(string)
	}

	policy, err := NormalizeAndCompareJSONPolicies(existingPolicy, actualPolicyText)
	if err != nil {
		return "", NewResourceError("error while comparing policies", bucket, err)
	}

	return policy, nil
}

func deleteAnonymousBucketPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}, bucket string) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Client

	if err := client.SetBucketPolicy(ctx, bucket, ""); err != nil {
		if isNoSuchBucketError(err) {
			log.Printf("[DEBUG] Bucket %q already deleted, skipping policy removal", bucket)
			return nil
		}
		return NewResourceError("error deleting bucket policy", bucket, err)
	}

	return nil
}

func decodeAnonymousAccessID(id string) string {
	if strings.HasPrefix(id, anonymousAccessResourcePrefix) {
		return strings.TrimPrefix(id, anonymousAccessResourcePrefix)
	}
	return id
}

func resourceMinioS3BucketAnonymousAccess() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioSetAnonymousPolicy,
		ReadContext:   minioReadAnonymousPolicy,
		UpdateContext: minioSetAnonymousPolicy,
		DeleteContext: minioDeleteAnonymousPolicy,
		Importer: &schema.ResourceImporter{
			StateContext: func(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
				bucket := decodeAnonymousAccessID(d.Id())
				d.SetId(encodeAnonymousAccessID(bucket))
				if err := d.Set("bucket", bucket); err != nil {
					return nil, fmt.Errorf("setting bucket: %w", err)
				}
				diags := minioReadAnonymousPolicy(ctx, d, meta)
				if diags.HasError() {
					return nil, errors.New(diags[0].Summary)
				}
				return []*schema.ResourceData{d}, nil
			},
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Read:   schema.DefaultTimeout(2 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Description: "Name of the bucket",
				Required:    true,
				ForceNew:    true,
			},
			"policy": {
				Type:             schema.TypeString,
				Description:      "Custom policy JSON string for anonymous access. For canned policies (public, public-read, public-read-write, public-write), use the access_type field instead.",
				Optional:         true,
				Computed:         true,
				ValidateFunc:     validateIAMPolicyJSON,
				DiffSuppressFunc: suppressEquivalentAwsPolicyDiffs,
				AtLeastOneOf:     []string{"policy", "access_type"},
			},
			"access_type": {
				Type:         schema.TypeString,
				Description:  "Canned access type for anonymous access",
				Optional:     true,
				AtLeastOneOf: []string{"policy", "access_type"},
				ValidateFunc: validation.StringInSlice([]string{"public", "public-read", "public-read-write", "public-write"}, false),
			},
		},
	}
}

func minioSetAnonymousPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketName := d.Get("bucket").(string)
	policy, err := getAnonymousPolicy(d, bucketName)
	if err != nil {
		return NewResourceError("building anonymous access policy", bucketName, err)
	}

	if policy == "" {
		return NewResourceError("validating anonymous access configuration", bucketName, errors.New("policy or access_type must be specified"))
	}

	// Normalize the policy for consistent formatting
	normalizedPolicy, err := structure.NormalizeJsonString(policy)
	if err != nil {
		return NewResourceError("failed to normalize policy JSON", bucketName, err)
	}

	// Set the resource ID to be unique for anonymous access resources
	d.SetId(encodeAnonymousAccessID(bucketName))

	if err := d.Set("policy", normalizedPolicy); err != nil {
		return NewResourceError("setting policy", bucketName, err)
	}

	// Preserve access_type if provided by user, otherwise derive from policy
	accessType := d.Get("access_type").(string)
	if accessType == "" {
		accessType, err = getAccessTypeFromPolicy(normalizedPolicy, bucketName)
		if err != nil {
			return NewResourceError("determining access_type", bucketName, err)
		}
		if accessType != "" {
			if err := d.Set("access_type", accessType); err != nil {
				return NewResourceError("setting access_type", bucketName, err)
			}
		}
	}

	log.Printf("[DEBUG] Setting anonymous access policy for bucket: %s, policy: %s, access_type: %s", bucketName, normalizedPolicy, accessType)

	if diags := putAnonymousBucketPolicy(ctx, d, meta, bucketName, normalizedPolicy); diags.HasError() {
		return diags
	}

	return minioReadAnonymousPolicy(ctx, d, meta)
}

func minioReadAnonymousPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketName := decodeAnonymousAccessID(d.Id())
	log.Printf("[DEBUG] Reading anonymous access policy for bucket: %s", bucketName)

	// Ensure the bucket attribute is populated for the shared bucket policy helpers
	if err := d.Set("bucket", bucketName); err != nil {
		return NewResourceError("setting bucket", bucketName, err)
	}

	policy, diags := readAnonymousBucketPolicy(ctx, d, meta, bucketName)
	if diags.HasError() {
		return diags
	}
	if d.Id() == "" {
		return nil
	}
	currentPolicy := policy

	if err := d.Set("policy", currentPolicy); err != nil {
		return NewResourceError("setting policy", bucketName, err)
	}

	// Derive access_type from policy - this is needed for import to work correctly
	// and to maintain consistency between policy and access_type fields
	accessType, err := getAccessTypeFromPolicy(currentPolicy, bucketName)
	if err != nil {
		return NewResourceError("determining access_type", bucketName, err)
	}
	if accessType != "" {
		if err := d.Set("access_type", accessType); err != nil {
			return NewResourceError("setting access_type", bucketName, err)
		}
	}

	return nil
}

func minioDeleteAnonymousPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	bucketName := decodeAnonymousAccessID(d.Id())
	if bucketName == "" {
		bucketName = d.Get("bucket").(string)
	}

	if err := d.Set("bucket", bucketName); err != nil {
		return NewResourceError("setting bucket", bucketName, err)
	}

	log.Printf("[DEBUG] Deleting anonymous access policy for bucket: %s", bucketName)

	if diags := deleteAnonymousBucketPolicy(ctx, d, meta, bucketName); diags.HasError() {
		return diags
	}

	return nil
}

func getAnonymousPolicy(d *schema.ResourceData, bucket string) (string, error) {
	// Determine if policy was explicitly set by checking raw config
	// During Create/Update, RawConfig is available; during Read/Delete, it's not
	rawConfig := d.GetRawConfig()
	policyExplicitlySet := false
	if !rawConfig.IsNull() {
		policyAttr := rawConfig.GetAttr("policy")
		policyExplicitlySet = !policyAttr.IsNull() && policyAttr.IsKnown() && policyAttr.AsString() != ""
	}

	accessType := d.Get("access_type").(string)

	// If policy is explicitly set by user, use it (it takes precedence over access_type)
	if policyExplicitlySet {
		policy := d.Get("policy").(string)
		if policy != "" {
			return policy, nil
		}
	}

	// If access_type is set, generate policy from it
	if accessType != "" {
		switch accessType {
		case "public":
			return marshalPolicy(PublicPolicy(&S3MinioBucket{MinioBucket: bucket}))
		case "public-read":
			return marshalPolicy(ReadOnlyPolicy(&S3MinioBucket{MinioBucket: bucket}))
		case "public-read-write":
			return marshalPolicy(ReadWritePolicy(&S3MinioBucket{MinioBucket: bucket}))
		case "public-write":
			return marshalPolicy(WriteOnlyPolicy(&S3MinioBucket{MinioBucket: bucket}))
		}
	}

	// Fallback: use whatever policy is in state (may be computed or from previous access_type)
	policy := d.Get("policy").(string)
	if policy != "" {
		return policy, nil
	}

	return "", nil
}

func marshalPolicy(policyStruct BucketPolicy) (string, error) {
	policyJSON, err := json.Marshal(policyStruct)
	if err != nil {
		return "", err
	}
	return string(policyJSON), nil
}

func getAccessTypeFromPolicy(policy string, bucketName string) (string, error) {
	// Generate canned policies for this specific bucket
	publicPolicy, _ := marshalPolicy(PublicPolicy(&S3MinioBucket{MinioBucket: bucketName}))
	readOnlyPolicy, _ := marshalPolicy(ReadOnlyPolicy(&S3MinioBucket{MinioBucket: bucketName}))
	readWritePolicy, _ := marshalPolicy(ReadWritePolicy(&S3MinioBucket{MinioBucket: bucketName}))
	writeOnlyPolicy, _ := marshalPolicy(WriteOnlyPolicy(&S3MinioBucket{MinioBucket: bucketName}))

	equivalent, err := awspolicy.PoliciesAreEquivalent(policy, readOnlyPolicy)
	if err != nil {
		return "", err
	}
	if equivalent {
		return "public-read", nil
	}

	equivalent, err = awspolicy.PoliciesAreEquivalent(policy, publicPolicy)
	if err != nil {
		return "", err
	}
	if equivalent {
		return "public", nil
	}

	equivalent, err = awspolicy.PoliciesAreEquivalent(policy, readWritePolicy)
	if err != nil {
		return "", err
	}
	if equivalent {
		return "public-read-write", nil
	}

	equivalent, err = awspolicy.PoliciesAreEquivalent(policy, writeOnlyPolicy)
	if err != nil {
		return "", err
	}
	if equivalent {
		return "public-write", nil
	}

	return "", nil
}
