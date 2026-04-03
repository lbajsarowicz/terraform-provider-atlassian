package jira_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func init() {
	resource.AddTestSweepers("atlassian_jira_status", &resource.Sweeper{
		Name:         "atlassian_jira_status",
		Dependencies: []string{"atlassian_jira_workflow"},
		F:            sweepStatuses,
	})
}

func sweepStatuses(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	allValues, err := client.GetAllPages(ctx, "/rest/api/3/statuses/search")
	if err != nil {
		return fmt.Errorf("listing statuses for sweep: %w", err)
	}

	for _, raw := range allValues {
		var status struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &status); err != nil {
			continue
		}
		if !strings.HasPrefix(status.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/statuses?id=%s", atlassian.QueryEscape(status.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete status %q (%s): %s\n", status.Name, status.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationStatusResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckStatusDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_status" "test" {
  name            = %q
  description     = "Integration test status (run %s)"
  status_category = "TODO"
}
`, rName, testutil.RunID()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "name", rName),
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "status_category", "TODO"),
					resource.TestCheckResourceAttrSet("atlassian_jira_status.test", "id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_status.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationStatusResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckStatusDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_status" "test" {
  name            = %q
  description     = "original"
  status_category = "TODO"
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "name", rName),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_status" "test" {
  name            = %q
  description     = "updated"
  status_category = "TODO"
}
`, rNameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "description", "updated"),
				),
			},
		},
	})
}

func TestIntegrationStatusResource_disappears(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckStatusDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_status" "test" {
  name            = %q
  status_category = "TODO"
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testDeleteStatusOutOfBand("atlassian_jira_status.test"),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestIntegrationStatusDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckStatusDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_status" "test" {
  name            = %q
  status_category = "TODO"
}

data "atlassian_jira_status" "test" {
  name = atlassian_jira_status.test.name
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_status.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_status.test", "id"),
					resource.TestCheckResourceAttr("data.atlassian_jira_status.test", "status_category", "TODO"),
				),
			},
		},
	})
}

func testCheckStatusDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_status" {
			continue
		}
		var result struct {
			Values []struct {
				ID string `json:"id"`
			} `json:"values"`
		}
		searchPath := fmt.Sprintf("/rest/api/3/statuses/search?id=%s&maxResults=1",
			atlassian.QueryEscape(rs.Primary.ID))
		if err := client.Get(ctx, searchPath, &result); err != nil {
			return fmt.Errorf("error checking status %s destruction: %w", rs.Primary.ID, err)
		}
		if len(result.Values) > 0 {
			return fmt.Errorf("status %s still exists", rs.Primary.ID)
		}
	}
	return nil
}

func testDeleteStatusOutOfBand(resourceAddr string) resource.TestCheckFunc {
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
		delPath := fmt.Sprintf("/rest/api/3/statuses?id=%s", atlassian.QueryEscape(rs.Primary.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			return fmt.Errorf("deleting status out-of-band: %w", delErr)
		}
		return nil
	}
}
