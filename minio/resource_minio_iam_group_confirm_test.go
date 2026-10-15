package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestConfirmGroupRemoved(t *testing.T) {
	const notFound = `{"Code":"XMinioAdminNoSuchGroup","Message":"The specified group does not exist.","Resource":"/minio/admin/v3/group"}`
	const denied = `{"Code":"AccessDenied","Message":"Access Denied.","Resource":"/minio/admin/v3/group"}`

	cases := []struct {
		name             string
		listedReads      int
		deniedRead       bool
		wantErr          string
		wantExtraRemoves int
	}{
		{name: "gone after the first removal", listedReads: 0, wantExtraRemoves: 0},
		{name: "listed once more is removed again", listedReads: 1, wantExtraRemoves: 1},
		{name: "still listed after every attempt fails the delete", listedReads: 99, wantErr: "is still listed after 3 removals", wantExtraRemoves: 2},
		{name: "a denied read fails without removing again", deniedRead: true, wantErr: "checking that the group was removed", wantExtraRemoves: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			reads, removes := 0, 0

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/update-group-members"):
					removes++
					w.WriteHeader(http.StatusOK)
				case strings.HasSuffix(r.URL.Path, "/group"):
					reads++
					switch {
					case tc.deniedRead:
						w.WriteHeader(http.StatusForbidden)
						_, _ = fmt.Fprint(w, denied)
					case reads <= tc.listedReads:
						w.WriteHeader(http.StatusOK)
						_, _ = fmt.Fprint(w, `{"name":"tfacc-group","status":"enabled","members":[],"policy":""}`)
					default:
						w.WriteHeader(http.StatusNotFound)
						_, _ = fmt.Fprint(w, notFound)
					}
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
				Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
				Secure: false,
			})
			if err != nil {
				t.Fatalf("creating the admin client: %s", err)
			}

			err = confirmGroupRemoved(context.Background(), &S3MinioIAMGroupConfig{MinioAdmin: admin}, "tfacc-group", 3, time.Millisecond)

			if tc.wantErr == "" && err != nil {
				t.Fatalf("confirmGroupRemoved() = %v, want nil", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("confirmGroupRemoved() = %v, want an error containing %q", err, tc.wantErr)
			}
			if removes != tc.wantExtraRemoves {
				t.Errorf("removed the group again %d times, want %d", removes, tc.wantExtraRemoves)
			}
		})
	}
}
