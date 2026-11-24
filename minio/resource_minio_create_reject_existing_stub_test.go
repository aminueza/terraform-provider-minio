package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func newCreateRejectStubAdmin(t *testing.T, handler http.HandlerFunc) *madmin.AdminClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}
	return admin
}

func applyCreateWithStub(t *testing.T, res *schema.Resource, raw map[string]interface{}, meta interface{}) (*terraform.InstanceState, diag.Diagnostics) {
	t.Helper()
	ctx := context.Background()
	diff, err := res.Diff(ctx, nil, terraform.NewResourceConfigRaw(raw), meta)
	if err != nil {
		t.Fatalf("planning create: %v", err)
	}
	return res.Apply(ctx, nil, diff, meta)
}

func assertCreateAlreadyExistsError(t *testing.T, diags diag.Diagnostics, state *terraform.InstanceState, resourceType, name, importID string) {
	t.Helper()
	if !diags.HasError() {
		t.Fatalf("expected Create to fail for existing %s %q, but it succeeded", resourceType, name)
	}
	if state != nil && state.ID != "" {
		t.Fatalf("expected empty state ID after failed create, got %q", state.ID)
	}

	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.Summary+" "+d.Detail)
	}
	msg := strings.Join(msgs, "\n")

	for _, want := range []string{
		resourceType,
		fmt.Sprintf("%q", name),
		"already exists",
		fmt.Sprintf("terraform import %s.<name> %s", resourceType, importID),
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if strings.Contains(msg, "<nil>") {
		t.Errorf("error %q unexpectedly contains <nil>", msg)
	}
}

func runConcurrentDuplicateCreate(t *testing.T, fn func() (*terraform.InstanceState, diag.Diagnostics), resourceType, name, importID string) {
	t.Helper()
	type outcome struct {
		state *terraform.InstanceState
		diags diag.Diagnostics
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan outcome, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			st, diags := fn()
			results <- outcome{state: st, diags: diags}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	var succeeded, failed []outcome
	for res := range results {
		if res.diags.HasError() {
			failed = append(failed, res)
		} else {
			succeeded = append(succeeded, res)
		}
	}

	if len(succeeded) != 1 || len(failed) != 1 {
		t.Fatalf("expected 1 success and 1 failure under per-name lock, got %d succeeded, %d failed", len(succeeded), len(failed))
	}
	assertCreateAlreadyExistsError(t, failed[0].diags, failed[0].state, resourceType, name, importID)
}

func TestResourceMinioIAMGroupCreateRejectsExistingAndLookupErrors(t *testing.T) {
	t.Run("existing disabled group with members", func(t *testing.T) {
		name := "tfacc-grp-" + acctest.RandString(8)
		var mu sync.Mutex
		var writeCalls []string

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/group") {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"name":%q,"status":"disabled","members":["alice"]}`, name)
				return
			}
			mu.Lock()
			writeCalls = append(writeCalls, r.Method+" "+r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		})

		state, diags := applyCreateWithStub(t, resourceMinioIAMGroup(), map[string]interface{}{
			"name":          name,
			"disable_group": false,
		}, &S3MinioClient{S3Admin: admin})

		assertCreateAlreadyExistsError(t, diags, state, "minio_iam_group", name, name)
		if len(writeCalls) > 0 {
			t.Fatalf("expected no write calls for existing group, got: %v", writeCalls)
		}
	})

	t.Run("non-not-found lookup error fails create", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			status int
			body   string
		}{
			{"access_denied", http.StatusForbidden, `{"Code":"AccessDenied","Message":"Access Denied."}`},
			{"internal_error", http.StatusInternalServerError, `{"Code":"InternalError","Message":"backend down"}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				name := "tfacc-grp-" + acctest.RandString(8)
				var mu sync.Mutex
				var writeCalls []string

				admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, "/group") {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(tc.status)
						_, _ = fmt.Fprint(w, tc.body)
						return
					}
					mu.Lock()
					writeCalls = append(writeCalls, r.Method+" "+r.URL.Path)
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
				})

				state, diags := applyCreateWithStub(t, resourceMinioIAMGroup(), map[string]interface{}{
					"name": name,
				}, &S3MinioClient{S3Admin: admin})

				if !diags.HasError() {
					t.Fatalf("expected Create to fail when lookup fails with %s", tc.name)
				}
				if state != nil && state.ID != "" {
					t.Fatalf("expected empty state ID, got %q", state.ID)
				}
				if len(writeCalls) > 0 {
					t.Fatalf("expected no write calls on lookup error, got: %v", writeCalls)
				}
			})
		}
	})

	t.Run("status failure after create keeps the group tainted in state", func(t *testing.T) {
		name := "tfacc-grp-" + acctest.RandString(8)

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/group") && r.Method == http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"Code":"XMinioAdminNoSuchGroup","Message":"The group not found"}`)
			case strings.Contains(r.URL.Path, "set-group-status"):
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(w, `{"Code":"InternalError","Message":"status backend down"}`)
			default:
				w.WriteHeader(http.StatusOK)
			}
		})

		state, diags := applyCreateWithStub(t, resourceMinioIAMGroup(), map[string]interface{}{
			"name":          name,
			"disable_group": true,
		}, &S3MinioClient{S3Admin: admin})

		if !diags.HasError() {
			t.Fatal("expected Create to fail when the status call fails")
		}
		if state == nil || state.ID != name {
			t.Fatalf("expected the created group to stay in state as tainted, got %#v", state)
		}
	})

	t.Run("concurrent duplicate creates serialize via per-name lock", func(t *testing.T) {
		name := "tfacc-grp-" + acctest.RandString(8)
		var mu sync.Mutex
		exists := false
		var createCalls int

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/group"):
				if !exists {
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `{"Code":"XMinioAdminNoSuchGroup","Message":"The specified group does not exist."}`)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `{"name":%q,"status":"enabled","members":[]}`, name)
			case strings.HasSuffix(r.URL.Path, "/update-group-members"):
				createCalls++
				exists = true
				w.WriteHeader(http.StatusOK)
			case strings.HasSuffix(r.URL.Path, "/set-group-status"):
				w.WriteHeader(http.StatusOK)
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}
		})

		meta := &S3MinioClient{S3Admin: admin}
		res := resourceMinioIAMGroup()
		runConcurrentDuplicateCreate(t, func() (*terraform.InstanceState, diag.Diagnostics) {
			return applyCreateWithStub(t, res, map[string]interface{}{"name": name}, meta)
		}, "minio_iam_group", name, name)

		if createCalls != 1 {
			t.Fatalf("expected exactly 1 UpdateGroupMembers call, got %d", createCalls)
		}
	})
}

func TestResourceMinioIAMPolicyCreateRejectsExistingAndLookupErrors(t *testing.T) {
	const policyJSON = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::original/*"]}]}`

	t.Run("existing policy fails without write call", func(t *testing.T) {
		name := "tfacc-pol-" + acctest.RandString(8)
		var mu sync.Mutex
		var writeCalls []string

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/info-canned-policy") {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"PolicyName":%q,"Policy":%s}`, name, policyJSON)
				return
			}
			mu.Lock()
			writeCalls = append(writeCalls, r.Method+" "+r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		})

		state, diags := applyCreateWithStub(t, resourceMinioIAMPolicy(), map[string]interface{}{
			"name":   name,
			"policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:*"],"Resource":["arn:aws:s3:::*"]}]}`,
		}, &S3MinioClient{S3Admin: admin})

		assertCreateAlreadyExistsError(t, diags, state, "minio_iam_policy", name, name)
		if len(writeCalls) > 0 {
			t.Fatalf("expected no write calls for existing policy, got: %v", writeCalls)
		}
	})

	t.Run("non-not-found lookup error fails create", func(t *testing.T) {
		name := "tfacc-pol-" + acctest.RandString(8)
		var mu sync.Mutex
		var writeCalls []string

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/info-canned-policy") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"Code":"AccessDenied","Message":"Access Denied."}`)
				return
			}
			mu.Lock()
			writeCalls = append(writeCalls, r.Method+" "+r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		})

		state, diags := applyCreateWithStub(t, resourceMinioIAMPolicy(), map[string]interface{}{
			"name":   name,
			"policy": policyJSON,
		}, &S3MinioClient{S3Admin: admin})

		if !diags.HasError() {
			t.Fatal("expected Create to fail when lookup is denied")
		}
		if state != nil && state.ID != "" {
			t.Fatalf("expected empty state ID, got %q", state.ID)
		}
		if len(writeCalls) > 0 {
			t.Fatalf("expected no write calls on lookup error, got: %v", writeCalls)
		}
	})

	t.Run("concurrent duplicate creates serialize via per-name lock", func(t *testing.T) {
		name := "tfacc-pol-" + acctest.RandString(8)
		var mu sync.Mutex
		stored := ""
		var addCalls int

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/info-canned-policy"):
				if stored == "" {
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `{"Code":"XMinioAdminNoSuchPolicy","Message":"The canned policy does not exist"}`)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `{"PolicyName":%q,"Policy":%s}`, name, stored)
			case strings.HasSuffix(r.URL.Path, "/add-canned-policy"):
				addCalls++
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("reading add-canned-policy body: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				stored = string(body)
				w.WriteHeader(http.StatusOK)
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}
		})

		meta := &S3MinioClient{S3Admin: admin}
		res := resourceMinioIAMPolicy()
		runConcurrentDuplicateCreate(t, func() (*terraform.InstanceState, diag.Diagnostics) {
			return applyCreateWithStub(t, res, map[string]interface{}{
				"name":   name,
				"policy": policyJSON,
			}, meta)
		}, "minio_iam_policy", name, name)

		if addCalls != 1 {
			t.Fatalf("expected exactly 1 AddCannedPolicy call, got %d", addCalls)
		}
	})
}

func TestResourceMinioIAMGroupPolicyCreateRejectsExistingAndLookupErrors(t *testing.T) {
	const policyJSON = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::original/*"]}]}`

	t.Run("existing canned policy fails without write call", func(t *testing.T) {
		name := "tfacc-gpol-" + acctest.RandString(8)
		group := "devs"
		var mu sync.Mutex
		var writeCalls []string

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/info-canned-policy"):
				_, _ = fmt.Fprintf(w, `{"PolicyName":%q,"Policy":%s}`, name, policyJSON)
			case strings.HasSuffix(r.URL.Path, "/group"):
				_, _ = fmt.Fprintf(w, `{"name":%q,"status":"enabled","members":[]}`, group)
			default:
				mu.Lock()
				writeCalls = append(writeCalls, r.Method+" "+r.URL.Path)
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}
		})

		state, diags := applyCreateWithStub(t, resourceMinioIAMGroupPolicy(), map[string]interface{}{
			"group":  group,
			"name":   name,
			"policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:*"],"Resource":["arn:aws:s3:::*"]}]}`,
		}, &S3MinioClient{S3Admin: admin})

		assertCreateAlreadyExistsError(t, diags, state, "minio_iam_group_policy", name, group+":"+name)
		if !strings.Contains(fmt.Sprint(diags), "not attached to group") {
			t.Fatalf("expected not-attached guidance when the policy is not attached to the group, got %v", diags)
		}
		if len(writeCalls) > 0 {
			t.Fatalf("expected no write calls for existing group policy, got: %v", writeCalls)
		}
	})

	t.Run("existing attached policy gets the plain import hint", func(t *testing.T) {
		name := "tfacc-gpol-" + acctest.RandString(8)
		group := "devs"

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/info-canned-policy"):
				_, _ = fmt.Fprintf(w, `{"PolicyName":%q,"Policy":%s}`, name, policyJSON)
			case strings.HasSuffix(r.URL.Path, "/group"):
				_, _ = fmt.Fprintf(w, `{"name":%q,"status":"enabled","members":[],"policy":%q}`, group, name)
			default:
				w.WriteHeader(http.StatusOK)
			}
		})

		state, diags := applyCreateWithStub(t, resourceMinioIAMGroupPolicy(), map[string]interface{}{
			"group":  group,
			"name":   name,
			"policy": policyJSON,
		}, &S3MinioClient{S3Admin: admin})

		assertCreateAlreadyExistsError(t, diags, state, "minio_iam_group_policy", name, group+":"+name)
		if strings.Contains(fmt.Sprint(diags), "not attached") {
			t.Fatalf("an attached policy must get the plain import hint, got %v", diags)
		}
	})

	t.Run("non-not-found policy lookup error fails create", func(t *testing.T) {
		name := "tfacc-gpol-" + acctest.RandString(8)
		var mu sync.Mutex
		var writeCalls []string

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/info-canned-policy") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"Code":"AccessDenied","Message":"Access Denied."}`)
				return
			}
			mu.Lock()
			writeCalls = append(writeCalls, r.Method+" "+r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		})

		state, diags := applyCreateWithStub(t, resourceMinioIAMGroupPolicy(), map[string]interface{}{
			"group":  "devs",
			"name":   name,
			"policy": policyJSON,
		}, &S3MinioClient{S3Admin: admin})

		if !diags.HasError() {
			t.Fatal("expected Create to fail when InfoCannedPolicy returns AccessDenied")
		}
		if state != nil && state.ID != "" {
			t.Fatalf("expected empty state ID, got %q", state.ID)
		}
		if len(writeCalls) > 0 {
			t.Fatalf("expected no write calls on lookup error, got: %v", writeCalls)
		}
	})

	t.Run("concurrent duplicate creates serialize via per-name lock", func(t *testing.T) {
		name := "tfacc-gpol-" + acctest.RandString(8)
		group := "devs"
		var mu sync.Mutex
		canned := map[string]string{}
		attached := []string{}
		var addCalls int

		admin := newCreateRejectStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/group"):
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `{"name":%q,"status":"enabled","members":[],"policy":%q}`, group, strings.Join(attached, ","))
			case strings.HasSuffix(r.URL.Path, "/info-canned-policy"):
				body, ok := canned[name]
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `{"Code":"XMinioAdminNoSuchPolicy","Message":"The canned policy does not exist"}`)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `{"PolicyName":%q,"Policy":%s}`, name, body)
			case strings.HasSuffix(r.URL.Path, "/add-canned-policy"):
				addCalls++
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("reading add-canned-policy body: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				canned[name] = string(body)
				w.WriteHeader(http.StatusOK)
			case strings.HasSuffix(r.URL.Path, "/idp/builtin/policy/attach"):
				dec, err := madmin.DecryptData("secretkey", r.Body)
				if err != nil {
					t.Errorf("decrypting attach req: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				var req madmin.PolicyAssociationReq
				if err := json.Unmarshal(dec, &req); err != nil {
					t.Errorf("unmarshaling attach req: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				attached = append([]string(nil), req.Policies...)
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}
		})

		meta := &S3MinioClient{S3Admin: admin}
		res := resourceMinioIAMGroupPolicy()
		runConcurrentDuplicateCreate(t, func() (*terraform.InstanceState, diag.Diagnostics) {
			return applyCreateWithStub(t, res, map[string]interface{}{
				"group":  group,
				"name":   name,
				"policy": policyJSON,
			}, meta)
		}, "minio_iam_group_policy", name, group+":"+name)

		if addCalls != 1 {
			t.Fatalf("expected exactly 1 AddCannedPolicy call, got %d", addCalls)
		}
	})
}

func TestResourceMinioS3BucketCreateTranslatesAlreadyExistsWithoutPreCheck(t *testing.T) {
	for _, code := range []string{minio.BucketAlreadyExists, minio.BucketAlreadyOwnedByYou} {
		t.Run(code, func(t *testing.T) {
			bucket := "tfacc-bkt-" + strings.ToLower(acctest.RandString(8))
			var mu sync.Mutex
			var headCalls, putCalls int

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				switch r.Method {
				case http.MethodHead:
					headCalls++
				case http.MethodPut:
					putCalls++
				}
				mu.Unlock()

				if r.Method == http.MethodPut {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusConflict)
					_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>Bucket already exists.</Message><Resource>/%s</Resource></Error>`, code, bucket)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			s3Client, err := minio.New(strings.TrimPrefix(srv.URL, "http://"), &minio.Options{
				Creds:        credentials.NewStaticV4("accesskey", "secretkey", ""),
				Secure:       false,
				Region:       "us-east-1",
				BucketLookup: minio.BucketLookupPath,
			})
			if err != nil {
				t.Fatalf("creating s3 client: %v", err)
			}

			state, diags := applyCreateWithStub(t, resourceMinioBucket(), map[string]interface{}{
				"bucket": bucket,
			}, &S3MinioClient{S3Client: s3Client})

			assertCreateAlreadyExistsError(t, diags, state, "minio_s3_bucket", bucket, bucket)
			if headCalls != 0 {
				t.Fatalf("expected 0 HEAD (BucketExists) pre-check requests, got %d", headCalls)
			}
			if putCalls != 1 {
				t.Fatalf("expected 1 PUT (MakeBucket) request, got %d", putCalls)
			}
		})
	}

	t.Run("non-conflict error propagates without nil or already-exists translation", func(t *testing.T) {
		bucket := "tfacc-bkt-" + strings.ToLower(acctest.RandString(8))
		var mu sync.Mutex
		var headCalls int

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodHead {
				mu.Lock()
				headCalls++
				mu.Unlock()
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied.</Message><Resource>/%s</Resource></Error>`, bucket)
		}))
		defer srv.Close()

		s3Client, err := minio.New(strings.TrimPrefix(srv.URL, "http://"), &minio.Options{
			Creds:        credentials.NewStaticV4("accesskey", "secretkey", ""),
			Secure:       false,
			Region:       "us-east-1",
			BucketLookup: minio.BucketLookupPath,
		})
		if err != nil {
			t.Fatalf("creating s3 client: %v", err)
		}

		state, diags := applyCreateWithStub(t, resourceMinioBucket(), map[string]interface{}{
			"bucket": bucket,
		}, &S3MinioClient{S3Client: s3Client})

		if !diags.HasError() {
			t.Fatal("expected Create to fail on AccessDenied")
		}
		if state != nil && state.ID != "" {
			t.Fatalf("expected empty state ID, got %q", state.ID)
		}
		if headCalls != 0 {
			t.Fatalf("expected 0 HEAD requests, got %d", headCalls)
		}
		if strings.Contains(diags[0].Summary+" "+diags[0].Detail, "already exists") {
			t.Fatalf("unexpected already exists error for AccessDenied: %v", diags)
		}
	})
}
