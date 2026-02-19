package minio

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

const prometheusIssuer = "prometheus"

func resourceMinioPrometheusBearerToken() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreatePrometheusBearerToken,
		ReadContext:   minioReadPrometheusBearerToken,
		UpdateContext: minioUpdatePrometheusBearerToken,
		DeleteContext: minioDeletePrometheusBearerToken,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Description: `Manages MinIO Prometheus bearer tokens for metrics authentication.

Bearer tokens are JWTs signed with MinIO credentials that authenticate
requests to Prometheus metrics endpoints. Each metric type (cluster, node,
bucket, resource) can have its own token.

Tokens are generated locally using the provider's access and secret keys,
so no API call is needed to create them. The token is valid for the specified
duration from creation time.`,

		Schema: map[string]*schema.Schema{
			"metric_type": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice([]string{"cluster", "node", "bucket", "resource"}, false),
				Description:  "Type of metrics to authenticate. Valid values: cluster, node, bucket, resource",
			},
			"expires_in": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "87600h",
				ValidateFunc: validation.StringMatch(regexp.MustCompile(`^\d+h$`), "must be in format: number followed by 'h' (e.g., 24h, 87600h)"),
				Description:  "Token expiry duration in whole hours only (e.g., 24h, 87600h). Go time.Duration formats like 24h30m or units such as m/s are not supported. Default: 87600h (10 years)",
			},
			"limit": {
				Type:         schema.TypeInt,
				Optional:     true,
				Computed:     true,
				Default:      876000,
				ForceNew:     true,
				ValidateFunc: validation.IntAtLeast(1),
				Description:  "Maximum token expiry in hours. Default: 876000 (100 years)",
			},
			"token": {
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
				Description: "Generated JWT bearer token for the metrics endpoint",
			},
			"token_expiry": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Expiry timestamp of the token in RFC3339 format",
			},
		},
	}
}

func minioCreatePrometheusBearerToken(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := PrometheusBearerTokenConfig(d, meta)

	log.Printf("[DEBUG] Creating Prometheus bearer token for metric type: %s", config.MetricType)

	duration, err := time.ParseDuration(config.ExpiresIn)
	if err != nil {
		return NewResourceError("parsing expires_in duration", config.MetricType, err)
	}

	token, err := generatePrometheusToken(config.MinioAccessKey, config.MinioSecretKey, duration, config.Limit)
	if err != nil {
		return NewResourceError("creating Prometheus bearer token", config.MetricType, err)
	}

	expiry := time.Now().UTC().Add(duration)

	id := config.MetricType
	d.SetId(id)

	if err := d.Set("token", token); err != nil {
		return NewResourceError("setting token", id, err)
	}

	if err := d.Set("token_expiry", expiry.Format(time.RFC3339)); err != nil {
		return NewResourceError("setting token_expiry", id, err)
	}

	log.Printf("[DEBUG] Created Prometheus bearer token for metric type: %s", config.MetricType)

	return minioReadPrometheusBearerToken(ctx, d, meta)
}

func minioReadPrometheusBearerToken(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	metricType := d.Id()

	log.Printf("[DEBUG] Reading Prometheus bearer token for metric type: %s", metricType)

	if err := d.Set("metric_type", metricType); err != nil {
		return NewResourceError("setting metric_type", metricType, err)
	}

	token, ok := d.GetOk("token")
	if ok {
		if err := d.Set("token", token); err != nil {
			return NewResourceError("setting token", metricType, err)
		}
	}

	tokenExpiry, ok := d.GetOk("token_expiry")
	if ok {
		if err := d.Set("token_expiry", tokenExpiry); err != nil {
			return NewResourceError("setting token_expiry", metricType, err)
		}
	}

	return nil
}

func minioUpdatePrometheusBearerToken(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	config := PrometheusBearerTokenConfig(d, meta)
	metricType := d.Id()

	log.Printf("[DEBUG] Updating Prometheus bearer token for metric type: %s", metricType)

	duration, err := time.ParseDuration(config.ExpiresIn)
	if err != nil {
		return NewResourceError("parsing expires_in duration", metricType, err)
	}

	token, err := generatePrometheusToken(config.MinioAccessKey, config.MinioSecretKey, duration, config.Limit)
	if err != nil {
		return NewResourceError("updating Prometheus bearer token", metricType, err)
	}

	expiry := time.Now().UTC().Add(duration)

	if err := d.Set("token", token); err != nil {
		return NewResourceError("setting token", metricType, err)
	}

	if err := d.Set("token_expiry", expiry.Format(time.RFC3339)); err != nil {
		return NewResourceError("setting token_expiry", metricType, err)
	}

	log.Printf("[DEBUG] Updated Prometheus bearer token for metric type: %s", metricType)

	return minioReadPrometheusBearerToken(ctx, d, meta)
}

func minioDeletePrometheusBearerToken(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	metricType := d.Id()

	log.Printf("[DEBUG] Deleting Prometheus bearer token for metric type: %s", metricType)

	d.SetId("")

	return nil
}

func generatePrometheusToken(accessKey, secretKey string, expiry time.Duration, limit int) (string, error) {
	if expiry.Hours() > float64(limit) {
		expiry = time.Duration(limit) * time.Hour
	}

	token, err := generateJWTToken(accessKey, secretKey, expiry)
	if err != nil {
		return "", fmt.Errorf("error generating Prometheus token: %w", err)
	}

	return token, nil
}

func generateJWTToken(accessKey, secretKey string, expiry time.Duration) (string, error) {
	jwt := &jwtClaim{
		Subject:   accessKey,
		Issuer:    prometheusIssuer,
		ExpiresAt: time.Now().Add(expiry).UTC(),
	}

	token, err := jwt.sign(secretKey)
	if err != nil {
		return "", err
	}

	return token, nil
}

type jwtClaim struct {
	Subject   string    `json:"sub"`
	Issuer    string    `json:"iss"`
	ExpiresAt time.Time `json:"exp"`
}

func (c *jwtClaim) sign(secretKey string) (string, error) {
	header := "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9"
	payloadBytes, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("error marshaling JWT payload: %w", err)
	}

	encodedPayload := base64URLEncode(payloadBytes)
	data := header + "." + encodedPayload

	h := hmac.New(sha512.New, []byte(secretKey))
	h.Write([]byte(data))
	signature := h.Sum(nil)

	return header + "." + encodedPayload + "." + base64URLEncode(signature), nil
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
