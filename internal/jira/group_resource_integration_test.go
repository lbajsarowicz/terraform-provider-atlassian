package jira_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func init() {
	resource.AddTestSweepers("atlassian_jira_group", &resource.Sweeper{
		Name: "atlassian_jira_group",
		F:    sweepGroups,
	})
}

func sweepGroups(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	// List all groups using the bulk endpoint.
	allValues, err := client.GetAllPages(ctx, "/rest/api/3/group/bulk")
	if err != nil {
		return fmt.Errorf("listing groups for sweep: %w", err)
	}

	for _, raw := range allValues {
		var group struct {
			GroupID string `json:"groupId"`
			Name    string `json:"name"`
		}
		if err := json.Unmarshal(raw, &group); err != nil {
			continue
		}
		if !strings.HasPrefix(group.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/group?groupId=%s", atlassian.QueryEscape(group.GroupID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete group %q (%s): %s\n", group.Name, group.GroupID, delErr)
		}
	}

	return nil
}

func TestIntegrationGroupResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckGroupDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_group" "test" { name = %q }`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_group.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_group.test", "group_id"),
				),
			},
			{
				ResourceName:                         "atlassian_jira_group.test",
				ImportState:                          true,
				ImportStateId:                        rName,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "group_id",
			},
		},
	})
}

func TestIntegrationGroupResource_disappears(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckGroupDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_group" "test" { name = %q }`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Delete the group out-of-band to simulate external deletion.
					testDeleteGroupOutOfBand("atlassian_jira_group.test"),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestIntegrationGroupDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckGroupDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_group" "test" { name = %q }

data "atlassian_jira_group" "test" {
  name = atlassian_jira_group.test.name
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_group.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_group.test", "group_id"),
				),
			},
		},
	})
}

func testCheckGroupDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_group" {
			continue
		}
		var result interface{}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/group?groupId=%s", atlassian.QueryEscape(rs.Primary.Attributes["group_id"])),
			&result,
		)
		if err != nil {
			return fmt.Errorf("error checking group %s destruction: %w", rs.Primary.Attributes["group_id"], err)
		}
		if statusCode != http.StatusNotFound {
			return fmt.Errorf("group %s still exists (status %d)", rs.Primary.Attributes["group_id"], statusCode)
		}
	}
	return nil
}

func testDeleteGroupOutOfBand(resourceAddr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceAddr)
		}
		client, err := testutil.SweepClient()
		if err != nil {
			return fmt.Errorf("creating client: %w", err)
		}
		ctx := context.Background()
		groupID := rs.Primary.Attributes["group_id"]
		delPath := fmt.Sprintf("/rest/api/3/group?groupId=%s", atlassian.QueryEscape(groupID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			return fmt.Errorf("deleting group out-of-band: %w", delErr)
		}
		return nil
	}
}
