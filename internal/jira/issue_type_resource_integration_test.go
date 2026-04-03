package jira_test

import (
	"context"
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
	resource.AddTestSweepers("atlassian_jira_issue_type", &resource.Sweeper{
		Name:         "atlassian_jira_issue_type",
		Dependencies: []string{"atlassian_jira_issue_type_scheme"},
		F:            sweepIssueTypes,
	})
}

func sweepIssueTypes(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	// GET /rest/api/3/issuetype returns a flat list (not paginated).
	var issueTypes []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := client.Get(ctx, "/rest/api/3/issuetype", &issueTypes); err != nil {
		return fmt.Errorf("listing issue types for sweep: %w", err)
	}

	for _, it := range issueTypes {
		if !strings.HasPrefix(it.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/issuetype/%s", atlassian.PathEscape(it.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete issue type %q (%s): %s\n", it.Name, it.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationIssueTypeResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_issue_type" "test" {
  name        = %q
  description = "Integration test issue type (run %s)"
  type        = "standard"
}
`, rName, testutil.RunID()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "name", rName),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "type", "standard"),
					resource.TestCheckResourceAttrSet("atlassian_jira_issue_type.test", "id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_issue_type.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationIssueTypeResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_issue_type" "test" {
  name        = %q
  description = "original"
  type        = "standard"
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "name", rName),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "description", "original"),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_issue_type" "test" {
  name        = %q
  description = "updated"
  type        = "standard"
}
`, rNameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "description", "updated"),
				),
			},
		},
	})
}

func TestIntegrationIssueTypeResource_disappears(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_issue_type" "test" {
  name = %q
  type = "standard"
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testDeleteIssueTypeOutOfBand("atlassian_jira_issue_type.test"),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestIntegrationIssueTypeDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_issue_type" "test" {
  name = %q
  type = "standard"
}

data "atlassian_jira_issue_type" "test" {
  name = atlassian_jira_issue_type.test.name
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_issue_type.test", "id"),
				),
			},
		},
	})
}

func testCheckIssueTypeDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_issue_type" {
			continue
		}
		var result interface{}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/issuetype/%s", atlassian.PathEscape(rs.Primary.ID)),
			&result,
		)
		if err != nil {
			return fmt.Errorf("error checking issue type %s destruction: %w", rs.Primary.ID, err)
		}
		if statusCode != http.StatusNotFound {
			return fmt.Errorf("issue type %s still exists (status %d)", rs.Primary.ID, statusCode)
		}
	}
	return nil
}

func testDeleteIssueTypeOutOfBand(resourceAddr string) resource.TestCheckFunc {
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
		delPath := fmt.Sprintf("/rest/api/3/issuetype/%s", atlassian.PathEscape(rs.Primary.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			return fmt.Errorf("deleting issue type out-of-band: %w", delErr)
		}
		return nil
	}
}
