package minio

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
)

func TestValidateMinioIamGroupName(t *testing.T) {
	minioValidNames := []string{
		"test-user",
		"test_user",
		"testuser123",
		"TestUser",
		"Test-User",
		"test.user",
		"test.123,user",
		"testuser@minio",
		"test+user@minio.io",
		"CN=ADMINS,OU=Groups,DC=gr-u,DC=it",
		"cn=ADMINS,ou=Groups,dc=gr-u,dc=it",
	}

	for _, minioName := range minioValidNames {
		_, err := validateMinioIamGroupName(minioName, "name")
		if len(err) != 0 {
			t.Fatalf("%q should be a valid IAM Group name: %q", minioName, err)
		}
	}

	minioInvalidNames := []string{
		"!",
		"/",
		" ",
		":",
		";",
		"test name",
		"/slash-at-the-beginning",
		"slash-at-the-end/",
		"DC=gr u,DC=it",
		"OU=Microsoft Exchange Security Groups",
	}

	for _, minioName := range minioInvalidNames {
		_, err := validateMinioIamGroupName(minioName, "name")
		if len(err) == 0 {
			t.Fatalf("%q should be an invalid IAM Group name", minioName)
		}
	}
}

func TestAccAWSGroup_Basic(t *testing.T) {
	var conf madmin.GroupDesc

	groupName := fmt.Sprintf("tf-acc-group-basic-%d", acctest.RandInt())
	groupName2 := fmt.Sprintf("tf-acc-group-basic-2-%d", acctest.RandInt())
	status1 := "enabled"
	status2 := "disabled"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioGroupConfig(groupName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupExists("minio_iam_group.test", &conf),
					testAccCheckMinioGroupAttributes(&conf, groupName, status1),
				),
			},
			{
				Config: testAccMinioGroupConfig2(groupName2),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupExists("minio_iam_group.test2", &conf),
					testAccCheckMinioGroupDisable(&conf, groupName2, status2),
				),
			},
			{
				ResourceName:            "minio_iam_group.test2",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"disable_group", "force_destroy", "name"},
			},
		},
	})
}

func testAccMinioGroupConfig(groupName string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "test" {
  name = "%s"
}
`, groupName)
}

func testAccMinioGroupConfig2(groupName string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "test2" {
  name = "%s"
  disable_group = "true"
}
`, groupName)
}

func testAccCheckMinioGroupExists(n string, res *madmin.GroupDesc) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no Group name is set")
		}

		minioIam := testAccClient().S3Admin

		resp, err := minioIam.GetGroupDescription(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}

		*res = *resp

		return nil
	}
}

func testAccCheckMinioGroupAttributes(group *madmin.GroupDesc, name string, status string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if group.Name != name {
			return fmt.Errorf("bad name: %s", group.Name)
		}

		if group.Status != status {
			return fmt.Errorf("bad status: %s", group.Status)
		}

		return nil
	}
}

func testAccCheckMinioGroupDisable(group *madmin.GroupDesc, name string, status string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		minioIam := testAccClient().S3Admin

		if group.Name != name {
			return fmt.Errorf("bad name: %s", group.Name)
		}

		err := minioIam.SetGroupStatus(context.Background(), group.Name, madmin.GroupStatus("disabled"))
		if err != nil {
			return err
		}

		resp, err := minioIam.GetGroupDescription(context.Background(), group.Name)
		if err != nil {
			return err
		}

		if resp.Status != status {
			return fmt.Errorf("bad status: %s", resp.Status)
		}

		return nil
	}
}

func TestWaitForGroupMembersToClear(t *testing.T) {
	errRead := errors.New("read failed")

	tests := []struct {
		name        string
		lists       [][]string
		readErr     error
		wantMembers []string
		wantErr     error
		wantCalls   int
	}{
		{
			name:      "empty on first read",
			lists:     [][]string{{}},
			wantCalls: 1,
		},
		{
			name:      "clears on a later read",
			lists:     [][]string{{"alice", "bob"}, {"bob"}, {}},
			wantCalls: 3,
		},
		{
			name:        "never clears",
			lists:       [][]string{{"alice"}, {"alice"}, {"alice"}},
			wantMembers: []string{"alice"},
			wantCalls:   3,
		},
		{
			name:      "read error stops the wait",
			readErr:   errRead,
			wantErr:   errRead,
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			members := func(context.Context) ([]string, error) {
				defer func() { calls++ }()
				if tt.readErr != nil {
					return nil, tt.readErr
				}
				return tt.lists[calls], nil
			}

			got, err := waitForGroupMembersToClear(context.Background(), members, 3, time.Millisecond)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.wantMembers, got); diff != "" {
				t.Errorf("members mismatch (-want +got):\n%s", diff)
			}
			if calls != tt.wantCalls {
				t.Errorf("read %d time(s), want %d", calls, tt.wantCalls)
			}
		})
	}
}

func TestWaitForGroupMembersToClearHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := waitForGroupMembersToClear(ctx, func(context.Context) ([]string, error) {
		return []string{"alice"}, nil
	}, 3, time.Hour)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want %v", err, context.Canceled)
	}
	if diff := cmp.Diff([]string{"alice"}, got); diff != "" {
		t.Errorf("members mismatch (-want +got):\n%s", diff)
	}
}
