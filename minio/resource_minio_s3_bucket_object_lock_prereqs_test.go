package minio

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7"
)

func TestResourceMinioS3BucketObjectLockConfigurationDefaultRetentionValidation(t *testing.T) {
	res := resourceMinioS3BucketObjectLockConfiguration()
	if err := res.InternalValidate(nil, true); err != nil {
		t.Fatalf("InternalValidate() failed: %v", err)
	}

	neitherPattern := regexp.MustCompile(`one of .*days.*years.* must be specified`)
	bothPattern := regexp.MustCompile(`only one of .*days.*years.* can be specified`)

	cases := []struct {
		name        string
		raw         map[string]interface{}
		wantErrExpr *regexp.Regexp
	}{
		{
			name: "default_retention without days or years fails validation",
			raw: map[string]interface{}{
				"bucket": "locked-bucket",
				"rule": []interface{}{map[string]interface{}{
					"default_retention": []interface{}{map[string]interface{}{
						"mode": "GOVERNANCE",
					}},
				}},
			},
			wantErrExpr: neitherPattern,
		},
		{
			name: "default_retention with both days and years fails validation",
			raw: map[string]interface{}{
				"bucket": "locked-bucket",
				"rule": []interface{}{map[string]interface{}{
					"default_retention": []interface{}{map[string]interface{}{
						"mode":  "GOVERNANCE",
						"days":  30,
						"years": 1,
					}},
				}},
			},
			wantErrExpr: bothPattern,
		},
		{
			name: "default_retention with days passes validation",
			raw: map[string]interface{}{
				"bucket": "locked-bucket",
				"rule": []interface{}{map[string]interface{}{
					"default_retention": []interface{}{map[string]interface{}{
						"mode": "GOVERNANCE",
						"days": 30,
					}},
				}},
			},
		},
		{
			name: "default_retention with years passes validation",
			raw: map[string]interface{}{
				"bucket": "locked-bucket",
				"rule": []interface{}{map[string]interface{}{
					"default_retention": []interface{}{map[string]interface{}{
						"mode":  "COMPLIANCE",
						"years": 1,
					}},
				}},
			},
		},
		{
			name: "omitted rule clears retention without triggering ExactlyOneOf",
			raw: map[string]interface{}{
				"bucket": "locked-bucket",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := res.Validate(terraform.NewResourceConfigRaw(tc.raw))
			if tc.wantErrExpr == nil {
				if diags.HasError() {
					t.Fatalf("unexpected validation errors: %v", diags)
				}
				return
			}
			if !diags.HasError() {
				t.Fatalf("expected validation error matching %s, got none", tc.wantErrExpr)
			}
			var parts []string
			for _, d := range diags {
				parts = append(parts, d.Summary, d.Detail)
			}
			joined := strings.Join(parts, " ")
			if !tc.wantErrExpr.MatchString(joined) {
				t.Fatalf("validation error = %q, want match %s", joined, tc.wantErrExpr)
			}
		})
	}
}

func TestValidateObjectLockChecksLockBeforeVersioningAgainstStubBackend(t *testing.T) {
	const notFoundXML = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>ObjectLockConfigurationNotFoundError</Code><Message>Object Lock configuration does not exist</Message></Error>`
	const lockEnabledXML = `<ObjectLockConfiguration><ObjectLockEnabled>Enabled</ObjectLockEnabled></ObjectLockConfiguration>`

	cases := []struct {
		name              string
		lockStatus        int
		lockBody          string
		versioningBody    string
		wantVersioningHit bool
		wantConfigErr     string
		wantRetentionErr  string
	}{
		{
			name:              "unversioned bucket without object lock reports object lock error without checking versioning",
			lockStatus:        http.StatusNotFound,
			lockBody:          notFoundXML,
			versioningBody:    `<VersioningConfiguration/>`,
			wantVersioningHit: false,
			wantConfigErr:     "doesn't have object lock enabled",
			wantRetentionErr:  "does not have object lock enabled",
		},
		{
			name:              "bucket with object lock enabled still checks versioning",
			lockStatus:        http.StatusOK,
			lockBody:          lockEnabledXML,
			versioningBody:    `<VersioningConfiguration/>`,
			wantVersioningHit: true,
			wantConfigErr:     "versioning enabled",
			wantRetentionErr:  "versioning enabled",
		},
	}

	for _, tc := range cases {
		for _, target := range []struct {
			name string
			fn   func(context.Context, *minio.Client, string) error
		}{
			{
				name: "object_lock_configuration",
				fn:   validateObjectLockPrerequisites,
			},
			{
				name: "bucket_retention",
				fn:   validateBucketObjectLock,
			},
		} {
			t.Run(tc.name+"/"+target.name, func(t *testing.T) {
				var versioningCalled bool
				client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/xml")
					switch {
					case r.URL.Query().Has("object-lock"):
						w.WriteHeader(tc.lockStatus)
						_, _ = fmt.Fprint(w, tc.lockBody)
					case r.URL.Query().Has("versioning"):
						versioningCalled = true
						w.WriteHeader(http.StatusOK)
						_, _ = fmt.Fprint(w, tc.versioningBody)
					default:
						w.WriteHeader(http.StatusOK)
					}
				})

				err := target.fn(context.Background(), client, "plain-bucket")
				if err == nil {
					t.Fatalf("expected error, got nil")
				}

				wantSub := tc.wantConfigErr
				if target.name == "bucket_retention" {
					wantSub = tc.wantRetentionErr
				}
				if !strings.Contains(err.Error(), wantSub) {
					t.Fatalf("error = %q, want substring %q", err.Error(), wantSub)
				}
				if !tc.wantVersioningHit && strings.Contains(strings.ToLower(err.Error()), "versioning") {
					t.Fatalf("error = %q, must not mention versioning when object lock is absent", err.Error())
				}
				if versioningCalled != tc.wantVersioningHit {
					t.Fatalf("versioningCalled = %v, want %v", versioningCalled, tc.wantVersioningHit)
				}
			})
		}
	}
}
