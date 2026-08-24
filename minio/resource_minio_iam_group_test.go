package minio

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

func TestAccMinioIAMGroup_forceDestroyWithOutsideMember(t *testing.T) {
	rString := acctest.RandString(8)
	groupName := fmt.Sprintf("tf-acc-group-fd-%s", rString)
	outsideUser := fmt.Sprintf("tf-acc-user-fd-%s", rString)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			// Registered here, not in the test body: without TF_ACC the SDK
			// skips before PreCheck runs, and a cleanup that reaches
			// testAccClient() would panic the whole package.
			t.Cleanup(func() { _ = testAccClient().S3Admin.RemoveUser(context.Background(), outsideUser) })
		},
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioGroupGone(groupName),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioGroupConfigForceDestroy(groupName, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_group.force", "force_destroy", "true"),
					testAccAddGroupMemberOutsideTerraform(groupName, outsideUser),
				),
			},
		},
	})
}

func TestAccMinioIAMGroup_forceDestroyKeepsGroupOnUpdate(t *testing.T) {
	var group madmin.GroupDesc

	groupName := fmt.Sprintf("tf-acc-group-fdu-%s", acctest.RandString(8))

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioGroupGone(groupName),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioGroupConfigForceDestroy(groupName, false),
				Check:  testAccCheckMinioGroupExists("minio_iam_group.force", &group),
			},
			{
				Config: testAccMinioGroupConfigForceDestroy(groupName, true),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupExists("minio_iam_group.force", &group),
					resource.TestCheckResourceAttr("minio_iam_group.force", "disable_group", "true"),
				),
			},
		},
	})
}

func testAccMinioGroupConfigForceDestroy(groupName string, disabled bool) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "force" {
  name          = "%s"
  force_destroy = true
  disable_group = %t
}
`, groupName, disabled)
}

// testAccAddGroupMemberOutsideTerraform puts a member in the group that no
// Terraform resource knows about, which is the case force_destroy exists for.
func testAccAddGroupMemberOutsideTerraform(groupName string, userName string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		minioIam := testAccClient().S3Admin

		if err := minioIam.AddUser(context.Background(), userName, "tfacc-outside-member"); err != nil {
			return fmt.Errorf("creating user %s outside Terraform: %w", userName, err)
		}

		return minioIam.UpdateGroupMembers(context.Background(), madmin.GroupAddRemove{
			Group:   groupName,
			Members: []string{userName},
		})
	}
}

// testAccCheckMinioGroupGone asserts the group is really gone from MinIO. Only
// a not-found error proves that: treating every error as success would let a
// transient admin API failure pass the destroy check.
func testAccCheckMinioGroupGone(groupName string) func(*terraform.State) error {
	return func(*terraform.State) error {
		minioIam := testAccClient().S3Admin

		_, err := minioIam.GetGroupDescription(context.Background(), groupName)
		if err == nil {
			return fmt.Errorf("group %s still exists", groupName)
		}
		if !strings.Contains(err.Error(), "not exist") {
			return fmt.Errorf("checking group %s was destroyed: %w", groupName, err)
		}

		return nil
	}
}
